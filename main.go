package main

import (
	"flag"
	"log"
	"time"

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
var mSegCount = 1
var mFecCount = 1
var mGap = 0
var mReport = false
var mTimeoutMilli = 20

var serverTunSrcPort = clientTunDstPort
var serverTunSrcIP = "10.1.1.2"
var serverSocketTo = "127.0.0.1:21007"
var fecCacheSize = 5000

//todo 发送延迟和接收延迟分别定义

var mCodec *fecCodec
var timeOutCount = 0

func main() {
	go func() {
		http.ListenAndServe(":8080", nil)
	}()
	isServer := flag.Bool("s", false, "is server")
	fClientTunDstIP := flag.String("dip", "", "client set,server ip")
	fClientTunDstPort := flag.Int("dport", 0, "client set, server port")
	fQueueLen := flag.Int("qlen", 0, "queue len")
	fEFec := flag.Bool("fec", false, "server and client,enable fec")
	fSegCount := flag.Int("seg", 1, "server one packet segment into")
	fFecCount := flag.Int("fseg", 1, "server fec segment count")
	fFecGap := flag.Int("gap", 0, "fec packet send time gap")
	fReport := flag.Bool("re", false, "get report")
	fCacheSize := flag.Int("fcs", 5000, "fec list cache size")
	fTimeoutMilli := flag.Int("timeout", 20, "time out in millis")
	flag.Parse()

	clientTunDstIP = *fClientTunDstIP
	clientTunDstPort = *fClientTunDstPort
	serverTunSrcPort = clientTunDstPort
	queueLen = *fQueueLen
	eFec = *fEFec
	mSegCount = *fSegCount
	mFecCount = *fFecCount
	mGap = *fFecGap
	mReport = *fReport
	fecCacheSize = *fCacheSize
	mTimeoutMilli = *fTimeoutMilli

	tun := createTUN("faketcp")

	cmd := exec.Command("ip", "address", "add", "10.1.1.1/24", "dev", "faketcp")
	cmd.Run()
	cmd = exec.Command("ip", "link", "set", "up", "dev", "faketcp")
	cmd.Run()

	mCodec = newFecCodec(mSegCount, mFecCount, fecCacheSize)

	if *isServer {
		serverHandShake(tun)
		//接收
		if eFec {
			go serverTunToSocketFEC(tun)
		} else {
			go serverTunToSocket(tun)
		}
		//发送
		if eFec {
			go serverSocketToQueueFEC(serverSocketTo, serverTunSrcPort)
		} else {
			go serverSocketToQueueNoFEC(serverSocketTo, serverTunSrcPort)
		}
		go serverQueueToTun(tun)
	} else {
		fmt.Println("server reader?")
		bufio.NewReader(os.Stdin).ReadString('\n')
		chs := newClientHandshak(time.Duration(2)*time.Second, tun)
		chs.startListen()
		chs.sendSYN()

		for {
			time.Sleep(time.Duration(1) * time.Second)
			if chs.checkConn() {
				break
			}
			chs.sendSYN()
			fmt.Println("client handshake retry")
		}

		chs.sendAck()

		//接收
		if eFec {
			go clientTunToSocketFEC(tun)
		} else {
			go clientTunToSocketNoFEC(tun)
		}
		//发送
		if eFec {
			go clientSocketToQueueFEC(clientSocketListenPort, clientTunDstIP, clientTunDstPort)
		} else {
			go clientSocketToQueue(clientSocketListenPort, clientTunDstIP, clientTunDstPort)
		}
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
	}
	fmt.Println("timeout count ", timeOutCount)
	if mReport {
		mCodec.dump()
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
