package main

import (
	"fmt"
	"net"

	"github.com/google/gopacket"

	"github.com/songgao/water"
)

var serverConn *net.UDPConn
var peerIP net.IP
var peerPort uint16
var mClientSeq uint32

func serverHandShake(tun *water.Interface) {
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

func serverSocketToTun(tun *water.Interface, serverSendto string, srcPort int) {
	fmt.Println("server socket to tun")
	udpAddr, err := net.ResolveUDPAddr("udp4", serverSendto)
	checkError(err)
	conn, err := net.DialUDP("udp", nil, udpAddr)
	checkError(err)
	serverConn = conn

	buffer := make([]byte, 2000)
	pBuffer := make([]byte, 2000)

	for {
		len, err := serverConn.Read(buffer[0:])
		if err != nil {
			fmt.Println("server read udp error")
			continue
		}
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
		pLen := craftPacket(buffer[:len], pBuffer, &fPacket)
		outPacket := pBuffer[:pLen]
		_, err = tun.Write(outPacket)
		checkError(err)
		// fmt.Println("send from tun")
		// fmt.Println(hex.Dump(outPacket))
	}
}
