package main

import (
	"flag"
	"io"
	"io/ioutil"
	"log"
	"strings"
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

var serverTunSrcPort = clientTunDstPort
var serverTunSrcIP = "10.1.1.2"
var serverSocketTo = "127.0.0.1:21007"
var mSb strings.Builder
var enableLog = false
var enableVerbose bool
var sendDelay int
var recDelay int

var debugSendSb strings.Builder
var debugRecSb strings.Builder
var enableDebugLog bool

//todo 发送延迟和接收延迟分别定义

func main() {
	go func() {
		http.ListenAndServe(":8080", nil)
	}()
	isServer := flag.Bool("s", false, "is server")
	fClientTunDstIP := flag.String("clientTunDstIP", "", "dst ip")
	fClientTunDstPort := flag.Int("clientTunDstPort", 0, "dst port")
	fQueueLen := flag.Int("queueLen", 0, "queue len")
	fRecDelay := flag.Int("recDelay", 0, "rec delay")
	fVerbos := flag.Bool("verb", false, "verbose")
	fSendDelay := flag.Int("sendDelay", 0, "redunt send delay")
	fEnableDebug := flag.Bool("debug", false, "enable debug log")
	flag.Parse()

	clientTunDstIP = *fClientTunDstIP
	clientTunDstPort = *fClientTunDstPort
	serverTunSrcPort = clientTunDstPort
	recDelay = *fRecDelay
	queueLen = *fQueueLen
	enableVerbose = *fVerbos
	sendDelay = *fSendDelay
	enableDebugLog = *fEnableDebug

	tun := createTUN("faketcp")

	cmd := exec.Command("ip", "address", "add", "10.1.1.1/24", "dev", "faketcp")
	cmd.Run()
	cmd = exec.Command("ip", "link", "set", "up", "dev", "faketcp")
	cmd.Run()

	if *isServer {
		serverHandShake(tun)
		//接收
		if recDelay > 0 {
			go serverTunToQueue(tun)
			go serverQueueToSocket()
		} else {
			go serverTunToSocket(tun)
		}

		//发送
		if recDelay > 0 {
			go reduntWorker(tun)
		}
		go serverSocketToQueue(serverSocketTo, serverTunSrcPort)
		go serverQueueToTun(tun)
	} else {
		fmt.Println("server reader?")
		bufio.NewReader(os.Stdin).ReadString('\n')
		handShake(tun)
		//接收
		if recDelay > 0 {
			go clientTunToQueue(tun)
			go clientQueueToSocket()
		} else {
			go clientTunToSocket(tun)
		}
		//发送
		if recDelay > 0 {
			go clientReduntWork(tun)
		}
		if enableVerbose {
			go iLog()
		}
		go clientSocketToQueue(clientSocketListenPort, clientTunDstIP, clientTunDstPort)
		go clientQueueToTun(tun)
	}
	reader := bufio.NewReader(os.Stdin)
	reader.ReadString('\n')

	if *isServer {
		fmt.Println("server drop ", serverDrop)
		fmt.Println("server send ", serverSendCount)
		fmt.Println("server recieve count ", serverReceiveCount)
		if enableLog {
			ioutil.WriteFile("serversend.txt", []byte(mSb.String()), 0644)
		}
		if enableDebugLog {
			ioutil.WriteFile("serverSendTime.csv", []byte(debugSendSb.String()), 0644)
			ioutil.WriteFile("serverRecTime.csv", []byte(debugRecSb.String()), 0644)
		}
	} else {
		fmt.Println("client drop ", clientDrop)
		fmt.Println("client send count ", clientSendCount)
		fmt.Println("client receive count ", clientReceiveCount)
		if enableLog {
			ioutil.WriteFile("clientread.txt", []byte(mSb.String()), 0644)

			serverLog, _ := os.Create("serversend.txt")
			resp, _ := http.Get("http://103.73.255.122/serversend.txt")
			io.Copy(serverLog, resp.Body)

			outPut, _ := exec.Command("python3", "compare.py").Output()
			fmt.Printf("out put %s\n", outPut)
		}

		if enableDebugLog {
			ioutil.WriteFile("clientSendTime.csv", []byte(debugSendSb.String()), 0644)
			ioutil.WriteFile("clientRecTime.csv", []byte(debugRecSb.String()), 0644)
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

func iLog() {
	for {
		lep := emptyPutCount
		lrc := reduntCount
		lreorderCount := reorderCount
		lpushbackCount := pushbackCount
		ltimeoutCount := timeoutCount
		lclientReceiveCount := clientReceiveCount

		time.Sleep(1 * time.Second)
		fmt.Println("空队放 ", emptyPutCount-lep,
			" 冗余包 ", reduntCount-lrc,
			" 队长 ", clientTunToSocketQueue.dataList.Len(),
			" 重排 ", reorderCount-lreorderCount,
			" 放队尾 ", pushbackCount-lpushbackCount,
			" 超时包 ", timeoutCount-ltimeoutCount,
			" 发上层 ", clientReceiveCount-lclientReceiveCount,
			" poolE ", poolWrongFlag)
	}
}
