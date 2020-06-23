package main

import (
	"fmt"
	"net"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"

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
	var ip4 layers.IPv4
	var tcp layers.TCP
	var payload gopacket.Payload
	parser := gopacket.NewDecodingLayerParser(layers.LayerTypeIPv4, &ip4, &tcp, &payload)
	decodedLayers := make([]gopacket.LayerType, 0, 10)

	for {
		len, err := tun.Read(buffer)
		checkError(err)
		data := buffer[:len]

		err = parser.DecodeLayers(data, &decodedLayers)
		checkError(err)

		for _, typ := range decodedLayers {
			switch typ {
			case layers.LayerTypeIPv4:
				peerIP = ip4.SrcIP
			case layers.LayerTypeTCP:
				peerPort = uint16(tcp.SrcPort)
				mClientSeq = addAck(tcp.Seq)
				_, err := serverConn.Write(tcp.Payload)
				checkError(err)
			}
		}
	}
}

func serverSocketToTun(tun *water.Interface, serverSendto string, srcPort int) {
	fmt.Println("server socket to tun")
	udpAddr, err := net.ResolveUDPAddr("udp4", serverSendto)
	checkError(err)
	conn, err := net.DialUDP("udp", nil, udpAddr)
	checkError(err)
	serverConn = conn
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		ComputeChecksums: true,
		FixLengths:       true,
	}

	buffer := make([]byte, 2000)
	ipL := &layers.IPv4{}
	tcpL := &layers.TCP{}
	for {
		len, err := serverConn.Read(buffer[0:])
		if err != nil {
			fmt.Println("server read udp error")
			continue
		}

		// fmt.Printf("server read udp len %d\n", len)
		ipL.Version = 4
		ipL.TOS = 0
		ipL.TTL = 60
		ipL.Id = 10
		ipL.Protocol = 6
		ipL.Flags = 0b010
		ipL.SrcIP = net.IP{10, 1, 1, 2}
		ipL.DstIP = peerIP

		tcpL.SrcPort = layers.TCPPort(srcPort)
		tcpL.DstPort = layers.TCPPort(peerPort)
		tcpL.Seq = nextSeq()
		tcpL.Ack = mClientSeq
		tcpL.NS = false
		tcpL.CWR = false
		tcpL.ECE = false
		tcpL.URG = false
		tcpL.ACK = true
		tcpL.PSH = false
		tcpL.RST = false
		tcpL.SYN = false
		tcpL.FIN = false
		tcpL.Window = 1600

		tcpL.SetNetworkLayerForChecksum(ipL)

		err = gopacket.SerializeLayers(buf, opts, ipL, tcpL, gopacket.Payload(buffer[:len]))
		checkError(err)

		outPacket := buf.Bytes()
		// fmt.Println(hex.Dump(outPacket))

		_, err = tun.Write(outPacket)
		checkError(err)
		// fmt.Println("send from tun")
	}
}
