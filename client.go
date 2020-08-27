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

type client struct {
	tun         *water.Interface
	socketConn  *net.UDPConn
	socketAddr  *net.UDPAddr
	packetQueue *blockingQueues.BlockingQueue
	lastRecvPkt *FPacket
	tunDstIP    net.IP
	tunSrcIP    net.IP
	sent        int
	recv        int
	drop        int
}

func newClient(tun *water.Interface) *client {
	clt := new(client)
	clt.packetQueue, _ = blockingQueues.NewArrayBlockingQueue(uint64(mConfig.QLen))
	clt.tun = tun

	udpAddr, err := net.ResolveUDPAddr("udp4", ":"+mConfig.ClientSocketListenPort)
	conn, err := net.ListenUDP("udp", udpAddr)
	checkError(err)
	fmt.Printf("client listen socket %s\n", udpAddr)

	clt.socketConn = conn
	clt.socketAddr = udpAddr
	clt.tunDstIP = net.ParseIP(mConfig.ClientTunToIP).To4()
	clt.tunSrcIP = net.ParseIP(mConfig.TunSrcIP).To4()
	clt.lastRecvPkt = new(FPacket)
	clt.lastRecvPkt.srcIP = make([]byte, 4)
	clt.lastRecvPkt.dstIP = make([]byte, 4)
	return clt
}

func (clt *client) tunToSocketFEC(chann chan string) {
	fmt.Println("client tun to socket with FEC")
	buffer := make([]byte, 2000)
	decodeResult := make([]*FBuffer, mConfig.SegCount)
	for i := range decodeResult {
		decodeResult[i] = new(FBuffer)
		decodeResult[i].data = make([]byte, 2000)
	}

	for {
		n, err := clt.tun.Read(buffer)
		clt.recv++
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
			_, err = clt.socketConn.WriteToUDP(d.data[:d.len], clt.socketAddr)
			d.len = 0 //设置为0 表示没有内容
		}

		checkError(err)
	}
}

func (clt *client) tunToSocketNoFEC(chann chan string) {
	fmt.Println("client tun")
	buffer := make([]byte, 2000)

	for {
		n, err := clt.tun.Read(buffer)
		clt.recv++
		checkError(err)
		data := buffer[:n]

		unpacket(data, clt.lastRecvPkt)

		_, err = clt.socketConn.WriteToUDP(clt.lastRecvPkt.payload, clt.socketAddr)
		checkError(err)
	}
}

func (clt *client) socketToQueue(chann chan string) {
	for {
		fBuf := poolGet()
		lenU, cAddr, err := clt.socketConn.ReadFromUDP(fBuf.data[(header.IPv4MinimumSize + header.TCPMinimumSize):])
		checkError(err)
		clt.socketAddr = cAddr

		fBuf.len = lenU + header.IPv4MinimumSize + header.TCPMinimumSize

		packet := fBuf.data[:fBuf.len]
		fPacket := FPacket{}
		fPacket.srcIP = clt.tunSrcIP
		fPacket.dstIP = clt.tunDstIP
		fPacket.srcPort = uint16(mConfig.TunSrcPort)
		fPacket.dstPort = uint16(mConfig.ClientTunToPort)
		fPacket.seqNum = clt.lastRecvPkt.ackNum
		if clt.lastRecvPkt.syn {
			// syn 也算一个
			fPacket.ackNum = clt.lastRecvPkt.seqNum + 1
		} else {
			fPacket.ackNum = clt.lastRecvPkt.seqNum + uint32(len(clt.lastRecvPkt.payload))
		}

		fPacket.syn = false
		fPacket.ack = true

		craftPacket(packet, &fPacket)

		_, err = clt.packetQueue.Push(fBuf)

		if err != nil {
			clt.drop++
			poolPut(fBuf)
			fmt.Println("client drop packet ", clt.drop)
		}
	}
}

func (clt *client) socketToQueueFEC(chann chan string) {
	readBuf := make([]byte, 2000)

	gapF := float64(mConfig.Gap) / float64(mConfig.SegCount+mConfig.FecCount)
	gap := int(math.Ceil(gapF))
	sb := newStageBuffer(mConfig.SegCount)
	fullDataBuffer := make([]byte, 2000*mConfig.SegCount)
	encodeResult := make([]*FBuffer, mConfig.SegCount+mConfig.FecCount)

	for {
		length, cAddr, err := clt.socketConn.ReadFromUDP(readBuf[0:])
		checkError(err)
		clt.socketAddr = cAddr

		sb.append(readBuf[0:length], uint16(length), fullDataBuffer, mCodec, func(cSb *stageBuffer, resultData []byte, realLength int) {
			for i := range encodeResult {
				encodeResult[i] = poolGet()
			}

			fPacket := FPacket{}
			fPacket.srcIP = clt.tunSrcIP
			fPacket.dstIP = clt.tunDstIP
			fPacket.srcPort = uint16(mConfig.TunSrcPort)
			fPacket.dstPort = uint16(mConfig.ClientTunToPort)
			fPacket.seqNum = clt.lastRecvPkt.ackNum
			if clt.lastRecvPkt.syn {
				// syn 也算一个
				fPacket.ackNum = clt.lastRecvPkt.seqNum + 1
			} else {
				fPacket.ackNum = clt.lastRecvPkt.seqNum + uint32(len(clt.lastRecvPkt.payload))
			}

			fPacket.syn = false
			fPacket.ack = true

			mCodec.encode(resultData, realLength, &fPacket, encodeResult)

			if mConfig.Gap > 0 {
				for i, data := range encodeResult {
					timer := time.NewTimer(time.Duration(gap*i) * time.Millisecond)
					go func(packetData *FBuffer) {
						<-timer.C
						_, err := clt.packetQueue.Push(packetData)
						if err != nil {
							clt.drop++
							println("client drop packet ", clt.drop)
							poolPut(packetData)
						}
					}(data)
				}
			} else {
				for i := range encodeResult {
					_, err := clt.packetQueue.Push(encodeResult[i])
					if err != nil {
						clt.drop++
						println("client drop packet ", clt.drop)
						poolPut(encodeResult[i])
					}
				}
			}
		})
	}
}

func (clt *client) queueToTun(chann chan string) {
	fmt.Println("client queue to tun")
	for {
		item, _ := clt.packetQueue.Get()
		fBuf := item.(*FBuffer)
		data := fBuf.data[:fBuf.len]

		writeLen, err := clt.tun.Write(data)
		clt.sent++
		poolPut(fBuf)
		checkError(err)
		if writeLen != len(data) {
			fmt.Println("client tun write not full")
		}
	}
}

func stopClient() {
	//把go毒死

}
