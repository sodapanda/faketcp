package main

import (
	"flag"
	"log"

	"bufio"
	"fmt"
	"os"

	"github.com/songgao/water"
)

var clientSocketListenPort = "21007"

var clientTunDstIP = "192.168.8.10"
var clientTunDstPort = 12272
var clientTunSrcIP = "10.1.1.2"
var clientTunSrcPort = 8888

var serverTunSrcPort = clientTunDstPort
var serverTunSrcIP = "10.1.1.2"
var serverSocketTo = "127.0.0.1:21007"
var seqNum uint32

func main() {
	isServer := flag.Bool("s", false, "is server")
	flag.Parse()
	seqNum = 1024

	tun := createTUN("faketcp")

	fmt.Println("setup tun ip ")
	bufio.NewReader(os.Stdin).ReadString('\n')

	if *isServer {
		serverHandShake(tun)
		go serverTunToSocket(tun)
		go serverSocketToQueue(serverSocketTo)
		go serverQueueToTun(tun, serverTunSrcPort)
	} else {
		handShake(tun)
		go clientTunToSocket(tun)
		go clientSocketToQueue(clientSocketListenPort)
		go clientQueueToTun(tun, clientTunDstIP, clientTunDstPort)
	}
	reader := bufio.NewReader(os.Stdin)
	reader.ReadString('\n')
	fmt.Println("server drop ", serverDrop)
	fmt.Println("client drop ", clientDrop)
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
