package main

import (
	"encoding/hex"
	"fmt"
	"net"
	"testing"
)

func TestPacket(t *testing.T) {
	fPacket := FPacket{
		srcIP:   net.IP{10, 1, 1, 2}.To4(),
		dstIP:   net.IP{10, 1, 1, 3}.To4(),
		srcPort: 8888,
		dstPort: 12270,
		syn:     true,
		ack:     false,
		seqNum:  1,
		ackNum:  2,
	}

	packet := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 3, 4}
	len := craftPacket(packet, &fPacket)
	fmt.Println("len =", len)
	fmt.Println(hex.Dump(packet))
}
