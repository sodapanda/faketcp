package main

import (
	"sync"
)

//FBuffer data and len
type FBuffer struct {
	data      []byte
	len       int
	enQueueTS int64
	waitTime  int64
}

var bufPool = sync.Pool{
	New: func() interface{} {
		buffer := FBuffer{}
		buffer.data = make([]byte, 2000)
		return &buffer
	},
}

func poolGet() *FBuffer {
	return bufPool.Get().(*FBuffer)
}

func poolPut(item *FBuffer) {
	bufPool.Put(item)
}
