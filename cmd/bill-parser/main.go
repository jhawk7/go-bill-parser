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

	messages, unreadLabelId := GetMessages()
	if len(messages) > 0 {
		records, parsedEmailIds := ParseMessages(messages)
		errcount := dbconn.WriteRecords(records)

		if errcount == 0 {
			// mark emails as read
			if markErr := markEmailsRead(parsedEmailIds, unreadLabelId); markErr != nil {
				common.LogError(fmt.Errorf("failed to mark emails as read; %v", markErr), false)
			} else {
				common.LogInfo(fmt.Sprintf("marked %v messages as read", len(parsedEmailIds)))
			}
		}
	}
}

func GetMessages() ([]*gmail.Message, string) {
	labelRes, lErr := svc.Users.Labels.List("me").Fields().Do()
	if lErr != nil {
		common.LogError(fmt.Errorf("failed to get mail labels; %v", lErr), false)
		return nil, ""
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

	for _, m := range listRes.Messages {
		msg, mErr := svc.Users.Messages.Get("me", m.Id).Do()
		if mErr != nil {
			common.LogError(fmt.Errorf("failed to get message by ID %v, %v", m.Id, mErr), false)
			continue
		}

		messages = append(messages, msg)
	}

	common.LogInfo(fmt.Sprintf("%v messages found", len(messages)))
	return messages, unreadLabelId
}

func ParseMessages(messages []*gmail.Message) (records []*tsdb.Record, parsedEmailIds []string) {

	for _, message := range messages {
		//get body of message

		var text string
		for _, part := range message.Payload.Parts {
			if part.MimeType == "text/plain" || part.MimeType == "text/html" {
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
			common.LogInfo("no text parsed")
			continue
		}

		amount, pErr := parseDollarAmount(text)
		if pErr != nil {
			common.LogError(pErr, false)
			continue
		}

		record, recErr := createRecord(message, amount, text)
		if recErr != nil {
			common.LogError(fmt.Errorf("failed to create record; %v", recErr), false)
			continue
		}

		parsedEmailIds = append(parsedEmailIds, message.Id)
		records = append(records, record)
	}

	common.LogInfo(fmt.Sprintf("created %v record(s)", len(records)))
	return records, parsedEmailIds
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
	strAmount = strings.Replace(strAmount, ",", "", -1)
	amount = strings.Trim(strAmount, " ")
	return
}

func createRecord(message *gmail.Message, amount string, text string) (record *tsdb.Record, err error) {
	var recordType string

	parseSender := func(t string) string {
		re := regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
		sender := re.FindString(t)
		return config.SenderMap[sender]
	}

	for _, headers := range message.Payload.Headers {
		if headers.Name == "From" {
			recordType = parseSender(headers.Value)
			break
		}
	}

	if len(recordType) == 0 {
		common.LogInfo("no record type found for sender; parsing text body")
		recordType = parseSender(text)
	}

	amountF64, convErr := strconv.ParseFloat(amount, 64)
	if convErr != nil {
		err = convErr
		return
	}

	record = &tsdb.Record{
		RecordType: recordType,
		Amount:     amountF64,
		Timestamp:  time.UnixMilli(message.InternalDate),
	}

	fmt.Printf("record type: %v, record amount %v, ts: %v\n", record.RecordType, record.Amount, record.Timestamp)

	return
}

func markEmailsRead(ids []string, unreadLabelId string) error {
	// mark emails as read
	if len(ids) == 0 {
		return fmt.Errorf("no email ids given")
	}
	removeLabelReq := gmail.BatchModifyMessagesRequest{
		Ids:            ids,
		RemoveLabelIds: []string{unreadLabelId},
	}

	modErr := svc.Users.Messages.BatchModify("me", &removeLabelReq).Do()
	return modErr
}
