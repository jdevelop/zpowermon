// RTLAMR - An rtl-sdr receiver for smart meters operating in the 900MHz ISM band.
// Copyright (C) 2015 Douglas Hall
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published
// by the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package packetio

import (
	"bytes"
	"flag"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"time"

	"github.com/pkg/errors"

	"github.com/bemasher/rtlamr/protocol"
	"github.com/bemasher/rtltcp"

	_ "github.com/bemasher/rtlamr/idm"
	_ "github.com/bemasher/rtlamr/netidm"
	_ "github.com/bemasher/rtlamr/r900"
	_ "github.com/bemasher/rtlamr/r900bcd"
	_ "github.com/bemasher/rtlamr/scm"
	_ "github.com/bemasher/rtlamr/scmplus"
)

var rcvr Receiver

type Receiver struct {
	rtltcp.SDR
	d  protocol.Decoder
	fc protocol.FilterChain

	stop chan struct{}

	err error
}

type UniqueFilter map[uint][]byte

func NewUniqueFilter() UniqueFilter {
	return make(UniqueFilter)
}

func (uf UniqueFilter) Filter(msg protocol.Message) bool {
	checksum := msg.Checksum()
	mid := uint(msg.MeterID())

	if val, ok := uf[mid]; ok && bytes.Compare(val, checksum) == 0 {
		return false
	}

	uf[mid] = make([]byte, len(checksum))
	copy(uf[mid], checksum)
	return true
}

func (rcvr *Receiver) NewReceiver(protocols []string) {
	rcvr.d = protocol.NewDecoder()

	rcvr.stop = make(chan struct{}, 1)

	// For each given msgType, register it with the decoder.
	for _, name := range protocols {
		p, err := protocol.NewParser(name, 72) // symbol length in samples (8, 32, 40, 48, 56, 64, 72, 80, 88, 96)
		if err != nil {
			log.Fatal(err)
		}

		rcvr.d.RegisterProtocol(p)
	}

	// Allocate the internal buffers of the decoder.
	rcvr.d.Allocate()

	// Connect to rtl_tcp server.
	if rcvr.err = rcvr.Connect(nil); rcvr.err != nil {
		log.Fatalf("%+v", errors.Wrap(rcvr.err, "rcvr.Connect"))
	}

	cfg := rcvr.d.Cfg

	gainFlagSet := false
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "centerfreq":
			cfg.CenterFreq = uint32(rcvr.Flags.CenterFreq)
		case "samplerate":
			cfg.SampleRate = int(rcvr.Flags.SampleRate)
		case "gainbyindex", "tunergainmode", "tunergain", "agcmode":
			gainFlagSet = true
		case "unique":
			rcvr.fc.Add(NewUniqueFilter())
		}
	})

	rcvr.SetCenterFreq(cfg.CenterFreq)
	rcvr.SetSampleRate(uint32(cfg.SampleRate))

	if !gainFlagSet {
		rcvr.SetGainMode(true)
	}

	rcvr.d.Cfg = cfg
	rcvr.d.Log()

	// Tell the user how many gain settings were reported by rtl_tcp.
	log.Println("GainCount:", rcvr.SDR.Info.GainCount)

	return
}

func (rcvr *Receiver) Close() {
	rcvr.stop <- struct{}{}
	rcvr.SDR.Close()
}

func (rcvr *Receiver) Run() {
	// Setup signal channel for interruption.
	sigint := make(chan os.Signal, 1)
	signal.Notify(sigint, os.Kill, os.Interrupt)

	// Setup time limit channel

	sampleBuf := new(bytes.Buffer)

	// Allocate a channel of blocks.
	blockCh := make(chan []byte)

	// Make maps for tracking messages spanning sample blocks.
	prev := map[protocol.Digest]bool{}
	next := map[protocol.Digest]bool{}

	// Read and send sample blocks to the decoder.
	go func() {
		// Make two sample blocks, one for reading, and one for the receiver to
		// decode, these are exchanged each time we read a new block.
		blockA := make([]byte, rcvr.d.Cfg.BlockSize2)
		blockB := make([]byte, rcvr.d.Cfg.BlockSize2)

		// When exiting this goroutine, close the block channel.
		defer close(blockCh)

		for {
			select {
			// Exit if we've been told to stop.
			case <-rcvr.stop:
				return
			default:
				rcvr.err = rcvr.SetDeadline(time.Now().Add(5 * time.Second))
				if rcvr.err != nil {
					rcvr.err = errors.Wrap(rcvr.err, "rcvr.SetDeadline")
					return
				}

				// Read new sample block.
				_, rcvr.err = io.ReadFull(rcvr, blockA)
				rcvr.err = errors.Wrap(rcvr.err, "io.ReadFull")

				// If we get an EOF, exit.
				if rcvr.err == io.EOF || rcvr.err == io.ErrUnexpectedEOF {
					return
				}

				if rcvr.err != nil {
					switch err := rcvr.err.(type) {
					// If we get a network operation error.
					case *net.OpError:
						if err.Temporary() {
							// If temporary, keep trying to read.
							continue
						} else {
							// If it's not temporary, exit.
							return
						}
					default:
						// For everything else, assume it's fatal and bail.
						return
					}
				}

				// Send the sample block.
				blockCh <- blockA

				// Exchange blocks for next read.
				blockA, blockB = blockB, blockA
			}
		}
	}()

	for {
		// Exit on interrupt or time limit, otherwise receive.
		select {
		case <-sigint:
			return
		case block, ok := <-blockCh:
			// If blockCh is closed, exit.
			if !ok {
				return
			}

			// Clear next map for this sample block.
			for key := range next {
				delete(next, key)
			}

			// For each message returned
			for msg := range rcvr.d.Decode(block) {
				// If the filterchain rejects the message, skip it.
				if !rcvr.fc.Match(msg) {
					continue
				}

				// Make a new LogMessage
				var logMsg protocol.LogMessage
				logMsg.Time = time.Now()
				logMsg.Length = sampleBuf.Len()
				logMsg.Type = msg.MsgType()
				logMsg.Message = msg

				// This should be unique enough to identify a message between blocks.
				msgDigest := protocol.NewDigest(msg)

				// Mark the message as seen for the next loop.
				next[msgDigest] = true

				// If the message was seen in the previous loop, skip it.
				if prev[msgDigest] {
					continue
				}

				// Encode the message
				rcvr.err = errors.Wrap(rcvr.err, "encoder.Encode")

				if rcvr.err != nil {
					return
				}

			}

			// Swap next and previous digest maps.
			next, prev = prev, next
		}
	}
}

func init() {
	log.SetFlags(log.Lshortfile | log.Lmicroseconds)
}
