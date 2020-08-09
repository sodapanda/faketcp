package main

import (
	"sync"
)

//FBuffer data and len
type FBuffer struct {
	data []byte
	len  int
	id   int
}

var bufPool = sync.Pool{
	New: func() interface{} {
		buffer := FBuffer{}
		buffer.data = make([]byte, 1400)
		return &buffer
	},
}

func poolGet() *FBuffer {
	return bufPool.Get().(*FBuffer)
}

func poolPut(item *FBuffer) {
	bufPool.Put(item)
}
