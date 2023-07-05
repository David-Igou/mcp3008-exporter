package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"periph.io/x/conn/v3/physic"
	"periph.io/x/conn/v3/spi"
	"periph.io/x/conn/v3/spi/spireg"
	"periph.io/x/host/v3"
)

var (
	spiPort = flag.String("port", "", "SPI port to use")
)

//main

func main() {
	flag.Parse()
	if _, err := host.Init(); err != nil {
		log.Fatal(err)
	}
	p, err := spireg.Open(*spiPort)
	if err != nil {
		log.Fatal(err)
	}
	defer p.Close()

	if err := p.LimitSpeed(1 * physic.MegaHertz); err != nil {
		log.Fatal(err)
	}

	c, err := p.Connect(1*physic.MegaHertz, spi.Mode0, 8)
	if err != nil {
		log.Fatal(err)
	}

	collector := newMcp3008Collector(c)

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	prometheus.MustRegister(collector)
	http.Handle("/metrics", promhttp.Handler())

	go func() {
		log.Println("Server is starting...")
		err := http.ListenAndServe(":8088", nil)
		if err != nil {
			log.Fatalf("Server stopped with error: %v", err)
		}
	}()

	// Block the main goroutine until an interrupt signal is received.
	<-interrupt
	log.Println("Received interrupt signal, exiting...")
}

//Collector

type collector struct {
	c     spi.Conn
	descs [8]*prometheus.Desc
}

func newMcp3008Collector(c spi.Conn) *collector {
	var descs [8]*prometheus.Desc
	for i := 0; i < 8; i++ {
		descs[i] = prometheus.NewDesc(
			fmt.Sprintf("mcp3008_channel_%d", i),
			fmt.Sprintf("Current value of the MCP3008 channel %d", i),
			nil, nil,
		)
	}
	return &collector{c: c, descs: descs}
}

func (c *collector) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range c.descs {
		ch <- desc
	}
}

func (c *collector) Collect(ch chan<- prometheus.Metric) {
	values, err := readAllChannels(c.c)
	if err != nil {
		log.Println("Failed to read MCP3008 channels:", err)
		return
	}
	for i, value := range values {
		ch <- prometheus.MustNewConstMetric(c.descs[i], prometheus.GaugeValue, float64(value))
	}
}

//Hardware stuff

func readMCP3008(c spi.Conn, channel int) (int, error) {
	tx := []byte{1, byte((8 + channel) << 4), 0}
	rx := make([]byte, 3)
	if err := c.Tx(tx, rx); err != nil {
		return -1, err
	}
	data := (int(rx[1])<<8 | int(rx[2])) & 0x3FF
	return data, nil
}

func readAllChannels(c spi.Conn) ([]int, error) {
	values := make([]int, 8)
	for channel := 0; channel < 8; channel++ {
		data, err := readMCP3008(c, channel)
		if err != nil {
			return nil, err
		}
		values[channel] = data
	}
	return values, nil
}
