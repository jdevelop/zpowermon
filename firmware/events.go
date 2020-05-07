package firmware

import "time"

type PowerMessage struct {
	EndpointID  uint `json:"EndpointID"`
	Consumption uint `json:"Consumption"`
}

type PowerEvent struct {
	Timestamp time.Time    `json:"Time"`
	Message   PowerMessage `json:"message"`
}
