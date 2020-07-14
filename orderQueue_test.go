package main

import (
	"fmt"
	"testing"
	"time"
)

func TestQueue(t *testing.T) {
	fmt.Println("start test")
	q := NewOrderQueue(5)
	go func() {
		for i := 0; i < 7; i++ {
			item := &FBuffer{
				id: i,
			}
			q.Put(item)
			fmt.Println("put item ", i, " len is ", q.Len())
			time.Sleep(1 * time.Second)
		}
	}()

	go func() {
		time.Sleep(10 * time.Second)
		for i := 0; i < 20; i++ {
			item := q.Get()
			fp := item.(*FBuffer)

			fmt.Println("get item ", fp.id, " len ", q.Len())
			time.Sleep(1 * time.Second)
		}
	}()

	time.Sleep(100 * time.Second)
}
