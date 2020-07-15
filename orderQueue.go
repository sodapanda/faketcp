package main

import (
	"container/list"
	"fmt"
	"sync"
	"time"
)

//OrderQueue with time out
type OrderQueue struct {
	dataList *list.List
	size     int
	lock     *sync.Mutex
	notEmpty *sync.Cond
	notFull  *sync.Cond
}

//NewOrderQueue create
func NewOrderQueue(size int) *OrderQueue {
	q := list.New()
	oq := new(OrderQueue)
	oq.dataList = q
	oq.size = size
	oq.lock = new(sync.Mutex)
	oq.notEmpty = sync.NewCond(oq.lock)
	oq.notFull = sync.NewCond(oq.lock)
	return oq
}

//Put add item with block
func (q *OrderQueue) Put(item *FBuffer) {
	item.enQueueTS = time.Now().UnixNano()
	item.waitTime = int64(enableRedunt) * int64(time.Millisecond)
	q.lock.Lock()
	for q.dataList.Len() == q.size {
		q.notFull.Wait()
	}
	//排序
	if q.dataList.Len() == 0 {
		q.dataList.PushBack(item)
	} else {
		var insertPoint *list.Element
		redunt := false
		for e := q.dataList.Front(); e != nil; e = e.Next() {
			qItem := e.Value.(*FBuffer)
			if qItem.id == item.id {
				redunt = true
				break
			}
			if qItem.id > item.id {
				insertPoint = e
				break
			}
		}

		if insertPoint != nil {
			q.dataList.InsertBefore(item, insertPoint)
		}
		if !redunt {
			q.dataList.PushBack(item)
		}
	}

	q.lock.Unlock()
	q.notEmpty.Signal()
}

//Get get item with block
func (q *OrderQueue) Get() *FBuffer {
	q.lock.Lock()
	for q.dataList.Len() == 0 {
		q.notEmpty.Wait()
	}

	first := q.dataList.Front().Value.(*FBuffer)
	if first == nil {
		fmt.Println("get nil")
	}
	q.lock.Unlock()

	//等待延迟时间，为了给队列里的数据包足够时间排序，等等那些乱序的包
	nowTs := time.Now().UnixNano()
	deltaTime := nowTs - first.enQueueTS
	if deltaTime > first.waitTime {
		q.lock.Lock()
		q.dataList.Remove(q.dataList.Front())
		q.lock.Unlock()
		q.notFull.Signal()
		return first
	}

	time.Sleep(time.Duration(first.waitTime-deltaTime) * time.Nanosecond)
	q.lock.Lock()
	q.dataList.Remove(q.dataList.Front())
	q.lock.Unlock()
	q.notFull.Signal()
	return first
}

//Len get len
func (q *OrderQueue) Len() int {
	q.lock.Lock()
	len := q.dataList.Len()
	q.lock.Unlock()
	return len
}
