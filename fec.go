package main

import (
	"container/list"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/google/netstack/tcpip/header"
	"github.com/klauspost/reedsolomon"
)

type rsFec struct {
	encoder reedsolomon.Encoder
}

//header 14 byte
type subPacket struct {
	data         *FBuffer
	parentID     uint64
	indexInRS    uint16
	parentLength uint16
	dataType     uint16 //1是fec包 2是非fec包
}

func newFec(dataCount int, rsCount int) *rsFec {
	fec := new(rsFec)
	enc, err := reedsolomon.New(dataCount, rsCount)
	checkError(err)
	fec.encoder = enc
	return fec
}

//上层包传入时长度已经对其到标准长度，上层包的真实长度传入，打包tcp的参数,返回的结果是FBuffer的list
func (fec *rsFec) encode(parentPkt []byte, parentLen int, segCount int, fecCount int, fPacket *FPacket, result []*FBuffer) {
	subPktLen := len(parentPkt) / segCount
	calcBuf := make([][]byte, segCount+fecCount)
	//把传入的标准长度包切割成相等大小
	for i := 0; i < segCount; i++ {
		start := i * subPktLen
		end := start + subPktLen
		calcBuf[i] = parentPkt[start:end]
	}
	//给fec包分配空间
	for i := 0; i < fecCount; i++ {
		calcBuf[segCount+i] = make([]byte, subPktLen)
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
		subPkt.dataType = 1
		craftSubPacket(subPkt, fPacket, result[i])
	}
}

//小数据不用fec直接复制 长度不用限制成标准长度
func (fec *rsFec) encodeSmallPkt(parentPkt []byte, fPacket *FPacket, result []*FBuffer) {
	parentID := uint64(time.Now().UnixNano())
	for i := 0; i < 2; i++ {
		subPkt := new(subPacket)
		subPkt.parentID = parentID
		subPkt.indexInRS = uint16(i) //没必要
		subPkt.parentLength = uint16(len(parentPkt))
		subPkt.data = &FBuffer{}
		subPkt.data.data = parentPkt
		subPkt.data.len = len(parentPkt)
		subPkt.dataType = 2 //不用fec
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
	iDataType := iPLen + 2
	iPayload := iDataType + 2

	buff := result.data
	result.len = 40 + 14 + subp.data.len
	binary.BigEndian.PutUint64(buff[iID:], subp.parentID)
	binary.BigEndian.PutUint16(buff[iRs:], subp.indexInRS)
	binary.BigEndian.PutUint16(buff[iPLen:], subp.parentLength)
	binary.BigEndian.PutUint16(buff[iDataType:], subp.dataType)

	copy(buff[iPayload:], subp.data.data[:subp.data.len])
	craftPacket(buff[:result.len], fPacket)
}

func unPackSub(data []byte, result *subPacket) {
	parentID := binary.BigEndian.Uint64(data[0:])
	rs := binary.BigEndian.Uint16(data[8:])
	pLen := binary.BigEndian.Uint16(data[10:])
	pDataType := binary.BigEndian.Uint16(data[12:])
	payLoad := data[14:]

	result.parentID = parentID
	result.indexInRS = rs
	result.parentLength = pLen
	result.dataType = pDataType
	result.data = &FBuffer{}
	result.data.data = make([]byte, 2000)
	result.data.len = len(payLoad)
	copy(result.data.data, payLoad)
}

type fecRecvCache struct {
	linkMap map[uint64][]*subPacket
	keyList *list.List
	capLen  int
}

func newRecvCache(size int) *fecRecvCache {
	cache := new(fecRecvCache)
	cache.linkMap = make(map[uint64][]*subPacket)
	cache.keyList = list.New()
	cache.capLen = size

	return cache
}

var decodeCount int
var startRmvKey uint64

func (fc *fecRecvCache) append(subPkt *subPacket, fec *rsFec, segCount int, fecCount int, result *FBuffer) bool {
	//看看key是否存在，不存在的话创建，并且把[][]byte造好，为了等下解码
	//放进去看看够不够2个，够了看看是不是两个原始包，是的话直接合并，不是的话解码合并
	_, found := fc.linkMap[subPkt.parentID]
	if !found {
		fc.linkMap[subPkt.parentID] = make([]*subPacket, segCount+fecCount)
		fc.keyList.PushBack(subPkt.parentID)
	}

	if len(fc.linkMap) >= fc.capLen {
		firstKeyElm := fc.keyList.Front()
		if startRmvKey == 0 {
			startRmvKey = firstKeyElm.Value.(uint64)
		}
		delete(fc.linkMap, firstKeyElm.Value.(uint64))
		fc.keyList.Remove(firstKeyElm)
	}

	group, _ := fc.linkMap[subPkt.parentID]
	group[subPkt.indexInRS] = subPkt

	gotCount := 0

	for _, v := range group {
		if v != nil {
			gotCount++
		}
	}

	if gotCount == segCount {
		tmp := make([][]byte, segCount+fecCount)

		for i, subP := range group {
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
		for i, v := range tmp[:segCount] {
			copy(result.data[i*subPkt.data.len:], v)
		}

		result.len = int(subPkt.parentLength)
		for _, subP := range group {
			if subP != nil {
				poolPut(subP.data)
			}
		}
		return true
	}

	return false
}

func (fc *fecRecvCache) appendSmall(subPkt *subPacket, result *FBuffer) bool {
	// _, found := fc.linkMap[subPkt.parentID]
	// if !found {
	// 	fc.linkMap[subPkt.parentID]= make([]*subPacket, mSegCount+mFecCount)
	// }

	// if len(fc.linkMap) > fc.capLen {
	// 	firstKey := fc.linkMap.Keys()[0]
	// 	fc.linkMap.Remove(firstKey)
	// }

	// group, _ := fc.linkMap.Get(subPkt.parentID)
	// groupS := group.([]*subPacket)
	// groupS[subPkt.indexInRS] = subPkt

	// gotCount := 0

	// for _, v := range groupS {
	// 	if v != nil {
	// 		gotCount++
	// 	}
	// }

	// if gotCount == 1 {
	// 	copy(result.data, subPkt.data.data[:subPkt.data.len])
	// }
	// result.len = int(subPkt.parentLength)
	// poolPut(subPkt.data)
	// if gotCount == 1 {
	// 	return true
	// }
	// return false
	return false
}

func (fc *fecRecvCache) dump() {
	inCompleteCount := 0
	for e := fc.keyList.Front(); e != nil; e = e.Next() {
		subPkts := fc.linkMap[e.Value.(uint64)]
		gotCount := 0
		for i := range subPkts {
			if subPkts[i] == nil {
				fmt.Printf("❌")
			} else {
				gotCount++
				fmt.Print("✅")
			}
		}
		if gotCount < mClientSegCount {
			inCompleteCount++
		}
		fmt.Println("")
	}
	fmt.Println("start remove at ", startRmvKey)
	fmt.Println("incomplete count", inCompleteCount)
}
