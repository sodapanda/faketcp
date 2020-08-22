package main

import (
	"encoding/binary"
	"fmt"
	"sync"
	"time"
)

type stageBuffer struct {
	buffer   []byte
	capacity int
	size     int
	cursor   int
	lock     sync.Mutex
	sTimer   *time.Timer
	waitTime time.Duration
}

func newStageBuffer(cap int) *stageBuffer {
	sbuffer := new(stageBuffer)
	sbuffer.capacity = cap
	sbuffer.buffer = make([]byte, 2000*cap)
	sbuffer.size = 0
	sbuffer.cursor = 0
	sbuffer.waitTime = time.Duration(20) * time.Millisecond
	sbuffer.sTimer = time.NewTimer(sbuffer.waitTime)
	return sbuffer
}

func (sb *stageBuffer) append(data []byte, length uint16, callback func(*stageBuffer)) {
	if sb.size == 0 {
		sb.sTimer.Reset(sb.waitTime)
		go func() {
			<-sb.sTimer.C
			fmt.Println("time out!")
			sb.lock.Lock()
			defer sb.lock.Unlock()
			callback(sb)
			sb.cursor = 0
			sb.size = 0
		}()
	}
	sb.lock.Lock()
	defer sb.lock.Unlock()
	binary.BigEndian.PutUint16(sb.buffer[sb.cursor:], length)
	sb.cursor = sb.cursor + 2
	copy(sb.buffer[sb.cursor:], data)
	sb.cursor = sb.cursor + int(length)
	sb.size = sb.size + 1
	if sb.size == sb.capacity {
		sb.sTimer.Stop()
		callback(sb)
		sb.cursor = 0
		sb.size = 0
	}
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
