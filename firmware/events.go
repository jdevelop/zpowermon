package firmware

import "time"

type PowerEvent struct {
	Timestamp time.Time `json:"Time"`
	Message   struct {
		ID          *uint `json:"ID"`
		EndpointID  *uint `json:"EndpointID"`
		Consumption uint  `json:"Consumption"`
	} `json:"Message"`
}

func (evt *PowerEvent) GetId() uint {
	if evt.Message.ID != nil {
		return *evt.Message.ID
	} else {
		return *evt.Message.EndpointID
	}
}
