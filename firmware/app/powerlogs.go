package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/hpcloud/tail"
	"github.com/jdevelop/zpowermon/firmware"
	"github.com/jdevelop/zpowermon/firmware/database"
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

	t, err := tail.TailFile(*path, tail.Config{Follow: true})
	if err != nil {
		log.Fatal(err)
	}

	pipeIn, pipeOut := io.Pipe()

	defer func() {
		pipeIn.Close()
		pipeOut.Close()
	}()

	go func() {
		firmware.ParseEventStream(pipeIn, func(evt *firmware.PowerEvent) error {
			fmt.Printf("%s :: %d = %d\n", evt.Timestamp.UTC().String(), evt.GetId(), evt.Message.Consumption)
			if err := db.AddEvent(evt); err != nil {
				fmt.Println(err)
			}
			return nil
		})
	}()

	for line := range t.Lines {
		pipeOut.Write([]byte(line.Text))
	}

}
