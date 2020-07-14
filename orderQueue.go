package main

import (
	"container/list"
	"sync"
)

//OrderQueue with time out
type OrderQueue struct {
	dataList *list.List
	size     int
	lock     *sync.Mutex
	notEmpty *sync.Cond
	notFull  *sync.Cond
}

type indexer interface {
	index() int
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
func (q *OrderQueue) Put(item indexer) {
	q.lock.Lock()
	for q.dataList.Len() == q.size {
		q.notFull.Wait()
	}
	q.dataList.PushBack(item)
	q.lock.Unlock()
	q.notEmpty.Signal()
}

//Get get item with block
func (q *OrderQueue) Get() indexer {
	q.lock.Lock()
	for q.dataList.Len() == 0 {
		q.notEmpty.Wait()
	}
	item := q.dataList.Remove(q.dataList.Front())
	q.lock.Unlock()
	q.notFull.Signal()
	return item.(indexer)
}

//Len get len
func (q *OrderQueue) Len() int {
	q.lock.Lock()
	len := q.dataList.Len()
	q.lock.Unlock()
	return len
}
