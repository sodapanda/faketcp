package main

import (
	"encoding/hex"
	"flag"
	"log"

	"bufio"
	"fmt"
	"net"
	"os"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/songgao/water"
)

var mServer bool
var mConn *net.UDPConn
var mUDPAddr *net.UDPAddr
var remoteIP net.IP
var remotePort uint16
var mSrcPort uint16

func main() {
	isServer := flag.Bool("s", false, "is server")
	flag.Parse()
	mServer = *isServer
	fmt.Println(mServer)

	createConn()
	tun := createTUN("faketcp")
	go listenInterface(tun)
	go listenSocket(tun)
	reader := bufio.NewReader(os.Stdin)
	reader.ReadString('\n')
}

func createConn() {
	if mServer {
		udpAddr, err := net.ResolveUDPAddr("udp4", "45.117.103.179:21007")
		checkError(err)
		conn, err := net.DialUDP("udp", nil, udpAddr)
		checkError(err)
		mConn = conn
		fmt.Println("server start")
	} else {
		udpAddr, err := net.ResolveUDPAddr("udp4", ":21001")
		checkError(err)
		mUDPAddr = udpAddr
		conn, err := net.ListenUDP("udp", udpAddr)
		checkError(err)
		mConn = conn
		fmt.Println("client start")
	}
	if !mServer {
		remoteIP = net.IP{45, 117, 103, 179}
		remotePort = 12271
		mSrcPort = 8888
	} else {
		mSrcPort = 12271
	}
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

		if mServer {
			fmt.Printf("server read tun %d\n", n)
			ipLayer := packet.Layer(layers.LayerTypeIPv4)
			if ipLayer != nil {
				ip := ipLayer.(*layers.IPv4)
				remoteIP = ip.SrcIP
				fmt.Println(ip.SrcIP)
			}
		}

		if tcpLayer := packet.Layer(layers.LayerTypeTCP); tcpLayer != nil {
			fmt.Println("TUN interface: This is a TCP packet!")
			// Get actual TCP data from this layer
			tcp, _ := tcpLayer.(*layers.TCP)
			fmt.Printf("src port %d,dst port %d,sqd %d,ack %d,urg %t,ACK %t,psh %t,rst %t,syn %t,fin %t,window %d\n",
				tcp.SrcPort, tcp.DstPort, tcp.Seq, tcp.Ack, tcp.URG, tcp.ACK, tcp.PSH, tcp.RST, tcp.SYN, tcp.FIN, tcp.Window)
			if mServer {
				remotePort = uint16(tcp.SrcPort)
				_, err := mConn.Write(tcp.Payload)
				checkError(err)
			} else {
				_, err := mConn.WriteToUDP(tcp.Payload, mUDPAddr)
				checkError(err)
			}
		}

		fmt.Println(hex.Dump(data))
	}
}

func listenSocket(tun *water.Interface) {
	buffer := make([]byte, 2000)
	var len int
	for {
		if !mServer {
			n, a, _ := mConn.ReadFromUDP(buffer[0:])
			len = n
			mUDPAddr = a
		} else {
			n, _ := mConn.Read(buffer[0:])
			len = n
		}

		fmt.Printf("UDP read %d %s\n", len, mUDPAddr)
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
			DstIP:    remoteIP,
		}
		tcpL := &layers.TCP{
			SrcPort: layers.TCPPort(mSrcPort),
			DstPort: layers.TCPPort(remotePort),
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
		err := gopacket.SerializeLayers(buf, opts,
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
		fmt.Println("send from tun")
	}
}

func checkError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %s\n", err.Error())
		os.Exit(1)
	}
}
