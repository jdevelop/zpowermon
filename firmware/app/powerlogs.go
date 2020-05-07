package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
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
	rtlTcpHost = "127.0.0.1"
	rtlTcpPort = "1234"
)

func main() {

	path := flag.String("path", "", "Path to the log file")
	host := flag.String("host", "localhost", "Hostname for InfluxDB")
	port := flag.Int("port", 8086, "Port of InfluxDB")

	flag.Parse()

	if *path == "" {
		flag.Usage()
		os.Exit(1)
	}

	db, err := database.Connect(*host, *port, "", "")
	if err != nil {
		log.Fatal(err)
	}

	c := make(chan os.Signal, 1)
	defer close(c)
	signal.Notify(c, os.Interrupt, os.Kill)
	go func() {
		cmd := exec.Command("rtl_tcp", "-a", rtlTcpHost, "-p", rtlTcpPort)
		go func(cmd *exec.Cmd) {
			<-c
			cmd.Process.Kill()
		}(cmd)
		if err := cmd.Run(); err != nil {
			log.Println(err)
		}
	}()

	time.Sleep(5 * time.Second)

	m, err := meter.NewMeter(rtlTcpHost + ":" + rtlTcpPort)
	if err != nil {
		log.Fatal(err)
	}
	defer m.Close()

	msgChan := make(chan protocol.Message)

	go func() {
		var consumption uint
		for msg := range msgChan {
			switch m := msg.(type) {
			case *scm.SCM:
				consumption = uint(m.Consumption)
			case *scmplus.SCM:
				consumption = uint(m.Consumption)
			case *idm.IDM:
				consumption = uint(m.LastConsumptionCount)
			case *netidm.NetIDM:
				consumption = uint(m.LastConsumption)
			case *r900.R900:
				consumption = uint(m.Consumption)
			case *r900bcd.R900BCD:
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

	if err := m.Run(msgChan); err != nil {
		close(c)
		log.Fatal(err)
	}

}
