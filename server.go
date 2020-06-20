package main

import (
	"encoding/hex"
	"fmt"
	"net"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"

	"github.com/songgao/water"
)

var serverConn *net.UDPConn
var peerIP net.IP
var peerPort uint16

func serverTunToSocket(tun *water.Interface) {
	fmt.Println("server tun to socket")
	buffer := make([]byte, 2000)
	for {
		len, err := tun.Read(buffer)
		checkError(err)
		data := buffer[:len]
		packet := gopacket.NewPacket(data, layers.LayerTypeIPv4, gopacket.Default)

		ipLayer := packet.Layer(layers.LayerTypeIPv4)
		if ipLayer != nil {
			ip := ipLayer.(*layers.IPv4)
			peerIP = ip.SrcIP
		}

		if tcpLayer := packet.Layer(layers.LayerTypeTCP); tcpLayer != nil {
			fmt.Println("TUN interface: This is a TCP packet!")
			// Get actual TCP data from this layer
			tcp, _ := tcpLayer.(*layers.TCP)
			fmt.Printf("src port %d,dst port %d,sqd %d,ack %d,urg %t,ACK %t,psh %t,rst %t,syn %t,fin %t,window %d\n",
				tcp.SrcPort, tcp.DstPort, tcp.Seq, tcp.Ack, tcp.URG, tcp.ACK, tcp.PSH, tcp.RST, tcp.SYN, tcp.FIN, tcp.Window)

			peerPort = uint16(tcp.SrcPort)

			fmt.Printf("server got packet from %s port %d\n", peerIP, peerPort)
			_, err := serverConn.Write(tcp.Payload)
			checkError(err)
		}

		fmt.Println(hex.Dump(data))
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
	for {
		len, err := serverConn.Read(buffer[0:])
		if err != nil {
			fmt.Println("server read udp error")
			continue
		}

		fmt.Printf("server read udp len %d\n", len)

		buf := gopacket.NewSerializeBuffer()
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
			DstIP:    peerIP,
		}
		tcpL := &layers.TCP{
			SrcPort: layers.TCPPort(srcPort),
			DstPort: layers.TCPPort(peerPort),
			Seq:     1,
			Ack:     0,
			NS:      true,
			CWR:     false,
			ECE:     false,
			URG:     false,
			ACK:     false,
			PSH:     false,
			RST:     false,
			SYN:     true,
			FIN:     false,
			Window:  60000,
		}
		tcpL.SetNetworkLayerForChecksum(ipL)
		err = gopacket.SerializeLayers(buf, opts,
			ipL,
			tcpL,
			gopacket.Payload(buffer[:len]))
		checkError(err)

		outPacket := buf.Bytes()
		fmt.Println(hex.Dump(outPacket))

		_, err = tun.Write(outPacket)
		checkError(err)
		fmt.Println("send from tun")
	}
}
