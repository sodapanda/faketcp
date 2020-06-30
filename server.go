package main

import (
	"fmt"
	"net"
	"time"

	"github.com/google/gopacket"

	"github.com/google/netstack/tcpip/header"
	"github.com/songgao/water"
	"github.com/theodesp/blockingQueues"
)

var serverConn *net.UDPConn
var peerIP net.IP
var peerPort uint16
var mClientSeq uint32
var mServerQueue *blockingQueues.BlockingQueue
var serverDrop int
var serverSendCount int
var serverReceiveCount int

func serverHandShake(tun *water.Interface) {
	mServerQueue, _ = blockingQueues.NewArrayBlockingQueue(uint64(queueLen))

	//等待syn
	fmt.Println("server waiting for SYN")
	rBuffer := make([]byte, 2000)
	var clientSeq uint32
	var clientIP net.IP
	var clientPort int
	for {
		len, err := tun.Read(rBuffer)
		checkError(err)
		tcpPacket := parsePacket(rBuffer[:len])
		if tcpPacket == nil {
			continue
		}
		if tcpPacket.SYN {
			clientSeq = tcpPacket.Seq
			clientIP = getSrcIP(rBuffer[:len])
			clientPort = int(tcpPacket.SrcPort)
			break
		}
	}
	fmt.Printf("server got SYN %s %d\n", clientIP, clientPort)
	//返回syn+ack
	fmt.Println("server sending SYN+ACk")
	mClientSeq = addAck(clientSeq)
	pBuffer := gopacket.NewSerializeBuffer()
	synAck := makePacket(pBuffer, net.IP{10, 1, 1, 2}, clientIP, serverTunSrcPort, clientPort, true, true, nextSeq(), mClientSeq, nil)
	_, err := tun.Write(synAck)
	checkError(err)
	fmt.Println("server sent SYN+ACK")
	//等待ACK
	fmt.Println("server waiting for ACK")
	for {
		len, err := tun.Read(rBuffer)
		checkError(err)
		tcpPacket := parsePacket(rBuffer[:len])
		if tcpPacket == nil {
			continue
		}
		if tcpPacket.ACK {
			break
		}
	}
	fmt.Println("server got ACK")
}

var serverTunToSocketReadMaxLen int

func serverTunToSocket(tun *water.Interface) {
	fmt.Println("server tun to socket")
	buffer := make([]byte, 2000)

	fPacket := FPacket{}
	fPacket.srcIP = make([]byte, 4)
	fPacket.dstIP = make([]byte, 4)

	for {
		len, err := tun.Read(buffer)
		if len > serverTunToSocketReadMaxLen {
			serverTunToSocketReadMaxLen = len
		}
		checkError(err)
		data := buffer[:len]
		unpacket(data, &fPacket)
		peerIP = fPacket.srcIP
		peerPort = fPacket.srcPort
		mClientSeq = addAck(fPacket.seqNum)
		_, err = serverConn.Write(fPacket.payload)
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
		len, _ := serverConn.Read(fBuf.data[(header.IPv4MinimumSize + header.TCPMinimumSize):])
		if len > serverSocketReadMaxLen {
			serverSocketReadMaxLen = len
		}
		//这里不能改变pool里每个对象的slice大小，因为改小了的话，下一个包可能不够用
		fBuf.len = len + header.IPv4MinimumSize + header.TCPMinimumSize
		//在这里包装成IP包 入队列直接是IP包

		fPacket := FPacket{
			srcIP:   net.IP{10, 1, 1, 2}.To4(),
			dstIP:   peerIP.To4(),
			srcPort: uint16(srcPort),
			dstPort: uint16(peerPort),
			syn:     false,
			ack:     true,
			seqNum:  nextSeq(),
			ackNum:  mClientSeq,
		}
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
