package main

import (
	"encoding/binary"
	"time"

	"github.com/google/netstack/tcpip/header"
	"github.com/klauspost/reedsolomon"
)

const (
	fecInputStdLen = 8
)

type rsFec struct {
	encoder reedsolomon.Encoder
}

//header 11 byte
type subPacket struct {
	data         []byte
	parentID     uint64
	indexInRS    uint16
	parentLength uint16
}

func newFec(dataCount int, rsCount int) *rsFec {
	fec := new(rsFec)
	enc, err := reedsolomon.New(dataCount, rsCount)
	checkError(err)
	fec.encoder = enc
	return fec
}

func (fec *rsFec) encode(parentPkt []byte, parentLen int, fPacket *FPacket, result []*FBuffer) {
	subPktLen := fecInputStdLen / 2
	calcBuf := make([][]byte, 3)
	calcBuf[0] = parentPkt[:subPktLen]
	calcBuf[1] = parentPkt[subPktLen:]
	calcBuf[2] = make([]byte, subPktLen)

	fec.encoder.Encode(calcBuf)

	parentID := uint64(time.Now().UnixNano())
	for i, v := range calcBuf {
		//数据复制到result里边，然后创建ip包
		subPkt := new(subPacket)
		subPkt.parentID = parentID
		subPkt.indexInRS = uint16(i)
		subPkt.parentLength = uint16(parentLen)
		subPkt.data = v
		craftSubPacket(subPkt, fPacket, result[i])
	}
}

func (fec *rsFec) decode(input [][]byte) {
	fec.encoder.Reconstruct(input)
}

func craftSubPacket(subp *subPacket, fPacket *FPacket, result *FBuffer) {
	iID := header.IPv4MinimumSize + header.TCPMinimumSize
	iRs := iID + 8
	iPLen := iRs + 2
	iPayload := iPLen + 2

	buff := result.data
	result.len = 40 + 12 + len(subp.data)
	binary.BigEndian.PutUint64(buff[iID:], subp.parentID)
	binary.BigEndian.PutUint16(buff[iRs:], subp.indexInRS)
	binary.BigEndian.PutUint16(buff[iPLen:], subp.parentLength)

	copy(buff[iPayload:], subp.data)
	craftPacket(buff, fPacket)
}
