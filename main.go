package main

import (
	"flag"
	"log"

	"bufio"
	"fmt"
	"os"
	"os/exec"

	"net/http"
	_ "net/http/pprof"

	"github.com/songgao/water"
)

var clientSocketListenPort = "21007"

var clientTunDstIP = "120.240.47.66"
var clientTunDstPort = 12272
var clientTunSrcIP = "10.1.1.2"
var clientTunSrcPort = 8888
var queueLen = 600
var eFec = false

var serverTunSrcPort = clientTunDstPort
var serverTunSrcIP = "10.1.1.2"
var serverSocketTo = "127.0.0.1:21007"

//todo 发送延迟和接收延迟分别定义

func main() {
	go func() {
		http.ListenAndServe(":8080", nil)
	}()
	isServer := flag.Bool("s", false, "is server")
	fClientTunDstIP := flag.String("dip", "", "client set,server ip")
	fClientTunDstPort := flag.Int("dport", 0, "client set, server port")
	fQueueLen := flag.Int("qlen", 0, "queue len")
	fEFec := flag.Bool("fec", false, "server and client,enable fec")
	flag.Parse()

	clientTunDstIP = *fClientTunDstIP
	clientTunDstPort = *fClientTunDstPort
	serverTunSrcPort = clientTunDstPort
	queueLen = *fQueueLen
	eFec = *fEFec

	tun := createTUN("faketcp")

	cmd := exec.Command("ip", "address", "add", "10.1.1.1/24", "dev", "faketcp")
	cmd.Run()
	cmd = exec.Command("ip", "link", "set", "up", "dev", "faketcp")
	cmd.Run()

	if *isServer {
		serverHandShake(tun)
		//接收
		go serverTunToSocket(tun)
		go serverSocketToQueue(serverSocketTo, serverTunSrcPort)
		go serverQueueToTun(tun)
	} else {
		fmt.Println("server reader?")
		bufio.NewReader(os.Stdin).ReadString('\n')
		handShake(tun)
		//接收
		go clientTunToSocket(tun)
		//发送
		go clientSocketToQueue(clientSocketListenPort, clientTunDstIP, clientTunDstPort)
		go clientQueueToTun(tun)
	}
	reader := bufio.NewReader(os.Stdin)
	reader.ReadString('\n')

	if *isServer {
		fmt.Println("server drop ", serverDrop)
		fmt.Println("server send ", serverSendCount)
		fmt.Println("server recieve count ", serverReceiveCount)
	} else {
		fmt.Println("client drop ", clientDrop)
		fmt.Println("client send count ", clientSendCount)
		fmt.Println("client receive count ", clientReceiveCount)
		fmt.Println("client reconstruct ", decodeCount)
		if eFec {
			fecRcv.dump()
		}
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
