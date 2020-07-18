package main

import (
	"time"

	"github.com/theodesp/blockingQueues"
)

var reduntQueue *blockingQueues.BlockingQueue

func reduntInit() {
	reduntQueue, _ = blockingQueues.NewArrayBlockingQueue(1000)
}

func reduntAdd(item *FBuffer) {
	_, err := reduntQueue.Put(item)
	checkError(err)
}

func reduntGet() *FBuffer {
	item, _ := reduntQueue.Get()
	packet := item.(*FBuffer)
	nowTs := time.Now().UnixNano()
	deltaTime := nowTs - packet.enQueueTS

	if deltaTime > packet.waitTime {
		return packet
	}
	time.Sleep(time.Duration(packet.waitTime-deltaTime) * time.Nanosecond)
	return packet
}
