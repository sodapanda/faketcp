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

	payload := []byte{1, 2, 3, 4}
	result := make([]byte, 1500)
	len := craftPacket(payload, result, &fPacket)
	fmt.Println("len =", len)
	data := result[:len]
	fmt.Println(hex.Dump(data))
}
