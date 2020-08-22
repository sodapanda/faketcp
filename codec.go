package main

import (
	"container/list"
	"encoding/binary"
	"math"

	"github.com/klauspost/reedsolomon"
)

type fecCodec struct {
	segCount            int
	fecSegCount         int
	encodeWorkspace     [][]byte
	decodeLinkMap       map[uint64][]*ftPacket
	keyList             *list.List
	decodeMapCapacity   int
	decodeTempWorkspace [][]byte
	encoder             reedsolomon.Encoder
	tmpPool             [][]byte //因为每次fec桶大小不一样
	currentID           uint64
	fullPacketHolder    []byte //分包合并然后拆分用的内存空间
}

func newFecCodec(segCount int, fecSegCount int, decodeMapCap int) *fecCodec {
	codec := new(fecCodec)
	codec.segCount = segCount
	codec.fecSegCount = fecSegCount
	codec.encodeWorkspace = make([][]byte, segCount+fecSegCount)
	codec.decodeLinkMap = make(map[uint64][]*ftPacket)
	codec.keyList = list.New()
	codec.decodeMapCapacity = decodeMapCap
	codec.decodeTempWorkspace = make([][]byte, segCount+fecSegCount)
	codec.encoder, _ = reedsolomon.New(segCount, fecSegCount)
	codec.tmpPool = make([][]byte, fecSegCount)
	for i := range codec.tmpPool {
		codec.tmpPool[i] = make([]byte, 2000)
	}

	codec.fullPacketHolder = make([]byte, 2000*segCount)

	return codec
}

func (codec *fecCodec) encode(data []byte, realLength int, ipInfo *FPacket, result []*FBuffer) {
	segSize := (len(data)) / codec.segCount
	for i := 0; i < codec.segCount; i++ {
		start := i * segSize
		end := start + segSize
		codec.encodeWorkspace[i] = data[start:end]
	}

	for i := 0; i < codec.fecSegCount; i++ {
		codec.encodeWorkspace[codec.segCount+i] = codec.tmpPool[i][:segSize]
	}

	codec.encoder.Encode(codec.encodeWorkspace)

	codec.currentID = codec.currentID + 1

	for i, data := range codec.encodeWorkspace {
		ftp := new(ftPacket)
		ftp.gID = codec.currentID
		ftp.index = uint16(i)
		ftp.realLength = uint16(realLength)
		ftp.data = data
		codeLen := ftp.encode(result[i].data[40:]) //把40长度的tcp ip留出来
		result[i].len = 40 + codeLen               //指示有效长度
	}

	for _, data := range result {
		craftPacket(data.data[:data.len], ipInfo)
	}
}

func (codec *fecCodec) decode(ftp *ftPacket, result []*FBuffer) bool {
	_, found := codec.decodeLinkMap[ftp.gID]
	if !found {
		codec.decodeLinkMap[ftp.gID] = make([]*ftPacket, codec.segCount+codec.fecSegCount)
		codec.keyList.PushBack(ftp.gID)
	}

	if len(codec.decodeLinkMap) >= codec.decodeMapCapacity {
		firstKeyElm := codec.keyList.Front()
		firstKey := firstKeyElm.Value.(uint64)
		ftps := codec.decodeLinkMap[firstKey]
		for _, ftp := range ftps {
			mFtPool.poolPut(ftp)
		}
		delete(codec.decodeLinkMap, firstKey)
		codec.keyList.Remove(firstKeyElm)
	}

	poolFtp := mFtPool.poolGet()
	poolFtp.len = len(ftp.data)
	poolFtp.gID = ftp.gID
	poolFtp.index = ftp.index
	poolFtp.realLength = ftp.realLength
	copy(poolFtp.data, ftp.data)

	row := codec.decodeLinkMap[ftp.gID]
	row[ftp.index] = poolFtp

	gotCount := 0
	for _, v := range row {
		if v != nil {
			gotCount++
		}
	}

	if gotCount != codec.segCount {
		return false
	}

	for i := range codec.decodeTempWorkspace {
		codec.decodeTempWorkspace[i] = nil
	}

	for i := range row {
		thisFtp := row[i]
		if thisFtp != nil {
			codec.decodeTempWorkspace[i] = thisFtp.data[:thisFtp.len]
		}
	}

	codec.encoder.Reconstruct(codec.decodeTempWorkspace)
	fCursor := 0
	for _, data := range codec.decodeTempWorkspace {
		copy(codec.fullPacketHolder[fCursor:], data)
		fCursor = fCursor + len(data)
	}

	fullData := codec.fullPacketHolder[:ftp.realLength]

	sCursor := 0
	for i := 0; i < codec.segCount; i++ {
		length := binary.BigEndian.Uint16(fullData[sCursor:])
		sCursor = sCursor + 2
		copy(result[i].data, fullData[sCursor:sCursor+int(length)])
		sCursor = sCursor + int(length)
		result[i].len = int(length)
	}

	return true
}

func (codec *fecCodec) align(length int) int {
	minBucket := math.Ceil(float64(length) / float64(codec.segCount))
	return int(minBucket) * codec.segCount
}
