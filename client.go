package main

import (
	"fmt"
	"net"

	"github.com/google/netstack/tcpip/header"
	"github.com/songgao/water"
	"github.com/theodesp/blockingQueues"
)

var clientConn *net.UDPConn
var clientUDPAddr *net.UDPAddr
var mClientQueue *blockingQueues.BlockingQueue
var clientDrop int
var clientSendCount int
var clientReceiveCount int
var cLastRecPacket FPacket
var clientTunToSocketQueue *OrderQueue

func handShake(tun *water.Interface) {
	//发送Syn
	mClientQueue, _ = blockingQueues.NewArrayBlockingQueue(uint64(queueLen))
	clientTunToSocketQueue = NewOrderQueue(200)

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

func clientTunToQueue(tun *water.Interface) {
	fmt.Println("client tun")

	for {
		fBuf := poolGet()
		readLen, err := tun.Read(fBuf.data)
		fBuf.len = readLen
		fBuf.id = int(header.IPv4(fBuf.data).ID())
		checkError(err)
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
		_, err := clientConn.WriteToUDP(cLastRecPacket.payload, clientUDPAddr)
		clientReceiveCount++
		checkError(err)
		poolPut(fBuf)
	}
}

func clientSocketToQueue(socketListenPort string) {
	fmt.Println("client socket to queue")

	udpAddr, err := net.ResolveUDPAddr("udp4", ":"+socketListenPort)
	checkError(err)
	conn, err := net.ListenUDP("udp", udpAddr)
	checkError(err)
	fmt.Printf("client listen socket %s\n", udpAddr)
	clientConn = conn

	for {
		fBuf := poolGet()
		len, addr, _ := conn.ReadFromUDP(fBuf.data[(header.IPv4MinimumSize + header.TCPMinimumSize):])
		clientUDPAddr = addr

		fBuf.len = len + header.IPv4MinimumSize + header.TCPMinimumSize
		_, err := mClientQueue.Put(fBuf)
		if err != nil {
			clientDrop++
			if clientDrop > 1000000 {
				clientDrop = 0
			}
			poolPut(fBuf)
		}
	}
}

func clientQueueToTun(tun *water.Interface, serverIP string, serverPort int) {
	for {
		item, _ := mClientQueue.Get()
		fBuf := item.(*FBuffer)
		packet := fBuf.data[:fBuf.len]

		fPacket := FPacket{}
		fPacket.srcIP = net.IP{10, 1, 1, 2}.To4()
		fPacket.dstIP = net.ParseIP(serverIP).To4()
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

		writeLen, err := tun.Write(packet)
		clientSendCount++
		poolPut(fBuf)
		checkError(err)
		if writeLen != len(packet) {
			fmt.Println("client tun write not full")
		}
	}
}
