package main

import (
	"fmt"
	"net"
	"sync"

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
var cLastRecPacketLock sync.Mutex

func handShake(tun *water.Interface) {
	//发送Syn
	mClientQueue, _ = blockingQueues.NewArrayBlockingQueue(uint64(queueLen))

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
	// var lastID uint16

	for {
		n, err := tun.Read(buffer)
		checkError(err)
		data := buffer[:n]

		cLastRecPacketLock.Lock()
		unpacket(data, &cLastRecPacket)
		if enableLog {
			mSb.WriteString(fmt.Sprintf("%d\n", int(cLastRecPacket.ipID)))
		}
		cLastRecPacketLock.Unlock()
		// if lastID != cLastRecPacket.ipID {
		_, err = clientConn.WriteToUDP(cLastRecPacket.payload, clientUDPAddr)
		// } else {
		// fmt.Println("remove redunt")
		// }
		// lastID = cLastRecPacket.ipID
		clientReceiveCount++
		checkError(err)
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
		_, err := mClientQueue.Push(fBuf)
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

		cLastRecPacketLock.Lock()
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
		cLastRecPacketLock.Unlock()

		_, err := tun.Write(packet)
		clientSendCount++
		poolPut(fBuf)
		checkError(err)
	}
}
