package main

import (
	"fmt"
	"net"
	"os"

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
var mSerSeq uint32
var reduntCounter int

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
	mSerSeq = 1000

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

func serverTunToSocket(tun *water.Interface) {
	fmt.Println("server tun to socket")
	buffer := make([]byte, 2000)

	for {
		len, err := tun.Read(buffer)
		if len == 0 {
			fmt.Println("server tun read len 0 ")
			os.Exit(1)
		}
		checkError(err)
		data := buffer[:len]
		_, err = serverConn.Write(data[header.IPv4MinimumSize+header.TCPMinimumSize:])
		serverReceiveCount++
		checkError(err)
	}
}

func serverSocketToQueueFEC(serverSendto string, srcPort int) {
	fmt.Println("server socket to queue with FEC")
	udpAddr, err := net.ResolveUDPAddr("udp4", serverSendto)
	checkError(err)
	conn, err := net.DialUDP("udp", nil, udpAddr)
	checkError(err)
	serverConn = conn
	fec := newFec(mSegCount, mFecCount)
	readBuf := make([]byte, fecInputStdLen)

	for {
		length, _ := serverConn.Read(readBuf[0:])
		fPacket := FPacket{
			srcIP:   net.IP{10, 1, 1, 2}.To4(),
			dstIP:   peerIP.To4(),
			srcPort: uint16(srcPort),
			dstPort: uint16(peerPort),
			syn:     false,
			ack:     true,
			seqNum:  mSerSeq + uint32(length),
			ackNum:  lastRecPacket.seqNum + uint32(len(lastRecPacket.payload)),
		}

		result := make([]*FBuffer, mSegCount+mFecCount)
		for i := range result {
			result[i] = poolGet()
		}

		fec.encode(readBuf[0:], length, &fPacket, result)

		for _, subBuf := range result {
			_, err := mServerQueue.Push(subBuf)
			if err != nil {
				serverDrop++
				println("server drop packet ", serverDrop)
			}
		}
	}
}

func serverSocketToQueueNoFEC(serverSendto string, srcPort int) {
	fmt.Println("server socket to queue")

	udpAddr, err := net.ResolveUDPAddr("udp4", serverSendto)
	checkError(err)
	conn, err := net.DialUDP("udp", nil, udpAddr)
	checkError(err)
	serverConn = conn

	for {
		fBuf := poolGet()
		length, _ := serverConn.Read(fBuf.data[(header.IPv4MinimumSize + header.TCPMinimumSize):])
		//这里不能改变pool里每个对象的slice大小，因为改小了的话，下一个包可能不够用
		fBuf.len = length + header.IPv4MinimumSize + header.TCPMinimumSize
		//在这里包装成IP包 入队列直接是IP包
		fPacket := FPacket{
			srcIP:   net.IP{10, 1, 1, 2}.To4(),
			dstIP:   peerIP.To4(),
			srcPort: uint16(srcPort),
			dstPort: uint16(peerPort),
			syn:     false,
			ack:     true,
			seqNum:  mSerSeq + uint32(length),
			ackNum:  lastRecPacket.seqNum + uint32(len(lastRecPacket.payload)),
		}
		craftPacket(fBuf.data[:fBuf.len], &fPacket)

		_, err := mServerQueue.Put(fBuf)

		if err != nil {
			serverDrop++
			println("server drop packet ", serverDrop)

			if serverDrop > 1000000 {
				serverDrop = 0
			}
			poolPut(fBuf)
		}
	}
}

func serverQueueToTun(tun *water.Interface) {
	for {
		item, _ := mServerQueue.Get()
		fBuf := item.(*FBuffer)
		data := fBuf.data[:fBuf.len]

		writeLen, err := tun.Write(data)
		serverSendCount++

		poolPut(fBuf)
		checkError(err)
		if writeLen != len(data) {
			fmt.Println("server tun write not full")
		}
	}
}
