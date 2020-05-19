package main

import (
	"fmt"
	"log"
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
		var consumption uint
		for msg := range msgChan {
			switch m := msg.(type) {
			case scm.SCM:
				consumption = uint(m.Consumption)
			case scmplus.SCM:
				consumption = uint(m.Consumption)
			case idm.IDM:
				consumption = uint(m.LastConsumptionCount)
			case netidm.NetIDM:
				consumption = uint(m.LastConsumption)
			case r900.R900:
				consumption = uint(m.Consumption)
			case r900bcd.R900BCD:
				consumption = uint(m.Consumption)
			}
			if err := db.AddEvent(&firmware.PowerEvent{
				Timestamp: time.Now(),
				Message: firmware.PowerMessage{
					EndpointID:  uint(msg.MeterID()),
					Consumption: uint(consumption),
				},
			}); err != nil {
				fmt.Println(err)
			}
		}
	}()

	log.Println("Starting ZPowerMon version", Version)

	if err := m.Run(msgChan); err != nil {
		log.Fatal(err)
	}

}
