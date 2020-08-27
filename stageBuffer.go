package main

import (
	"encoding/binary"
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
	sbuffer.waitTime = time.Duration(mConfig.StageTimeout) * time.Millisecond
	return sbuffer
}

func (sb *stageBuffer) append(data []byte, length uint16, resultBuffer []byte, codec *fecCodec, callback func(*stageBuffer, []byte, int)) {
	sb.lock.Lock()
	defer sb.lock.Unlock()

	if sb.size == 0 {
		sb.sTimer = time.NewTimer(sb.waitTime)
		go func() {
			<-sb.sTimer.C
			timeOutCount++
			sb.lock.Lock()
			defer sb.lock.Unlock()
			alignLen := codec.align(sb.cursor)
			copy(resultBuffer, sb.buffer[:sb.cursor])
			callback(sb, resultBuffer[:alignLen], sb.cursor)
			sb.cursor = 0
			sb.size = 0
		}()
	}

	binary.BigEndian.PutUint16(sb.buffer[sb.cursor:], length)
	sb.cursor = sb.cursor + 2
	copy(sb.buffer[sb.cursor:], data)
	sb.cursor = sb.cursor + int(length)
	sb.size = sb.size + 1
	if sb.size == sb.capacity {
		sb.sTimer.Stop()
		alignLen := codec.align(sb.cursor)
		copy(resultBuffer, sb.buffer[:sb.cursor])
		callback(sb, resultBuffer[:alignLen], sb.cursor)
		sb.cursor = 0
		sb.size = 0
	}
}
