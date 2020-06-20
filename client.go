package main

import (
	"encoding/hex"
	"fmt"
	"net"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/songgao/water"
)

var clientConn *net.UDPConn
var clientUDPAddr *net.UDPAddr

func clientTunToSocket(tun *water.Interface) {
	fmt.Println("client tun")
	buffer := make([]byte, 2000)
	for {
		n, err := tun.Read(buffer)
		checkError(err)
		data := buffer[:n]
		packet := gopacket.NewPacket(data, layers.LayerTypeIPv4, gopacket.Default)

		if tcpLayer := packet.Layer(layers.LayerTypeTCP); tcpLayer != nil {
			fmt.Println("TUN interface: This is a TCP packet!")
			// Get actual TCP data from this layer
			tcp, _ := tcpLayer.(*layers.TCP)
			fmt.Printf("src port %d,dst port %d,sqd %d,ack %d,urg %t,ACK %t,psh %t,rst %t,syn %t,fin %t,window %d\n",
				tcp.SrcPort, tcp.DstPort, tcp.Seq, tcp.Ack, tcp.URG, tcp.ACK, tcp.PSH, tcp.RST, tcp.SYN, tcp.FIN, tcp.Window)

			_, err := clientConn.WriteToUDP(tcp.Payload, clientUDPAddr)
			checkError(err)
		}
		fmt.Println(hex.Dump(data))
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

	for {
		len, addr, err := conn.ReadFromUDP(buffer[0:])
		if err != nil {
			fmt.Println("client read udp error" + err.Error())
			continue
		}
		clientUDPAddr = addr

		fmt.Printf("client read udp %d\n", len)

		packetBuff := gopacket.NewSerializeBuffer()
		opts := gopacket.SerializeOptions{
			ComputeChecksums: true,
			FixLengths:       true,
		}

		ipL := &layers.IPv4{
			Version:  4,
			TOS:      0,
			TTL:      20,
			Id:       10,
			Protocol: 6,
			Flags:    0b010,
			SrcIP:    net.IP{10, 1, 1, 2},
			DstIP:    net.ParseIP(serverIP),
		}
		tcpL := &layers.TCP{
			SrcPort: layers.TCPPort(8888),
			DstPort: layers.TCPPort(serverPort),
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
		err = gopacket.SerializeLayers(packetBuff, opts,
			ipL,
			tcpL,
			gopacket.Payload(buffer[:len]))
		checkError(err)
		outPacket := packetBuff.Bytes()
		fmt.Println(hex.Dump(outPacket))

		_, err = tun.Write(outPacket)
		checkError(err)
		fmt.Println("client send out by tun")
	}
}
