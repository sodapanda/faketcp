package main

import (
	"fmt"
	"math"
	"net"
	"time"

	"github.com/google/netstack/tcpip/header"
	"github.com/songgao/water"
	"github.com/theodesp/blockingQueues"
)

var clientConn *net.UDPConn
var clientAddr *net.UDPAddr
var mClientQueue *blockingQueues.BlockingQueue
var clientDrop int
var clientSendCount int
var clientReceiveCount int
var cLastRecPacket FPacket

var reduntCount int
var reorderCount int
var pushbackCount int
var emptyPutCount int
var poolWrongFlag bool

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

func clientTunToSocketFEC(tun *water.Interface) {
	fmt.Println("client tun to socket with FEC")
	buffer := make([]byte, 2000)
	decodeResult := make([]*FBuffer, mSegCount)
	for i := range decodeResult {
		decodeResult[i] = new(FBuffer)
		decodeResult[i].data = make([]byte, 2000)
	}

	for {
		n, err := tun.Read(buffer)
		clientReceiveCount++
		checkError(err)
		data := buffer[:n]

		rcvPkt := new(ftPacket)
		rcvPkt.decode(data[40:])

		done := mCodec.decode(rcvPkt, decodeResult)
		if !done {
			continue
		}

		for _, d := range decodeResult {
			if d.len == 0 {
				continue
			}
			_, err = clientConn.WriteToUDP(d.data[:d.len], clientAddr)
		}

		checkError(err)
	}
}

func clientTunToSocketNoFEC(tun *water.Interface) {
	fmt.Println("client tun")
	buffer := make([]byte, 2000)

	for {
		n, err := tun.Read(buffer)
		checkError(err)
		data := buffer[:n]

		unpacket(data, &cLastRecPacket)

		_, err = clientConn.WriteToUDP(cLastRecPacket.payload, clientAddr)
		clientReceiveCount++
		checkError(err)
	}
}

func clientSocketToQueue(socketListenPort string, serverIP string, serverPort int) {
	udpAddr, err := net.ResolveUDPAddr("udp4", ":"+socketListenPort)
	conn, err := net.ListenUDP("udp", udpAddr)
	checkError(err)
	fmt.Printf("client listen socket %s\n", udpAddr)
	clientConn = conn
	dstIP := net.ParseIP(serverIP).To4()
	srcIP := net.IP{10, 1, 1, 2}.To4()
	for {
		fBuf := poolGet()
		lenU, cAddr, err := conn.ReadFromUDP(fBuf.data[(header.IPv4MinimumSize + header.TCPMinimumSize):])
		checkError(err)
		clientAddr = cAddr

		fBuf.len = lenU + header.IPv4MinimumSize + header.TCPMinimumSize

		packet := fBuf.data[:fBuf.len]
		fPacket := FPacket{}
		fPacket.srcIP = srcIP
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

		_, err = mClientQueue.Push(fBuf)

		if err != nil {
			clientDrop++
			poolPut(fBuf)
			fmt.Println("client drop packet ", clientDrop)
		}
	}
}

func clientSocketToQueueFEC(socketListenPort string, serverIP string, serverPort int) {
	udpAddr, err := net.ResolveUDPAddr("udp4", ":"+socketListenPort)
	conn, err := net.ListenUDP("udp", udpAddr)
	checkError(err)
	fmt.Printf("client listen socket with FEC %s\n", udpAddr)
	clientConn = conn
	dstIP := net.ParseIP(serverIP).To4()
	srcIP := net.IP{10, 1, 1, 2}.To4()

	readBuf := make([]byte, 2000)

	gapF := float64(mGap) / float64(mSegCount+mFecCount)
	gap := int(math.Ceil(gapF))
	sb := newStageBuffer(mSegCount)
	fullDataBuffer := make([]byte, 2000*mSegCount)
	encodeResult := make([]*FBuffer, mSegCount+mFecCount)

	for {
		length, cAddr, err := conn.ReadFromUDP(readBuf[0:])
		checkError(err)
		clientAddr = cAddr

		sb.append(readBuf[0:length], uint16(length), fullDataBuffer, mCodec, func(cSb *stageBuffer, resultData []byte, realLength int) {
			for i := range encodeResult {
				encodeResult[i] = poolGet()
			}

			fPacket := FPacket{}
			fPacket.srcIP = srcIP
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

			mCodec.encode(resultData, realLength, &fPacket, encodeResult)

			if mGap > 0 {
				for i, data := range encodeResult {
					timer := time.NewTimer(time.Duration(gap*i) * time.Millisecond)
					go func(packetData *FBuffer) {
						<-timer.C
						_, err := mClientQueue.Push(packetData)
						if err != nil {
							clientDrop++
							println("client drop packet ", clientDrop)
							poolPut(packetData)
						}
					}(data)
				}
			} else {
				for i := range encodeResult {
					_, err := mClientQueue.Push(encodeResult[i])
					if err != nil {
						clientDrop++
						println("client drop packet ", clientDrop)
						poolPut(encodeResult[i])
					}
				}
			}
		})
	}
}

func clientQueueToTun(tun *water.Interface) {
	fmt.Println("client queue to tun")
	for {
		item, _ := mClientQueue.Get()
		fBuf := item.(*FBuffer)
		data := fBuf.data[:fBuf.len]

		writeLen, err := tun.Write(data)
		clientSendCount++
		poolPut(fBuf)
		checkError(err)
		if writeLen != len(data) {
			fmt.Println("client tun write not full")
		}
	}
}
