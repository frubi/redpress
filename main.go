package main

import (
	"fmt"
	"net"
	"os"
	"time"

	"golang.org/x/net/icmp"
	"crypto/rand"
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

		peer.Payload = make([]byte, 16)
		rand.Read(peer.Payload)

		peers = append(peers, peer)
	}

	output, err := os.Create(fmt.Sprintf("redpress-%d.tsv", startupTime.Unix()))
	if err != nil {
		fmt.Println("Failed to create output file:", err)
		return
	}

	// Create "connection" to send/receivce packets
	conn, err := icmp.ListenPacket("udp4", "0.0.0.0")
	if err != nil {
		fmt.Println("Failed to listen for ICMP packets:", err)
		return
	}
	defer conn.Close()

	// Async reception of ICMP packets
	go recv(peers, conn)

	// Async transmission of ICMP packets
	go send(peers, conn, config.ProbeInterval)

	// Delay reporting by 3 seconds. Otherwise the reports will generate at the
	// same time as the last send.
	time.Sleep(3 * time.Second)

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