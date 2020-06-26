package main

import (
	"fmt"
	"net"

	"github.com/google/gopacket"

	"github.com/songgao/water"
	"github.com/theodesp/blockingQueues"
)

var serverConn *net.UDPConn
var peerIP net.IP
var peerPort uint16
var mClientSeq uint32
var mServerQueue *blockingQueues.BlockingQueue
var serverDrop int

func serverHandShake(tun *water.Interface) {
	mServerQueue, _ = blockingQueues.NewArrayBlockingQueue(300)

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

func serverTunToSocket(tun *water.Interface) {
	fmt.Println("server tun to socket")
	buffer := make([]byte, 2000)

	fPacket := FPacket{}
	fPacket.srcIP = make([]byte, 4)
	fPacket.dstIP = make([]byte, 4)

	for {
		len, err := tun.Read(buffer)
		checkError(err)
		data := buffer[:len]
		unpacket(data, &fPacket)
		peerIP = fPacket.srcIP
		peerPort = fPacket.srcPort
		mClientSeq = addAck(fPacket.seqNum)
		_, err = serverConn.Write(fPacket.payload)
		checkError(err)
	}

}

func serverSocketToQueue(serverSendto string) {
	fmt.Println("server socket to queue")

	udpAddr, err := net.ResolveUDPAddr("udp4", serverSendto)
	checkError(err)
	conn, err := net.DialUDP("udp", nil, udpAddr)
	checkError(err)
	serverConn = conn
	tmpBuf := make([]byte, 2000)

	for {
		len, _ := serverConn.Read(tmpBuf)
		fBuf := poolGet()
		copy(fBuf.data, tmpBuf[:len])
		fBuf.len = len
		_, err := mServerQueue.Push(fBuf)
		if err != nil {
			serverDrop++
			if serverDrop > 1000000 {
				serverDrop = 0
			}
			poolPut(fBuf)
		}
	}
}

func serverQueueToTun(tun *water.Interface, srcPort int) {
	pBuffer := make([]byte, 2000)
	for {
		item, _ := mServerQueue.Get()
		fBuf := item.(*FBuffer)
		data := fBuf.data[:fBuf.len]

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
		pLen := craftPacket(data, pBuffer, &fPacket)
		outPacket := pBuffer[:pLen]
		_, err := tun.Write(outPacket)
		poolPut(fBuf)
		checkError(err)
	}
}
