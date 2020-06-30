package main

import (
	"flag"
	"log"

	"bufio"
	"fmt"
	"os"
	"os/exec"

	"github.com/songgao/water"
)

var clientSocketListenPort = "21007"

var clientTunDstIP = "120.240.47.66"
var clientTunDstPort = 12272
var clientTunSrcIP = "10.1.1.2"
var clientTunSrcPort = 8888
var queueLen = 600

var serverTunSrcPort = clientTunDstPort
var serverTunSrcIP = "10.1.1.2"
var serverSocketTo = "127.0.0.1:21007"
var seqNum uint32

func main() {
	isServer := flag.Bool("s", false, "is server")
	fClientTunDstIP := flag.String("clientTunDstIP", "", "dst ip")
	fClientTunDstPort := flag.Int("clientTunDstPort", 0, "dst port")
	fQueueLen := flag.Int("queueLen", 0, "queue len")
	flag.Parse()

	seqNum = 1024
	clientTunDstIP = *fClientTunDstIP
	clientTunDstPort = *fClientTunDstPort
	serverTunSrcPort = clientTunDstPort
	queueLen = *fQueueLen

	tun := createTUN("faketcp")

	cmd := exec.Command("ip", "address", "add", "10.1.1.1/24", "dev", "faketcp")
	cmd.Run()
	cmd = exec.Command("ip", "link", "set", "up", "dev", "faketcp")
	cmd.Run()

	if *isServer {
		serverHandShake(tun)
		go serverTunToSocket(tun)
		go serverSocketToQueue(serverSocketTo, serverTunSrcPort)
		go serverQueueToTun(tun)
	} else {
		fmt.Println("server reader?")
		bufio.NewReader(os.Stdin).ReadString('\n')
		handShake(tun)
		go clientTunToSocket(tun)
		go clientSocketToQueue(clientSocketListenPort)
		go clientQueueToTun(tun, clientTunDstIP, clientTunDstPort)
	}
	reader := bufio.NewReader(os.Stdin)
	reader.ReadString('\n')

	if *isServer {
		fmt.Println("server drop ", serverDrop)
		fmt.Println("server send ", serverSendCount)
		fmt.Println("server recieve count ", serverReceiveCount)
		fmt.Println("serverTunToSocketReadMaxLen ", serverTunToSocketReadMaxLen)
		fmt.Println("serverSocketReadMaxLen", serverSocketReadMaxLen)
	} else {
		fmt.Println("client drop ", clientDrop)
		fmt.Println("client send count ", clientSendCount)
		fmt.Println("client receive count ", clientReceiveCount)
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

func checkError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %s\n", err.Error())
		os.Exit(1)
	}
}
