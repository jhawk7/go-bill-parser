package tsdb

import (
	"context"
	"fmt"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/jhawk7/bill-parser/internal/common"
)

type DBConn struct {
	client influxdb2.Client
	org    string
	bucket string
}

type Record struct {
	RecordType string
	Amount     float64
	Timestamp  time.Time
}

func InitTSDB(config *common.Config) *DBConn {
	client := influxdb2.NewClient(config.DBUrl, config.DBToken)
	org, bucket := config.DBOrg, config.DBBucket
	return &DBConn{
		client: client,
		org:    org,
		bucket: bucket,
	}
}

func (conn *DBConn) WriteRecords(records []*Record) (errcount int) {
	w := conn.client.WriteAPIBlocking(conn.org, conn.bucket)
	for _, record := range records {
		point := influxdb2.NewPoint(record.RecordType,
			map[string]string{"unit": "dollar"},
			map[string]interface{}{"amount": record.Amount},
			record.Timestamp,
		)

		if err := w.WritePoint(context.Background(), point); err != nil {
			common.LogError(fmt.Errorf("failed to write record to db; %v", err), false)
			errcount++
		}
	}

	if errcount == 0 {
		common.LogInfo(fmt.Sprintf("successfully wrote %v records to tsdb", len(records)))
	}

	return
}
