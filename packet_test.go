package main

import (
	"encoding/hex"
	"fmt"
	"testing"
)

func TestPacket(t *testing.T) {
	fPacket := FPacket{
		srcIP:   "\x0a\x01\x01\x01",
		dstIP:   "\x0a\x01\x01\x02",
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
