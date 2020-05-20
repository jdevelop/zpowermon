package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"time"

	"github.com/bemasher/rtlamr/idm"
	"github.com/bemasher/rtlamr/netidm"
	"github.com/bemasher/rtlamr/protocol"
	"github.com/bemasher/rtlamr/r900"
	"github.com/bemasher/rtlamr/r900bcd"
	"github.com/bemasher/rtlamr/scm"
	"github.com/bemasher/rtlamr/scmplus"
	"github.com/jdevelop/zpowermon/firmware"
	"github.com/jdevelop/zpowermon/firmware/database"
	"github.com/jdevelop/zpowermon/firmware/meter"
)

const (
	envRtlTcpHost  = "RTL_TCP_HOST"
	envRtlTcpPort  = "RTL_TCP_PORT"
	envInfluxHost  = "INFLUXDB_HOST"
	envInfluxPort  = "INFLUXDB_PORT"
	envInfluxUser  = "INFLUXDB_USER"
	envInfluxPass  = "INFLUXDB_PASS"
	envInfluxName  = "INFLUXDB_NAME"
	envEmbedRTLTCP = "EMBED_RTLTCP"
)

var Version string

func getOSEnv(name string, value *string) error {
	if v, ok := os.LookupEnv(name); ok {
		*value = v
		return nil
	} else {
		return fmt.Errorf("can't find environment variable %s", name)
	}
}

func asInt(src string) int {
	if v, err := strconv.ParseInt(src, 10, 64); err != nil {
		panic(err)
	} else {
		return int(v)
	}
}

