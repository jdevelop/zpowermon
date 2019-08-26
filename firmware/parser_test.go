package firmware

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSimpleParse(t *testing.T) {

	f, err := os.Open("testdata/events.json")
	if err != nil {
		t.Fatal(err)
	}

	events := make([]PowerEvent, 0, 10)

	err = ParseEventStream(f, func(evt *PowerEvent) error {
		events = append(events, *evt)
		return nil
	})

	assert.Nil(t, err)

	assert.Equal(t, 2, len(events))
}
