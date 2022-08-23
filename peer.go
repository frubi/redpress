package main

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"

	"github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
)

type Peer struct {
	// Mutex used for locking this struct
	sync.Mutex

	// IP address of remote peer
	Addr net.IP
	// Sequence used for detection of delayed packages
	Seq uint16
	// Random payload used for identification
	Payload []byte
	// Time of the last transmit
	Transmit []time.Time

	// Last reset of packets numbers
	LastReset time.Time
	// Number of send packets
	Send int
	// Number of received in order packets
	RecvInOrder int
	// Number of received out of order packets
	RecvOutOfOrder int
}

type Peers []*Peer

// Find matching peer by IP address
func (peers Peers) Lookup(addr net.IP) *Peer {
	for _, peer := range peers {
		if peer.Addr.Equal(addr) {
			return peer
		}
	}

	return nil
}

func send(peers Peers, conn *icmp.PacketConn, interval int) {
	ticker := time.Tick(time.Duration(interval) * time.Second)

	for t := range ticker {
		now := t.Format("2006-01-02 15:04:05")

		for _, peer := range peers {
			// Safe update of send numbers
			peer.Lock()

			peer.Seq++
			seq := peer.Seq

			copy(peer.Transmit, peer.Transmit[1:])
			peer.Transmit[len(peer.Transmit)-1] = t

			peer.Send++
			peer.Unlock()

			// Construct ICMP message
			msg := icmp.Message{
				Type: ipv4.ICMPTypeEcho, Code: 0,
				Body: &icmp.Echo{
					ID: 0xCAFE, Seq: int(seq),
					Data: peer.Payload,
				},
			}

			// Convert ICMP struct to raw byte array
			pkt, err := msg.Marshal(nil)
			if err != nil {
				fmt.Println("Failed to create ICMP packet:", err)
				os.Exit(2)
			}

			fmt.Printf("%s -> Send %s\n", peer.Addr.String(), now)

			// Transmit ICMP package
			_, err = conn.WriteTo(pkt, &net.UDPAddr{IP: peer.Addr})
			if err != nil {
				fmt.Println("Failed to send ICMP packet:", err)
				os.Exit(2)
			}
		}
	}

}

func recv(peers Peers, conn *icmp.PacketConn, points chan *write.Point) {
	pkt := make([]byte, 1500)

	for {
		n, from, err := conn.ReadFrom(pkt)
		if err != nil {
			fmt.Println("Failed to receive ICMP packet:", err)
			os.Exit(1)
		}

		remoteName, _, err := net.SplitHostPort(from.String())
		if err != nil {
			fmt.Println("Failed to resolve remote addr:", err)
			os.Exit(1)
		}

		remoteAddr := net.ParseIP(remoteName)
		if remoteAddr == nil {
			fmt.Println("Failed to resolve remote addr: parse IP failed")
			os.Exit(1)
		}

		msg, err := icmp.ParseMessage(1, pkt[:n])
		if err != nil {
			fmt.Println("Failed to parse ICMP packet:", err)
			continue
		}

		// Received ICMP packet is not an echo reply
		if msg.Type != ipv4.ICMPTypeEchoReply {
			continue
		}

		// Cast message body to echo reply
		reply := msg.Body.(*icmp.Echo)

		// Find peer description for packet
		peer := peers.Lookup(remoteAddr)
		if peer == nil {
			continue
		}

		// Check payload for identification
		if bytes.Equal(reply.Data, peer.Payload) == false {
			continue
		}

		var tx time.Time

		peer.Lock()
		if peer.Seq == uint16(reply.Seq) {
			peer.RecvInOrder++
		} else {
			peer.RecvOutOfOrder++
		}

		delta := int(peer.Seq) - reply.Seq
		if delta < 0 {
			delta += 0x10001
		}
		if delta < len(peer.Transmit) {
			tx = peer.Transmit[len(peer.Transmit)-delta-1]

		}

		peer.Unlock()

		if tx.IsZero() == false {
			rx := time.Now()
			delay := rx.Sub(tx)

			p := influxdb2.NewPointWithMeasurement("rtt").
				AddTag("host", peer.Addr.String()).
				AddField("rtt", delay.Milliseconds()).
				SetTime(rx)
			points <- p
		}

		fmt.Printf("%s <- Receive\n", peer.Addr.String())
	}
}
