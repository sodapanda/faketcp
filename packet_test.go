package main

import (
	"encoding/hex"
	"fmt"
	"net"
	"testing"
)

// func TestPacket(t *testing.T) {
// 	fPacket := FPacket{
// 		srcIP:   net.IP{10, 1, 1, 2}.To4(),
// 		dstIP:   net.IP{10, 1, 1, 3}.To4(),
// 		srcPort: 8888,
// 		dstPort: 12270,
// 		syn:     true,
// 		ack:     false,
// 		seqNum:  1,
// 		ackNum:  2,
// 	}

// 	packet := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 3, 4}
// 	len := craftPacket(packet, &fPacket)
// 	fmt.Println("len =", len)
// 	fmt.Println(hex.Dump(packet))
// }

func TestFec(t *testing.T) {
	fmt.Println("test FEC")
	fec := newFec(2, 1)
	data := make([]byte, fecInputStdLen)
	for i := range data {
		data[i] = byte(i)
	}

	result := make([]*FBuffer, 3)
	for i := range result {
		result[i] = poolGet()
	}

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

	fecRcv := newRecvCache(5)
	fec.encode(data, 7, &fPacket, result)
	for i, v := range result {
		pData := v.data[:v.len]
		// fmt.Println("len ", len(pData))
		// fmt.Println(hex.Dump(pData))

		if i != 1 {
			doRcv(pData, fec, fecRcv)
		}
	}
}

func doRcv(packet []byte, fec *rsFec, fecRcv *fecRecvCache) {
	subPkt := new(subPacket)
	unPackSub(packet[40:], subPkt)
	fmt.Println("subPkt ", subPkt.indexInRS)
	result := poolGet()
	done := fecRcv.append(subPkt, fec, result)
	if done {
		fmt.Println("done")
		fmt.Println(hex.Dump(result.data[:result.len]))
	} else {
		fmt.Println("not done")
	}
}