func main() {

	var (
		influxDbName, influxHost, influxPort, influxUser, influxPwd, rtlHost, rtlPort string
	)
	for _, kv := range []struct {
		name  string
		value *string
	}{{envRtlTcpHost, &rtlHost}, {envRtlTcpPort, &rtlPort}, {envInfluxHost, &influxHost},
		{envInfluxPort, &influxPort}, {envInfluxUser, &influxUser}, {envInfluxPass, &influxPwd}, {envInfluxName, &influxDbName}} {
		if err := getOSEnv(kv.name, kv.value); err != nil {
			log.Fatal(err)
		}
	}

	db, err := database.Connect(influxDbName, influxHost, asInt(influxPort), influxUser, envInfluxPass)
	if err != nil {
		log.Fatal(err)
	}

	if os.Getenv(envEmbedRTLTCP) == "true" {
		var c = make(chan os.Signal, 1)
		defer close(c)
		signal.Notify(c, os.Interrupt, os.Kill)
		go func() {
			cmd := exec.Command("rtl_tcp", "-a", rtlHost, "-p", rtlPort)
			go func(cmd *exec.Cmd) {
				<-c
				cmd.Process.Kill()
			}(cmd)
			if err := cmd.Run(); err != nil {
				log.Println(err)
			}
		}()

		time.Sleep(5 * time.Second)
	}

	m, err := meter.NewMeter(rtlHost + ":" + rtlPort)
	if err != nil {
		log.Fatal(err)
	}
	defer m.Close()

	msgChan := make(chan protocol.Message)

	go func() {
		for msg := range msgChan {
			evt := firmware.PowerEvent{
				Timestamp:  time.Now(),
				EndpointID: uint(msg.MeterID()),
			}
			switch m := msg.(type) {
			case scm.SCM:
				evt.Consumption = uint(m.Consumption)
				evt.MeterType = "scm"
				evt.Fields = map[string]interface{}{
					"type":      m.Type,
					"tamperphy": m.TamperPhy,
					"tamperenc": m.TamperEnc,
					"checksum":  m.ChecksumVal,
				}
			case scmplus.SCM:
				evt.Consumption = uint(m.Consumption)
				evt.MeterType = "scm+"
				evt.Fields = map[string]interface{}{
					"framesync":    m.FrameSync,
					"protocolid":   m.ProtocolID,
					"endpointtype": m.EndpointType,
					"tamper":       m.Tamper,
					"packetcrc":    m.PacketCRC,
				}
			case idm.IDM:
				evt.Consumption = uint(m.LastConsumptionCount)
				evt.MeterType = "idm"
				evt.Fields = map[string]interface{}{
					"preamble":                         m.Preamble,
					"packettypeid":                     m.PacketTypeID,
					"packetlength":                     m.PacketLength,
					"hammingcode":                      m.HammingCode,
					"applicationversion":               m.ApplicationVersion,
					"erttype":                          m.ERTType,
					"ertserialnumber":                  m.ERTSerialNumber,
					"consumptionintervalcount":         m.ConsumptionIntervalCount,
					"moduleprogrammingstate":           m.ModuleProgrammingState,
					"tampercounters":                   m.TamperCounters,
					"asynchronouscounters":             m.AsynchronousCounters,
					"poweroutageflags":                 m.PowerOutageFlags,
					"lastconsumptioncount":             m.LastConsumptionCount,
					"differentialconsumptionintervals": m.DifferentialConsumptionIntervals,
					"transmittimeoffset":               m.TransmitTimeOffset,
					"serialnumbercrc":                  m.SerialNumberCRC,
					"packetcrc":                        m.PacketCRC,
				}
			case netidm.NetIDM:
				evt.Consumption = uint(m.LastConsumption)
				evt.MeterType = "netidm"
				evt.Fields = map[string]interface{}{
					"preamble":                         m.Preamble,
					"protocolid":                       m.ProtocolID,
					"packetlength":                     m.PacketLength,
					"hammingcode":                      m.HammingCode,
					"applicationversion":               m.ApplicationVersion,
					"erttype":                          m.ERTType,
					"ertserialnumber":                  m.ERTSerialNumber,
					"consumptionintervalcount":         m.ConsumptionIntervalCount,
					"programmingstate":                 m.ProgrammingState,
					"lastgeneration":                   m.LastGeneration,
					"lastconsumption":                  m.LastConsumption,
					"lastconsumptionnet":               m.LastConsumptionNet,
					"differentialconsumptionintervals": m.DifferentialConsumptionIntervals,
					"transmittimeoffset":               m.TransmitTimeOffset,
					"serialnumbercrc":                  m.SerialNumberCRC,
					"packetcrc":                        m.PacketCRC,
				}
			case r900.R900:
				evt.Consumption = uint(m.Consumption)
				evt.MeterType = "r900"
				evt.Fields = map[string]interface{}{
					"unkn1":    m.Unkn1,
					"nouse":    m.NoUse,
					"backflow": m.BackFlow,
					"unkn3":    m.Unkn3,
					"leak":     m.Leak,
					"leaknow":  m.LeakNow,
					"checksum": m.Checksum(),
				}
			case r900bcd.R900BCD:
				evt.Consumption = uint(m.Consumption)
				evt.MeterType = "r900bcd"
				evt.Fields = map[string]interface{}{
					"unkn1":    m.Unkn1,
					"nouse":    m.NoUse,
					"backflow": m.BackFlow,
					"unkn3":    m.Unkn3,
					"leak":     m.Leak,
					"leaknow":  m.LeakNow,
					"checksum": m.Checksum(),
				}
			}
			if err := db.AddEvent(&evt); err != nil {
				fmt.Println(err)
			}
		}
	}()

	log.Println("Starting ZPowerMon version", Version)

	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("content-type", "application/json")
			if err := json.NewEncoder(w).Encode(m.GetStatus()); err != nil {
				log.Printf("can't encode link: %+v", err)
			}
		})
		s := http.Server{
			Addr:              ":8080",
			Handler:           mux,
			ReadTimeout:       time.Second * 10,
			WriteTimeout:      time.Second * 10,
			MaxHeaderBytes:    256,
			ReadHeaderTimeout: 10 * time.Second,
		}
		log.Println("Starting REST status")
		if err := s.ListenAndServe(); err != nil {
			log.Printf("shutting down HTTP server %v", err)
		}
	}()

	if err := m.Run(msgChan); err != nil {
		log.Fatal(err)
	}

}
