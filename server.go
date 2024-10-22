package main

import (
	"fmt"
	"math"
	"net"
	"os"
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
var mSerSeq uint32
var reduntCounter int

func serverTunToSocket(tun *water.Interface, chann chan string) {
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
		isSyn := checkSYN(data)

		if isSyn {
			chann <- "newSyn"
			return
		}

		_, err = serverConn.Write(data[header.IPv4MinimumSize+header.TCPMinimumSize:])
		serverReceiveCount++
		checkError(err)
	}
}

func serverTunToSocketFEC(tun *water.Interface, chann chan string) {
	fmt.Println("server tun to socket FEC")
	buffer := make([]byte, 2000)
	decodeResult := make([]*FBuffer, mConfig.SegCount)
	for i := range decodeResult {
		decodeResult[i] = new(FBuffer)
		decodeResult[i].data = make([]byte, 2000)
	}

	for {
		len, err := tun.Read(buffer)
		serverReceiveCount++
		checkError(err)
		data := buffer[:len]

		isSyn := checkSYN(data)

		if isSyn {
			chann <- "newSyn"
			return
		}

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
			_, err = serverConn.Write(d.data[:d.len])
			d.len = 0 //设置为0 表示没有内容
		}
		checkError(err)
	}
}

func serverSocketToQueueFEC(serverSendto string, srcPort int, chann chan string) {
	fmt.Println("server socket to queue with FEC")
	udpAddr, err := net.ResolveUDPAddr("udp4", serverSendto)
	checkError(err)
	conn, err := net.DialUDP("udp", nil, udpAddr)
	checkError(err)
	serverConn = conn
	readBuf := make([]byte, 2000)
	gapF := float64(mConfig.Gap) / float64(mConfig.SegCount+mConfig.FecCount)
	gap := int(math.Ceil(gapF))
	sb := newStageBuffer(mConfig.SegCount)
	fullDataBuffer := make([]byte, 2000*mConfig.SegCount)
	encodeResult := make([]*FBuffer, mConfig.SegCount+mConfig.FecCount)

	for {
		length, _ := serverConn.Read(readBuf[0:])
		sb.append(readBuf[0:length], uint16(length), fullDataBuffer, mCodec, func(cSb *stageBuffer, resultData []byte, realLength int) {
			for i := range encodeResult {
				encodeResult[i] = poolGet()
			}

			fPacket := FPacket{
				srcIP:   net.ParseIP(mConfig.TunSrcIP).To4(),
				dstIP:   peerIP.To4(),
				srcPort: uint16(srcPort),
				dstPort: uint16(peerPort),
				syn:     false,
				ack:     true,
				seqNum:  mSerSeq + uint32(length),
				ackNum:  lastRecPacket.seqNum + uint32(len(lastRecPacket.payload)),
			}

			mCodec.encode(resultData, realLength, &fPacket, encodeResult)

			if mConfig.Gap > 0 {
				for i, data := range encodeResult {
					timer := time.NewTimer(time.Duration(gap*i) * time.Millisecond)
					go func(packetData *FBuffer) {
						<-timer.C
						_, err := mServerQueue.Push(packetData)
						if err != nil {
							serverDrop++
							println("server drop packet ", serverDrop)
							poolPut(packetData)
						}
					}(data)
				}
			} else {
				for i := range encodeResult {
					_, err := mServerQueue.Push(encodeResult[i])
					if err != nil {
						serverDrop++
						println("server drop packet ", serverDrop)
						poolPut(encodeResult[i])
					}
				}
			}
		})
	}
}

func serverSocketToQueueNoFEC(serverSendto string, srcPort int, chann chan string) {
	fmt.Println("server socket to queue")

	udpAddr, err := net.ResolveUDPAddr("udp4", serverSendto)
	checkError(err)
	conn, err := net.DialUDP("udp", nil, udpAddr)
	checkError(err)
	serverConn = conn

	for {
		fBuf := poolGet()
		length, err := serverConn.Read(fBuf.data[(header.IPv4MinimumSize + header.TCPMinimumSize):])
		checkError(err)
		//这里不能改变pool里每个对象的slice大小，因为改小了的话，下一个包可能不够用
		fBuf.len = length + header.IPv4MinimumSize + header.TCPMinimumSize
		//在这里包装成IP包 入队列直接是IP包
		fPacket := FPacket{
			srcIP:   net.ParseIP(mConfig.TunSrcIP).To4(),
			dstIP:   peerIP.To4(),
			srcPort: uint16(srcPort),
			dstPort: uint16(peerPort),
			syn:     false,
			ack:     true,
			seqNum:  mSerSeq + uint32(length),
			ackNum:  lastRecPacket.seqNum + uint32(len(lastRecPacket.payload)),
		}
		craftPacket(fBuf.data[:fBuf.len], &fPacket)

		_, err = mServerQueue.Push(fBuf)

		if err != nil {
			serverDrop++
			println("server drop packet ", serverDrop)
			poolPut(fBuf)
		}
	}
}

func serverQueueToTun(tun *water.Interface, chann chan string) {
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

var checkPacket FPacket

func checkSYN(pkt []byte) bool {
	unpacket(pkt, &checkPacket)
	if checkPacket.syn {
		return true
	}
	return false
}

func serverStop() {

}
