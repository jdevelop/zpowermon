package firmware

import "time"

type PowerEvent struct {
	Timestamp   time.Time
	EndpointID  uint
	Consumption uint
	MeterType   string
	Fields      map[string]interface{}
}
