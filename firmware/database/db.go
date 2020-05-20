package database

import (
	"fmt"
	"strconv"

	"github.com/influxdata/influxdb/client/v2"
	"github.com/jdevelop/zpowermon/firmware"
)

type Database struct {
	dbname string
	dbh    client.Client
}

func Connect(dbname, host string, port int, username, password string) (*Database, error) {

	address := fmt.Sprintf("http://%s:%d", host, port)

	c, err := client.NewHTTPClient(client.HTTPConfig{
		Addr:     address,
		Username: username,
		Password: password,
	})

	if err != nil {
		return nil, err
	}

	db := &Database{
		dbname: dbname,
		dbh:    c,
	}

	return db, nil

}

func (db *Database) AddEvent(event *firmware.PowerEvent) error {
	bp, err := client.NewBatchPoints(client.BatchPointsConfig{
		Database:  db.dbname,
		Precision: "s",
	})
	if err != nil {
		return err
	}
	tags := map[string]string{
		"meterid":   strconv.FormatUint(uint64(event.EndpointID), 10),
		"metertype": event.MeterType,
	}

	pointData := map[string]interface{}{
		"consumption": event.Consumption * 1.0,
	}

	for k, v := range event.Fields {
		pointData[k] = v
	}

	pt, err := client.NewPoint(db.dbname, tags, pointData, event.Timestamp)
	if err != nil {
		return err
	}
	bp.AddPoint(pt)

	return db.dbh.Write(bp)
}
