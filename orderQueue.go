package main

import (
	"container/list"
	"fmt"
	"os"
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
	item.waitTime = int64(recDelay) * int64(time.Millisecond)
	q.lock.Lock()
	for q.dataList.Len() == q.size {
		q.notFull.Wait()
	}

	//队列空，直接放进去
	if q.dataList.Len() == 0 {
		emptyPutCount = emptyPutCount + 1
		q.dataList.PushBack(item)

		q.lock.Unlock()
		q.notEmpty.Signal()
		return
	}

	//找到重复的包
	for e := q.dataList.Front(); e != nil; e = e.Next() {
		thisItem := e.Value.(*FBuffer)
		if &(item.data) == &(thisItem.data) {
			fmt.Println("same address")
			os.Exit(1)
		}
		if item.id == thisItem.id {
			reduntCount = reduntCount + 1
			poolPut(item)
			q.lock.Unlock()
			return
		}
	}

	//包排序，或者直接放到最后
	var insertMark *list.Element
	insertMark = nil
	for e := q.dataList.Front(); e != nil; e = e.Next() {
		thisItem := e.Value.(*FBuffer)
		if thisItem.id > item.id {
			insertMark = e
			break
		}
	}
	if insertMark != nil {
		q.dataList.InsertBefore(item, insertMark)
		reorderCount = reorderCount + 1
	} else {
		q.dataList.PushBack(item)
		pushbackCount = pushbackCount + 1
	}

	q.lock.Unlock()
}

//Get get item with block
func (q *OrderQueue) Get() *FBuffer {
	q.lock.Lock()
	for q.dataList.Len() == 0 {
		q.notEmpty.Wait()
	}

	firstE := q.dataList.Front()
	first := firstE.Value.(*FBuffer)

	//等待延迟时间，为了给队列里的数据包足够时间排序，等等那些乱序的包
	nowTs := time.Now().UnixNano()
	deltaTime := nowTs - first.enQueueTS
	if deltaTime > first.waitTime {
		timeoutCount = timeoutCount + 1
		q.dataList.Remove(firstE)
		q.lock.Unlock()
		q.notFull.Signal()
		return first
	}

	//先解锁，不然队列放不进东西去
	q.lock.Unlock()

	//这时候front可能已经不是刚才那个了，怎么办
	time.Sleep(time.Duration(first.waitTime-deltaTime) * time.Nanosecond)
	q.lock.Lock()
	q.dataList.Remove(firstE)
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
