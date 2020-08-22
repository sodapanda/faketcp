package main

import (
	"encoding/hex"
	"fmt"
	"net"
	"testing"
	"time"
)

func TestEncode(t *testing.T) {
	readLength1 := 7
	readLength2 := 6
	udpData1 := make([]byte, readLength1)
	for i := range udpData1 {
		udpData1[i] = byte(i)
	}
	udpData2 := make([]byte, readLength2)
	for i := range udpData2 {
		udpData2[i] = byte(i)
	}
	sb := newStageBuffer(2)

	codec := newFecCodec(2, 1, 200)

	for i := 0; i < 2; i++ {
		time.Sleep(time.Duration(1) * time.Second)
		var udpData []byte
		if i == 0 {
			udpData = udpData1
		} else {
			udpData = udpData2
		}
		sb.append(udpData, uint16(len(udpData)), func(sb *stageBuffer) {
			realLen := sb.length()
			alignSize := codec.align(realLen)
			fmt.Println("align size", alignSize)
			fullData := make([]byte, alignSize)
			sb.getFullData(fullData)

			fmt.Println(hex.Dump(fullData))

			encodeResult := make([]*FBuffer, 3)
			for i := range encodeResult {
				encodeResult[i] = &FBuffer{}
				encodeResult[i].data = make([]byte, 1000)
			}

			ipInfo := &FPacket{}
			ipInfo.srcIP = net.IP{10, 0, 0, 1}.To4()
			ipInfo.dstIP = net.IP{192, 168, 8, 1}.To4()
			ipInfo.srcPort = 8888
			ipInfo.dstPort = 8888
			ipInfo.seqNum = 1

			codec.encode(fullData, realLen, ipInfo, encodeResult)

			for _, d := range encodeResult {
				fmt.Println(hex.Dump(d.data[:d.len]))
			}

			//decode
			decodeResult := make([]*FBuffer, 2)
			for i := range decodeResult {
				decodeResult[i] = new(FBuffer)
				decodeResult[i].data = make([]byte, 2000)
			}

			for _, encodedPkt := range encodeResult {
				rcvPkt := new(ftPacket)
				rcvPkt.decode(encodedPkt.data[40:encodedPkt.len])

				done := codec.decode(rcvPkt, decodeResult)
				if done {
					break
				}
			}

			fmt.Println("decode")
			for _, d := range decodeResult {
				fmt.Println(hex.Dump(d.data[:d.len]))
			}
		})
	}

	time.Sleep(time.Duration(10) * time.Second)
}
