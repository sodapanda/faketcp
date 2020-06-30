package main

import (
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/google/netstack/tcpip/header"
	"github.com/songgao/water"
	"github.com/theodesp/blockingQueues"
)

var serverConn *net.UDPConn
var peerIP net.IP
var peerPort uint16
var mServerQueue *blockingQueues.BlockingQueue
var serverDrop int
var serverSendCount int
var serverReceiveCount int
var lastRecPacket FPacket
var lastRecPacketLock sync.Mutex

func serverHandShake(tun *water.Interface) {
	mServerQueue, _ = blockingQueues.NewArrayBlockingQueue(uint64(queueLen))
	//等待syn
	fmt.Println("server waiting for SYN")
	packet := make([]byte, 40)
	_, err := tun.Read(packet)
	checkError(err)
	fPacket := FPacket{}
	fPacket.srcIP = make([]byte, 4)
	fPacket.dstIP = make([]byte, 4)
	unpacket(packet, &fPacket)
	if !fPacket.syn {
		fmt.Println("server get first packet not SYN!")
		os.Exit(1)
	}
	peerIP = make([]byte, 4)
	copy(peerIP, fPacket.srcIP)
	peerPort = fPacket.srcPort
	clientSeq := fPacket.seqNum

	//返回syn+ack
	fmt.Println("server send syn+ack")
	fPacket = FPacket{}
	fPacket.srcIP = net.ParseIP(clientTunSrcIP).To4()
	fPacket.dstIP = make([]byte, 4)
	copy(fPacket.dstIP, peerIP)
	fPacket.srcPort = uint16(serverTunSrcPort)
	fPacket.dstPort = peerPort
	fPacket.syn = true
	fPacket.ack = true
	fPacket.seqNum = 1000 //首次发送syn
	fPacket.ackNum = clientSeq + 1
	fPacket.payload = nil

	craftPacket(packet, &fPacket)
	_, err = tun.Write(packet)
	checkError(err)

	//等待ack
	_, err = tun.Read(packet)
	lastRecPacket = FPacket{}
	lastRecPacket.srcIP = make([]byte, 4)
	lastRecPacket.dstIP = make([]byte, 4)
	unpacket(packet, &fPacket)
	if !fPacket.ack {
		fmt.Println("server get not ack")
		os.Exit(1)
	}
	fmt.Println("server got ack hand shake done")
}

var serverTunToSocketReadMaxLen int

func serverTunToSocket(tun *water.Interface) {
	fmt.Println("server tun to socket")
	buffer := make([]byte, 2000)

	for {
		len, err := tun.Read(buffer)
		if len > serverTunToSocketReadMaxLen {
			serverTunToSocketReadMaxLen = len
		}
		checkError(err)
		data := buffer[:len]
		lastRecPacketLock.Lock()
		unpacket(data, &lastRecPacket)
		lastRecPacketLock.Unlock()
		_, err = serverConn.Write(lastRecPacket.payload)
		serverReceiveCount++
		checkError(err)
	}
}

var serverSocketReadMaxLen int

func serverSocketToQueue(serverSendto string, srcPort int) {
	fmt.Println("server socket to queue")

	udpAddr, err := net.ResolveUDPAddr("udp4", serverSendto)
	checkError(err)
	conn, err := net.DialUDP("udp", nil, udpAddr)
	checkError(err)
	serverConn = conn

	for {
		fBuf := poolGet()
		length, _ := serverConn.Read(fBuf.data[(header.IPv4MinimumSize + header.TCPMinimumSize):])
		if length > serverSocketReadMaxLen {
			serverSocketReadMaxLen = length
		}
		//这里不能改变pool里每个对象的slice大小，因为改小了的话，下一个包可能不够用
		fBuf.len = length + header.IPv4MinimumSize + header.TCPMinimumSize
		//在这里包装成IP包 入队列直接是IP包
		lastRecPacketLock.Lock()
		fPacket := FPacket{
			srcIP:   net.IP{10, 1, 1, 2}.To4(),
			dstIP:   peerIP.To4(),
			srcPort: uint16(srcPort),
			dstPort: uint16(peerPort),
			syn:     false,
			ack:     true,
			seqNum:  lastRecPacket.ackNum,
			ackNum:  lastRecPacket.seqNum + uint32(len(lastRecPacket.payload)),
		}
		lastRecPacketLock.Unlock()
		craftPacket(fBuf.data[:fBuf.len], &fPacket)

		_, err := mServerQueue.Push(fBuf)
		if err != nil {
			serverDrop++
			println("server drop packet ", serverDrop)

			if serverDrop > 1000000 {
				serverDrop = 0
			}
			poolPut(fBuf)
			//出现丢包就等一等
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func serverQueueToTun(tun *water.Interface) {
	for {
		item, _ := mServerQueue.Get()
		fBuf := item.(*FBuffer)
		data := fBuf.data[:fBuf.len]

		_, err := tun.Write(data)
		serverSendCount++
		poolPut(fBuf)
		checkError(err)
	}
}
