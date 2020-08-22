package main

import "encoding/binary"

type stageBuffer struct {
	buffer   []byte
	capacity int
	size     int
	cursor   int
}

func newStageBuffer(cap int) *stageBuffer {
	sbuffer := new(stageBuffer)
	sbuffer.capacity = cap
	sbuffer.buffer = make([]byte, 2000)
	sbuffer.size = 0
	sbuffer.cursor = 0
	return sbuffer
}

func (sb *stageBuffer) append(data []byte, length uint16) bool {
	binary.BigEndian.PutUint16(sb.buffer[sb.cursor:], length)
	sb.cursor = sb.cursor + 2
	copy(sb.buffer[sb.cursor:], data)
	sb.cursor = sb.cursor + int(length)
	sb.size = sb.size + 1
	if sb.size == sb.capacity {
		return true
	}
	return false
}

func (sb *stageBuffer) length() int {
	return sb.cursor
}

func (sb *stageBuffer) getFullData(result []byte) {
	copy(result, sb.buffer[:sb.cursor])
	//取出来之后重置一下
	sb.cursor = 0
	sb.size = 0
}
