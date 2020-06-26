package main

import (
	"sync"
)

var poolCount int

//FBuffer data and len
type FBuffer struct {
	data []byte
	len  int
}

var bufPool = sync.Pool{
	New: func() interface{} {
		poolCount = poolCount + 1
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
