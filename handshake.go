package main

import (
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/songgao/water"
	"github.com/theodesp/blockingQueues"
)

type clientHandShake struct {
	sync.Mutex
	tun       *water.Interface
	timeout   time.Duration
	recvPkt   *FBuffer
	getSynAck bool
	clt       *client
}

func newClientHandshak(timeout time.Duration, tun *water.Interface, clt *client) *clientHandShake {
	chs := new(clientHandShake)
	chs.timeout = timeout
	chs.tun = tun
	chs.recvPkt = &FBuffer{}
	chs.recvPkt.data = make([]byte, 40)
	chs.clt = clt
	chs.recvPkt.len = 0 //用0表示没有内容
	return chs
}

//连不上的时候要不断的发送是为了等底层的网络重播完成之后拿到了新的ip了，要把一路上的nat打通
func (chs *clientHandShake) sendSYN() {
	chs.Lock()
	defer chs.Unlock()

	srcIP := net.ParseIP(mConfig.TunSrcIP).To4()
	dstIP := net.ParseIP(mConfig.ClientTunToIP).To4()

	fPacket := FPacket{}
	fPacket.srcIP = srcIP
	fPacket.dstIP = dstIP
	fPacket.srcPort = uint16(mConfig.TunSrcPort)
	fPacket.dstPort = uint16(mConfig.ClientTunToPort)
	fPacket.syn = true
	fPacket.ack = false
	fPacket.seqNum = 1024 //初始化seq
	fPacket.ackNum = 0
	fPacket.payload = nil
	chs.recvPkt.len = 40
	craftPacket(chs.recvPkt.data, &fPacket)
	_, err := chs.tun.Write(chs.recvPkt.data[:chs.recvPkt.len])
	fmt.Printf("client sending SYN... %s %d \n", dstIP, mConfig.ClientTunToPort)
	checkError(err)
	chs.recvPkt.len = 0 //表示还没收到内容
}

func (chs *clientHandShake) startListen() {
	//等待syn+ack 或者超时
	go func() {
		readLen, err := chs.tun.Read(chs.recvPkt.data)
		chs.Lock()
		defer chs.Unlock()
		chs.recvPkt.len = readLen
		checkError(err)

		packet := chs.recvPkt.data[:chs.recvPkt.len]

		unpacket(packet, chs.clt.lastRecvPkt)
		chs.getSynAck = true
		fmt.Println("client got syn+ack")
	}()
}

func (chs *clientHandShake) checkConn() bool {
	chs.Lock()
	chs.Unlock()
	return chs.getSynAck
}

func (chs *clientHandShake) sendAck() {
	//发送ack
	fPacket := FPacket{}
	srcIP := net.ParseIP(mConfig.TunSrcIP).To4()
	dstIP := net.ParseIP(mConfig.ClientTunToIP).To4()

	fPacket.srcIP = srcIP.To4()
	fPacket.dstIP = dstIP.To4()
	fPacket.srcPort = uint16(mConfig.TunSrcPort)
	fPacket.dstPort = uint16(mConfig.ClientTunToPort)
	fPacket.syn = false
	fPacket.ack = true
	fPacket.seqNum = chs.clt.lastRecvPkt.ackNum
	fPacket.ackNum = chs.clt.lastRecvPkt.ackNum + 1
	fPacket.payload = nil

	craftPacket(chs.recvPkt.data, &fPacket)
	chs.recvPkt.len = 40
	_, err := chs.tun.Write(chs.recvPkt.data[:chs.recvPkt.len])
	checkError(err)
	fmt.Println("send ack")
}

/*
服务端握手
*/

type serverHandshake struct {
	tun       *water.Interface
	clientSeq uint32
}

func newServerHandshak(tun *water.Interface) *serverHandshake {
	mServerQueue, _ = blockingQueues.NewArrayBlockingQueue(uint64(mConfig.QLen))
	sh := new(serverHandshake)
	sh.tun = tun
	return sh
}

func (sh *serverHandshake) waitSyn() {
	fmt.Println("server waiting for SYN")
	packet := make([]byte, 40)
	_, err := sh.tun.Read(packet)
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
	sh.clientSeq = fPacket.seqNum
}

func (sh *serverHandshake) sendSynAck() {
	fmt.Println("server send syn+ack")
	packet := make([]byte, 40)
	fPacket := FPacket{}
	fPacket.srcIP = net.ParseIP(mConfig.TunSrcIP).To4()
	fPacket.dstIP = make([]byte, 4)
	copy(fPacket.dstIP, peerIP)
	fPacket.srcPort = uint16(mConfig.ServerTunPort)
	fPacket.dstPort = peerPort
	fPacket.syn = true
	fPacket.ack = true
	fPacket.seqNum = 1000 //首次发送syn
	fPacket.ackNum = sh.clientSeq + 1
	fPacket.payload = nil

	craftPacket(packet, &fPacket)
	_, err := sh.tun.Write(packet)
	checkError(err)
	mSerSeq = 1000
}

func (sh *serverHandshake) waitAck() {
	packet := make([]byte, 40)
	fPacket := FPacket{}
	_, err := sh.tun.Read(packet)
	checkError(err)
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
