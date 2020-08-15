package main

import (
	"encoding/hex"
	"fmt"
	"net"
	"testing"

	"github.com/emirpasic/gods/maps/linkedhashmap"
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
	eFec = true
	mSegCount = 2
	mFecCount = 1
	var paySize = 53
	fec := newFec(mSegCount, mFecCount)
	data := make([]byte, 2000)
	alignSize := minAlignSize(paySize, mSegCount)
	for i := range data {
		data[i] = byte(i)
	}

	result := make([]*FBuffer, mSegCount+mFecCount)
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
	fec.encode(data[:alignSize], paySize, &fPacket, result)
	fmt.Println("encode:")
	for _, v := range result {
		pData := v.data[:v.len]
		fmt.Println("part Data")
		fmt.Println(hex.Dump(pData))
	}

	fmt.Println("decode:")
	for i, fBuf := range result {
		pData := fBuf.data[:fBuf.len]
		if i != len(result)-1 {
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

func TestLink(t *testing.T) {
	linkMap := linkedhashmap.New()
	linkMap.Put("1", "a")
	linkMap.Put("2", "b")
	fmt.Println(linkMap)

	keys := linkMap.Keys()
	fmt.Println("key ", keys[0])
}
