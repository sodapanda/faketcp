package main

import (
	"encoding/binary"
	"fmt"
	"time"

	"github.com/emirpasic/gods/maps/linkedhashmap"
	"github.com/google/netstack/tcpip/header"
	"github.com/klauspost/reedsolomon"
)

const (
	fecInputStdLen = 1400
)

type rsFec struct {
	encoder reedsolomon.Encoder
}

//header 12 byte
type subPacket struct {
	data         *FBuffer
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

//上层包传入时长度已经对其到标准长度，上层包的真实长度传入，打包tcp的参数,返回的结果是FBuffer的list
func (fec *rsFec) encode(parentPkt []byte, parentLen int, fPacket *FPacket, result []*FBuffer) {
	subPktLen := fecInputStdLen / mSegCount
	calcBuf := make([][]byte, mSegCount+mFecCount)
	//把传入的标准长度包切割成相等大小
	for i := 0; i < mSegCount; i++ {
		start := i * subPktLen
		end := start + subPktLen
		calcBuf[i] = parentPkt[start:end]
	}
	//给fec包分配空间
	for i := 0; i < mFecCount; i++ {
		calcBuf[mSegCount+i] = make([]byte, subPktLen)
	}

	fec.encoder.Encode(calcBuf)

	parentID := uint64(time.Now().UnixNano())
	for i, v := range calcBuf {
		//数据复制到result里边，然后创建ip包
		subPkt := new(subPacket)
		subPkt.parentID = parentID
		subPkt.indexInRS = uint16(i)
		subPkt.parentLength = uint16(parentLen)
		subPkt.data = &FBuffer{}
		subPkt.data.data = v
		subPkt.data.len = len(v)
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
	result.len = 40 + 12 + subp.data.len
	binary.BigEndian.PutUint64(buff[iID:], subp.parentID)
	binary.BigEndian.PutUint16(buff[iRs:], subp.indexInRS)
	binary.BigEndian.PutUint16(buff[iPLen:], subp.parentLength)

	copy(buff[iPayload:], subp.data.data[:subp.data.len])
	craftPacket(buff[:result.len], fPacket)
}

func unPackSub(data []byte, result *subPacket) {
	parentID := binary.BigEndian.Uint64(data[0:])
	rs := binary.BigEndian.Uint16(data[8:])
	pLen := binary.BigEndian.Uint16(data[8+2:])
	payLoad := data[12:]

	result.parentID = parentID
	result.indexInRS = rs
	result.parentLength = pLen
	result.data = poolGet()
	result.data.len = len(payLoad)
	copy(result.data.data, payLoad)
}

type fecRecvCache struct {
	linkMap *linkedhashmap.Map
	capLen  int
}

func newRecvCache(size int) *fecRecvCache {
	cache := new(fecRecvCache)
	cache.linkMap = linkedhashmap.New()
	cache.capLen = size

	return cache
}

var decodeCount int

func (fc *fecRecvCache) append(subPkt *subPacket, fec *rsFec, result *FBuffer) bool {
	//看看key是否存在，不存在的话创建，并且把[][]byte造好，为了等下解码
	//放进去看看够不够2个，够了看看是不是两个原始包，是的话直接合并，不是的话解码合并
	_, found := fc.linkMap.Get(subPkt.parentID)
	if !found {
		fc.linkMap.Put(subPkt.parentID, make([]*subPacket, mSegCount+mFecCount))
	}

	if fc.linkMap.Size() > fc.capLen {
		firstKey := fc.linkMap.Keys()[0]
		first, _ := fc.linkMap.Get(firstKey)
		firstP := first.([]*subPacket)
		for _, subp := range firstP {
			if subp != nil {
				poolPut(subp.data)
			}
		}
		fc.linkMap.Remove(firstKey)
	}

	group, _ := fc.linkMap.Get(subPkt.parentID)
	groupS := group.([]*subPacket)
	groupS[subPkt.indexInRS] = subPkt

	gotCount := 0

	for _, v := range groupS {
		if v != nil {
			gotCount++
		}
	}

	if gotCount >= mSegCount {
		tmp := make([][]byte, mSegCount+mFecCount)

		for i, subP := range groupS {
			if subP == nil {
				tmp[i] = nil
			} else {
				tmp[i] = subP.data.data[:subP.data.len]
			}
		}
		//现场解码
		fec.decode(tmp)
		decodeCount++
		//合并
		for i, v := range tmp[:mSegCount] {
			copy(result.data[i*subPkt.data.len:], v)
		}

		result.len = int(subPkt.parentLength)
		for _, subP := range groupS {
			if subP != nil {
				poolPut(subP.data)
			}
		}
		fc.linkMap.Remove(subPkt.parentID)
		return true
	}

	return false
}

func (fc *fecRecvCache) dump() {
	fc.linkMap.Each(func(key interface{}, value interface{}) {
		groupS := value.([]*subPacket)
		if groupS[0] != nil || groupS[1] != nil {
			fmt.Println("found real loss ", key)
		}
	})
}
