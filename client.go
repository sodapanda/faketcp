package main

import (
	"fmt"
	"net"

	"github.com/google/gopacket"
	"github.com/google/netstack/tcpip/header"
	"github.com/songgao/water"
	"github.com/theodesp/blockingQueues"
)

var clientConn *net.UDPConn
var clientUDPAddr *net.UDPAddr
var mServerSeq uint32
var mClientQueue *blockingQueues.BlockingQueue
var clientDrop int
var clientSendCount int
var clientReceiveCount int

func handShake(tun *water.Interface) bool {
	mClientQueue, _ = blockingQueues.NewArrayBlockingQueue(uint64(queueLen))
	//发送SYN
	pBuffer := gopacket.NewSerializeBuffer()
	srcIP := net.ParseIP(clientTunSrcIP)
	dstIP := net.ParseIP(clientTunDstIP)
	fmt.Printf("client sending SYN... %s %d \n", dstIP, clientTunDstPort)
	syn := makePacket(pBuffer, srcIP, dstIP, clientTunSrcPort, clientTunDstPort, true, false, nextSeq(), 0, nil)
	_, err := tun.Write(syn)
	checkError(err)
	fmt.Println("client SYN sent")
	//等待SYN+ACK
	fmt.Println("client waiting for SYN+ACK")
	rBuffer := make([]byte, 2000)
	var serverSeq uint32
	for {
		len, err := tun.Read(rBuffer)
		checkError(err)
		tcpPacket := parsePacket(rBuffer[:len])
		if tcpPacket == nil {
			continue
		}

		if tcpPacket.SYN && tcpPacket.ACK {
			serverSeq = tcpPacket.Seq
			break
		}
	}
	fmt.Println("client got SYN+ACK ")

	//发送ACK
	fmt.Println("client sending ACK")
	mServerSeq = addAck(serverSeq)
	ack := makePacket(pBuffer, srcIP, dstIP, clientTunSrcPort, clientTunDstPort, false, true, nextSeq(), mServerSeq, nil)
	_, err = tun.Write(ack)
	checkError(err)
	fmt.Println("client ACK sent")
	return true
}

func clientTunToSocket(tun *water.Interface) {
	fmt.Println("client tun")
	buffer := make([]byte, 2000)

	for {
		n, err := tun.Read(buffer)
		checkError(err)
		data := buffer[:n]
		fPacket := FPacket{}
		unpacket(data, &fPacket)
		mServerSeq = addAck(fPacket.seqNum)
		_, err = clientConn.WriteToUDP(fPacket.payload, clientUDPAddr)
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

		fPacket := FPacket{}
		fPacket.srcIP = net.IP{10, 1, 1, 2}.To4()
		fPacket.dstIP = net.ParseIP(serverIP).To4()
		fPacket.srcPort = 8888
		fPacket.dstPort = uint16(serverPort)
		fPacket.seqNum = nextSeq()
		fPacket.ackNum = mServerSeq
		fPacket.syn = false
		fPacket.ack = true

		craftPacket(packet, &fPacket)

		_, err := tun.Write(packet)
		clientSendCount++
		poolPut(fBuf)
		checkError(err)
	}
}
