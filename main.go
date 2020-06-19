package main

import (
	"encoding/hex"
	"log"

	"bufio"
	"fmt"
	"net"
	"os"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/songgao/water"
)

func main() {
	tun := createTUN("faketcp")
	go listenInterface(tun)
	go listenSocket("21001", tun)
	reader := bufio.NewReader(os.Stdin)
	reader.ReadString('\n')
}

func createTUN(name string) *water.Interface {
	config := water.Config{
		DeviceType: water.TUN,
	}
	config.Name = name

	tunInterface, err := water.New(config)
	if err != nil {
		log.Fatal(err)
		return nil
	}
	return tunInterface
}

func listenInterface(tun *water.Interface) {
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

			udpAddr, err := net.ResolveUDPAddr("udp4", "192.168.27.208:21007")
			checkError(err)
			conn, err := net.DialUDP("udp", nil, udpAddr)
			checkError(err)
			_, err = conn.Write(tcp.Payload)
			checkError(err)
		}
		if udpLayer := packet.Layer(layers.LayerTypeUDP); udpLayer != nil {
			fmt.Println("This is a UDP packet!")
			// Get actual UDP data from this layer
			udp, _ := udpLayer.(*layers.UDP)
			fmt.Printf("From src port %d to dst port %d\n", udp.SrcPort, udp.DstPort)
		}

		fmt.Println(hex.Dump(data))
	}
}

func listenSocket(port string, tun *water.Interface) {
	udpAddr, err := net.ResolveUDPAddr("udp4", ":"+port)
	checkError(err)
	conn, err := net.ListenUDP("udp", udpAddr)
	checkError(err)

	buffer := make([]byte, 2000)
	for {
		len, addr, err := conn.ReadFromUDP(buffer[0:])
		if err != nil {
			return
		}

		fmt.Printf("UDP read %d %s\n", len, addr)

		buf := gopacket.NewSerializeBuffer()
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
			DstIP:    net.IP{120, 240, 47, 66},
		}
		tcpL := &layers.TCP{
			SrcPort: layers.TCPPort(8888),
			DstPort: layers.TCPPort(12271),
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

		packet := gopacket.NewPacket(outPacket, layers.LayerTypeIPv4, gopacket.Default)
		for _, layer := range packet.Layers() {
			fmt.Println("PACKET LAYER:", layer.LayerType())
		}
		fmt.Println()

		_, err = tun.Write(outPacket)
		checkError(err)
	}
}

func checkError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %s\n", err.Error())
		os.Exit(1)
	}
}
