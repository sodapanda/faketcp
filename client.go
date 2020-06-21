package main

import (
	"fmt"
	"net"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/songgao/water"
)

var clientConn *net.UDPConn
var clientUDPAddr *net.UDPAddr
var mServerSeq uint32

func handShake(tun *water.Interface) bool {
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
	var ip4 layers.IPv4
	var tcp layers.TCP
	var payload gopacket.Payload

	parser := gopacket.NewDecodingLayerParser(layers.LayerTypeIPv4, &ip4, &tcp, &payload)
	decodedLayers := make([]gopacket.LayerType, 0, 10)

	for {
		n, err := tun.Read(buffer)
		checkError(err)
		data := buffer[:n]

		err = parser.DecodeLayers(data, &decodedLayers)
		checkError(err)

		for _, typ := range decodedLayers {
			switch typ {
			case layers.LayerTypeIPv4:
			case layers.LayerTypeTCP:
				mServerSeq = addAck(tcp.Seq)
				_, err := clientConn.WriteToUDP(tcp.Payload, clientUDPAddr)
				checkError(err)
			}
		}
	}
}

func clientSocketToTun(socketListenPort string, tun *water.Interface, serverIP string, serverPort int) {
	buffer := make([]byte, 2000)
	udpAddr, err := net.ResolveUDPAddr("udp4", ":"+socketListenPort)
	checkError(err)
	conn, err := net.ListenUDP("udp", udpAddr)
	checkError(err)
	fmt.Printf("client listen socket %s\n", udpAddr)
	clientConn = conn
	packetBuff := gopacket.NewSerializeBuffer()

	for {
		len, addr, err := conn.ReadFromUDP(buffer[0:])
		if err != nil {
			fmt.Println("client read udp error" + err.Error())
			continue
		}
		clientUDPAddr = addr

		// fmt.Printf("client read udp %d\n", len)

		opts := gopacket.SerializeOptions{
			ComputeChecksums: true,
			FixLengths:       true,
		}

		ipL := &layers.IPv4{
			Version:  4,
			TOS:      0,
			TTL:      60,
			Id:       10,
			Protocol: 6,
			Flags:    0b010,
			SrcIP:    net.IP{10, 1, 1, 2},
			DstIP:    net.ParseIP(serverIP),
		}
		tcpL := &layers.TCP{
			SrcPort: layers.TCPPort(8888),
			DstPort: layers.TCPPort(serverPort),
			Seq:     nextSeq(),
			Ack:     mServerSeq,
			NS:      false,
			CWR:     false,
			ECE:     false,
			URG:     false,
			ACK:     true,
			PSH:     false,
			RST:     false,
			SYN:     false,
			FIN:     false,
			Window:  1600,
		}
		tcpL.SetNetworkLayerForChecksum(ipL)
		err = gopacket.SerializeLayers(packetBuff, opts,
			ipL,
			tcpL,
			gopacket.Payload(buffer[:len]))
		checkError(err)
		outPacket := packetBuff.Bytes()
		// fmt.Println(hex.Dump(outPacket))

		_, err = tun.Write(outPacket)
		checkError(err)
		// fmt.Println("client send out by tun")
	}
}
