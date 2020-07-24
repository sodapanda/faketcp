package main

import (
	"fmt"
	"net"
	"time"

	"github.com/google/netstack/tcpip/header"
	"github.com/songgao/water"
	"github.com/theodesp/blockingQueues"
)

var clientConnF *net.UDPConn
var mClientQueue *blockingQueues.BlockingQueue
var clientDrop int
var clientSendCount int
var clientReceiveCount int
var cLastRecPacket FPacket
var clientTunToSocketQueue *OrderQueue

var reduntCount int
var reorderCount int
var pushbackCount int
var timeoutCount int
var emptyPutCount int
var poolWrongFlag bool

func handShake(tun *water.Interface) {
	//发送Syn
	mClientQueue, _ = blockingQueues.NewArrayBlockingQueue(uint64(queueLen))
	clientTunToSocketQueue = NewOrderQueue(2000)

	srcIP := net.ParseIP(clientTunSrcIP)
	dstIP := net.ParseIP(clientTunDstIP)
	fmt.Printf("client sending SYN... %s %d \n", dstIP, clientTunDstPort)

	packet := make([]byte, 40)
	fPacket := FPacket{}
	fPacket.srcIP = srcIP.To4()
	fPacket.dstIP = dstIP.To4()
	fPacket.srcPort = uint16(clientTunSrcPort)
	fPacket.dstPort = uint16(clientTunDstPort)
	fPacket.syn = true
	fPacket.ack = false
	fPacket.seqNum = 1024 //初始化seq
	fPacket.ackNum = 0
	fPacket.payload = nil

	craftPacket(packet, &fPacket)
	_, err := tun.Write(packet)
	checkError(err)

	//接受syn+ack
	_, err = tun.Read(packet)

	cLastRecPacket = FPacket{}
	cLastRecPacket.srcIP = make([]byte, 4)
	cLastRecPacket.dstIP = make([]byte, 4)

	unpacket(packet, &cLastRecPacket)
	serverSeq := cLastRecPacket.seqNum
	serverAck := cLastRecPacket.ackNum
	fmt.Println("client got syn+ack")

	//发送ack
	fPacket.syn = false
	fPacket.ack = true
	fPacket.seqNum = serverAck
	fPacket.ackNum = serverSeq + 1

	craftPacket(packet, &fPacket)
	_, err = tun.Write(packet)
	checkError(err)
	fmt.Println("client send ack")
}

func clientTunToSocket(tun *water.Interface) {
	fmt.Println("client tun")
	buffer := make([]byte, 2000)

	for {
		n, err := tun.Read(buffer)
		checkError(err)
		data := buffer[:n]

		unpacket(data, &cLastRecPacket)
		startTs := time.Now().UnixNano()
		if enableLog {
			mSb.WriteString(fmt.Sprintf("%d\n", int(cLastRecPacket.ipID)))
		}
		_, err = clientConnF.Write(cLastRecPacket.payload)
		endTs := time.Now().UnixNano()
		if enableDebugLog {
			debugRecSb.WriteString(fmt.Sprintf("%d,%d,%d\n", startTs, endTs, endTs-startTs))
		}
		clientReceiveCount++
		checkError(err)
	}
}

func clientTunToQueue(tun *water.Interface) {
	fmt.Println("client tun to queue")

	for {
		fBuf := poolGet()
		readLen, err := tun.Read(fBuf.data[0:])
		fBuf.len = readLen
		fBuf.id = int(header.IPv4(fBuf.data[:header.IPv4MinimumSize]).ID())
		checkError(err)
		clientReceiveCount = clientReceiveCount + 1
		clientTunToSocketQueue.Put(fBuf)
	}
}

func clientQueueToSocket() {
	for {
		fBuf := clientTunToSocketQueue.Get()
		data := fBuf.data[:fBuf.len]
		unpacket(data, &cLastRecPacket)
		if enableLog {
			mSb.WriteString(fmt.Sprintf("%d\n", int(cLastRecPacket.ipID)))
		}
		_, err := clientConnF.Write(cLastRecPacket.payload)
		clientReceiveCount++
		checkError(err)
		poolPut(fBuf)
	}
}

func clientSocketToQueue(socketListenPort string, serverIP string, serverPort int) {
	fmt.Println("client socket to queue")

	udpAddr, err := net.ResolveUDPAddr("udp4", ":"+socketListenPort)
	conn, err := net.ListenUDP("udp", udpAddr)
	checkError(err)
	fmt.Printf("client listen socket %s\n", udpAddr)
	dstIP := net.ParseIP(serverIP).To4()
	clientConnF = nil
	for {
		fBuf := poolGet()
		lenU := 0
		if clientConnF == nil {
			var addr *net.UDPAddr
			lenU, addr, _ = conn.ReadFromUDP(fBuf.data[(header.IPv4MinimumSize + header.TCPMinimumSize):])
			conn.Close()
			clientConnF, _ = net.DialUDP("udp4", udpAddr, addr)
			fmt.Println("clientConnF Dial ", udpAddr, addr)
		} else {
			lenU, _ = clientConnF.Read(fBuf.data[(header.IPv4MinimumSize + header.TCPMinimumSize):])
		}

		fBuf.len = lenU + header.IPv4MinimumSize + header.TCPMinimumSize
		fBuf.debugTs = time.Now().UnixNano()

		packet := fBuf.data[:fBuf.len]
		fPacket := FPacket{}
		fPacket.srcIP = net.IP{10, 1, 1, 2}.To4()
		fPacket.dstIP = dstIP
		fPacket.srcPort = 8888
		fPacket.dstPort = uint16(serverPort)
		fPacket.seqNum = cLastRecPacket.ackNum
		if cLastRecPacket.syn {
			// syn 也算一个
			fPacket.ackNum = cLastRecPacket.seqNum + 1
		} else {
			fPacket.ackNum = cLastRecPacket.seqNum + uint32(len(cLastRecPacket.payload))
		}

		fPacket.syn = false
		fPacket.ack = true

		craftPacket(packet, &fPacket)

		if sendDelay > 0 {
			reFBuf := poolGet()
			reFBuf.len = fBuf.len
			copy(reFBuf.data, fBuf.data)
			reFBuf.enQueueTS = time.Now().UnixNano()
			reFBuf.waitTime = int64(sendDelay) * int64(time.Millisecond)
			reduntAdd(reFBuf)
		}

		_, err := mClientQueue.Push(fBuf)

		if err != nil {
			clientDrop++
			poolPut(fBuf)
			fmt.Println("client drop packet ", clientDrop)
		}
	}
}

func clientReduntWork(tun *water.Interface) {
	reduntInit()

	for {
		fBuf := reduntGet()
		data := fBuf.data[:fBuf.len]

		writeLen, err := tun.Write(data)
		serverSendCount++
		poolPut(fBuf)
		checkError(err)
		if writeLen != len(data) {
			fmt.Println("client tun write not full")
		}
	}
}

func clientQueueToTun(tun *water.Interface) {
	for {
		item, _ := mClientQueue.Get()
		fBuf := item.(*FBuffer)
		data := fBuf.data[:fBuf.len]

		writeLen, err := tun.Write(data)
		endTs := time.Now().UnixNano()
		if enableDebugLog {
			debugSendSb.WriteString(fmt.Sprintf("%d,%d,%d\n", fBuf.debugTs, endTs, endTs-fBuf.debugTs))
		}
		clientSendCount++
		poolPut(fBuf)
		checkError(err)
		if writeLen != len(data) {
			fmt.Println("client tun write not full")
		}
	}
}
