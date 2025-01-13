package main

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jhawk7/bill-parser/internal/common"
	gmailsvc "github.com/jhawk7/bill-parser/internal/gmail_svc"
	"github.com/jhawk7/bill-parser/internal/tsdb"
	"google.golang.org/api/gmail/v1"
)

var (
	dbconn *tsdb.DBConn
	svc    *gmail.Service
	config *common.Config
)

func main() {
	config = common.GetConfig()
	dbconn = tsdb.InitTSDB(config)
	svc = gmailsvc.InitGSvc()

	messages, ids, unreadLabelId := GetMessages()
	if len(messages) > 0 {
		records := ParseMessages(messages)
		errcount := dbconn.WriteRecords(records)

		if errcount == 0 {
			// mark emails as read
			if markErr := markEmailsRead(ids, unreadLabelId); markErr != nil {
				common.LogError(fmt.Errorf("failed to mark emails as read; %v", markErr), false)
			} else {
				common.LogInfo(fmt.Sprintf("marked %v messages as read", len(ids)))
			}
		}
	}
}

func GetMessages() ([]*gmail.Message, []string, string) {
	labelRes, lErr := svc.Users.Labels.List("me").Fields().Do()
	if lErr != nil {
		common.LogError(fmt.Errorf("failed to get mail labels; %v", lErr), false)
		return nil, nil, ""
	}

	var customLabelId string
	var unreadLabelId string

	for _, label := range labelRes.Labels {
		if label.Name == "Bills" {
			customLabelId = label.Id
		}

		if label.Name == "UNREAD" {
			unreadLabelId = label.Id
		}
	}

	//get all unread email IDs under bills label (this only gets the IDs despite other documented fields on the list response!)
	listRes, mesErr := svc.Users.Messages.List("me").LabelIds(customLabelId, unreadLabelId).Do()
	if mesErr != nil {
		common.LogError(fmt.Errorf("failed to get list of messages by label; %v", mesErr), false)
	}

	var messages []*gmail.Message
	var ids []string

	for _, m := range listRes.Messages {
		msg, mErr := svc.Users.Messages.Get("me", m.Id).Do()
		if mErr != nil {
			common.LogError(fmt.Errorf("failed to get message by ID %v, %v", m.Id, mErr), false)
			continue
		}

		ids = append(ids, m.Id)
		messages = append(messages, msg)
	}

	common.LogInfo(fmt.Sprintf("%v messages found", len(messages)))
	return messages, ids, unreadLabelId
}

func ParseMessages(messages []*gmail.Message) []*tsdb.Record {
	var records []*tsdb.Record

	for _, message := range messages {
		//get body of message

		var text string
		for _, part := range message.Payload.Parts {
			if part.MimeType == "text/plain" {
				text = decodeMessageTxt(part.Body.Data)

			} else if part.MimeType == "multipart/alternative" {
				for _, subPart := range part.Parts {
					if subPart.MimeType == "text/plain" {
						text = decodeMessageTxt(subPart.Body.Data)
					}
				}
			}
		}

		if len(text) == 0 {
			continue
		}

		amount, pErr := parseDollarAmount(text)
		if pErr != nil {
			common.LogError(pErr, false)
			continue
		}

		record, recErr := createRecord(text, amount, message.InternalDate)
		if recErr != nil {
			common.LogError(fmt.Errorf("failed to create record; %v", recErr), false)
			continue
		}
		records = append(records, record)
	}

	common.LogInfo(fmt.Sprintf("created %v record(s)", len(records)))
	return records
}

func decodeMessageTxt(body string) (text string) {
	decodedBytes, dErr := base64.URLEncoding.DecodeString(body)
	if dErr != nil {
		common.LogError(fmt.Errorf("failed to url decode email body; %v", dErr), false)
		return
	}

	text = string(decodedBytes)
	return
}

func parseDollarAmount(text string) (amount string, err error) {
	re := regexp.MustCompile(`\$(\s+|\s?)\d+(?:,\d{3})*(?:\.\d{2})?`)
	matches := re.FindAllString(text, -1)
	if len(matches) == 0 {
		err = fmt.Errorf("failed to find dollar amounts in email text")
		return
	}

	strAmount := matches[len(matches)-1]
	strAmount = strings.Replace(strAmount, "$", "", -1)
	amount = strings.Trim(strAmount, " ")
	return
}

func createRecord(text string, amount string, epoch int64) (record *tsdb.Record, err error) {
	var recordType string
	re := regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
	sender := re.FindString(text)
	recordType = config.SenderMap[sender]

	amountF64, convErr := strconv.ParseFloat(amount, 64)
	if convErr != nil {
		err = convErr
		return
	}

	record = &tsdb.Record{
		RecordType: recordType,
		Amount:     amountF64,
		Timestamp:  time.Unix(0, epoch*int64(time.Millisecond)),
	}

	fmt.Printf("record type: %v, record amount %v, ts: %v\n", record.RecordType, record.Amount, record.Timestamp)

	return
}

func markEmailsRead(ids []string, unreadLabelId string) error {
	// mark emails as read
	removeLabelReq := gmail.BatchModifyMessagesRequest{
		Ids:            ids,
		RemoveLabelIds: []string{unreadLabelId},
	}

	modErr := svc.Users.Messages.BatchModify("me", &removeLabelReq).Do()
	return modErr
}
