package main

import (
	"fmt"
	"net"
	"os"
	"time"

	"crypto/rand"
	"golang.org/x/net/icmp"

	"github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage:", os.Args[0], "CONFIG")
		return
	}

	// Load the configuration from YAML file. The basic rules of the
	// configuation are checked within the function
	config, err := LoadConfig(os.Args[1])
	if err != nil {
		fmt.Println("Failed to load configuration:", err)
		return
	}

	// Get the local hostname to allow a better differentiation in the
	// recorded data
	source, err := os.Hostname()
	if err != nil {
		fmt.Println("Failed to get local hostname:", err)
		return
	}

	// Use startup time as first reset time
	startupTime := time.Now()

	// Prepare list of peers from configuration
	peers := make([]*Peer, 0)
	for _, host := range config.Hosts {
		peer := new(Peer)

		addr := net.ParseIP(host)
		if addr == nil {
			fmt.Println("Failed to parse IP address:", host)
			return
		}

		peer.Addr = addr
		peer.LastReset = startupTime
		peer.Transmit = make([]time.Time, 16)

		peer.Payload = make([]byte, 16)
		rand.Read(peer.Payload)

		peers = append(peers, peer)
	}

	output, err := os.Create(fmt.Sprintf("redpress-%d.tsv", startupTime.Unix()))
	if err != nil {
		fmt.Println("Failed to create output file:", err)
		return
	}

	points := make(chan *write.Point)

	if config.Influxdb != nil {
		client := influxdb2.NewClient(config.Influxdb.Url, config.Influxdb.Token)
		defer client.Close()

		write := client.WriteAPI(config.Influxdb.Org, config.Influxdb.Bucket)
		go func() {
			for p := range points {
				p.AddTag("source", source)
				write.WritePoint(p)
			}
		}()
	} else {
		go func() {
			for range points {
			}
		}()
	}

	// Create "connection" to send/receivce packets
	conn, err := icmp.ListenPacket("udp4", "0.0.0.0")
	if err != nil {
		fmt.Println("Failed to listen for ICMP packets:", err)
		return
	}
	defer conn.Close()

	fmt.Println("Redpress -- Startup complete :)")

	// Async reception of ICMP packets
	go recv(peers, conn, points)

	// Async transmission of ICMP packets
	go send(peers, conn, config.ProbeInterval)

	// Delay reporting by half the probe interval. Otherwise the reports will
	// generate at the same time as the last send.
	time.Sleep(time.Duration(config.ProbeInterval/2) * time.Second)

	ticker := time.Tick(time.Duration(config.ReportInterval) * time.Second)

	for t := range ticker {
		fmt.Println("Report at", t.Format("2006-01-02 15:04:05"))

		for _, peer := range peers {
			peer.Lock()

			lost := peer.Send - peer.RecvInOrder - peer.RecvOutOfOrder

			line := fmt.Sprintf("%q\t%d\t%d\t%d\t%d\t%d\t%d",
				peer.Addr.String(),
				peer.LastReset.Unix(), t.Unix(),
				peer.Send,
				peer.RecvInOrder, peer.RecvOutOfOrder,
				lost)

			p := influxdb2.NewPointWithMeasurement("reach").
				AddTag("host", peer.Addr.String()).
				AddField("send", peer.Send).
				AddField("recv", peer.RecvInOrder).
				AddField("delayed", peer.RecvOutOfOrder).
				AddField("lost", lost).
				SetTime(t)
			points <- p

			peer.Send = 0
			peer.RecvInOrder = 0
			peer.RecvOutOfOrder = 0

			peer.LastReset = t

			peer.Unlock()

			fmt.Println(" ", line)
			fmt.Fprintln(output, line)
		}
	}

}
