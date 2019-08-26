package firmware

import (
	"encoding/json"
	"io"
)

type EventHandler func(evt *PowerEvent) error

func ParseEventStream(stream io.Reader, handler EventHandler) error {
	dec := json.NewDecoder(stream)
	for dec.More() {
		var evt PowerEvent
		if err := dec.Decode(&evt); err != nil {
			return err
		}
		if err := handler(&evt); err != nil {
			return err
		}
	}
	return nil
}
