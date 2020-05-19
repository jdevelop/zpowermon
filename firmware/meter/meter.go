package meter

import (
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/bemasher/rtlamr/protocol"
	"github.com/bemasher/rtltcp"

	_ "github.com/bemasher/rtlamr/scm"
	_ "github.com/bemasher/rtlamr/scmplus"
)

type Meter struct {
	rtltcp.SDR
	decoder []*protocol.Decoder
	maxSize int
	stop    chan struct{}
}

var types = []string{"scm", "scm+"}

const symLen = 72

func NewMeter(rtlTcpAddr string) (*Meter, error) {
	var m Meter
	m.decoder = make([]*protocol.Decoder, 1)
	for i := range m.decoder {
		d := protocol.NewDecoder()
		m.decoder[i] = &d
		for _, mt := range types {
			protoParser, err := protocol.NewParser(mt, 72)
			if err != nil {
				return nil, err
			}
			d.RegisterProtocol(protoParser)
		}
		d.Allocate()
		if d.Cfg.BlockSize2 > m.maxSize {
			m.maxSize = d.Cfg.BlockSize2
		}
	}
	if address, err := net.ResolveTCPAddr("tcp", rtlTcpAddr); err != nil {
		return nil, err
	} else if err := m.Connect(address); err != nil {
		return nil, err
	}
	cfg := m.decoder[0].Cfg
	m.SetCenterFreq(cfg.CenterFreq)
	m.SetSampleRate(uint32(cfg.SampleRate))

	m.stop = make(chan struct{})

	return &m, nil
}

func (m *Meter) Close() error {
	close(m.stop)
	return m.SDR.Close()
}

type Counter struct {
	*sync.Pool
}

func (c *Counter) Get() []byte {
	return c.Pool.Get().([]byte)
}

func (c *Counter) Put(src []byte) {
	c.Pool.Put(src)
}

func (m *Meter) Run(consumer chan<- protocol.Message) error {
	buffer := Counter{&sync.Pool{
		New: func() interface{} {
			return make([]byte, m.maxSize)
		},
	}}

	var bufferChan = make(chan []byte)
	defer close(bufferChan)

	for _, w := range m.decoder {
		go func(d *protocol.Decoder) {
			for {
				select {
				case <-m.stop:
					return
				case pd, ok := <-bufferChan:
					if !ok {
						return
					}
					decodeChan := d.Decode(pd)
				decodeLoop:
					for {
						select {
						case <-m.stop:
							buffer.Put(pd)
							return
						case msg, ok := <-decodeChan:
							if !ok {
								break decodeLoop
							}
							select {
							case consumer <- msg:
								// ok
							case <-time.After(100 * time.Millisecond):
								log.Printf("timeout for message processing")
							}
						}
					}
					buffer.Put(pd)
				}
			}
		}(w)
	}

	for {
		select {
		case <-m.stop:
			return nil
		default:
			if err := m.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
				return err
			}
			pd := buffer.Get()
			if _, err := io.ReadFull(m, pd); err != nil {
				buffer.Put(pd)
				switch err := err.(type) {
				case *net.OpError:
					if err.Temporary() {
						continue
					} else {
						return err
					}
				default:
					return err
				}
			}
			bufferChan <- pd
		}
	}
	return nil
}
