package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
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

var mCodec *fecCodec
var timeOutCount = 0

var mConfig Config

func main() {
	go func() {
		http.ListenAndServe(":8080", nil)
	}()

	fConfigPath := flag.String("c", "config.json", "config file path")
	flag.Parse()
	configPath := *fConfigPath

	configFile, err := os.Open(configPath)
	checkError(err)
	defer configFile.Close()
	configByte, _ := ioutil.ReadAll(configFile)
	json.Unmarshal(configByte, &mConfig)

	tun := createTUN("faketcp")

	cmd := exec.Command("ip", "address", "add", "10.1.1.1/24", "dev", "faketcp")
	cmd.Run()
	cmd = exec.Command("ip", "link", "set", "up", "dev", "faketcp")
	cmd.Run()

	mCodec = newFecCodec(mConfig.SegCount, mConfig.FecCount, mConfig.FecCacheSize)

	go func() {
		if mConfig.Server {
			//如果收到客户端要重连的请求，关闭其他操作之后执行握手
			sh := newServerHandshak(tun)
			sh.waitSyn() //这个不能在loop里，因为之所以重复循环是因为已经收到了syn包 不用再等了
			for {
				//握手
				sh.sendSynAck()
				sh.waitAck()

				serverTuntoSocketChan := make(chan string)
				serverSocketToQueueChan := make(chan string)
				serverQueueToTunChan := make(chan string)
				serverHeartBeatChan := make(chan string)

				//心跳
				shb := newServerHeartBeat()
				go shb.start(serverHeartBeatChan)
				//接收
				if mConfig.EnableFEC {
					go serverTunToSocketFEC(tun, serverTuntoSocketChan)
				} else {
					go serverTunToSocket(tun, serverTuntoSocketChan)
				}
				//发送
				if mConfig.EnableFEC {
					go serverSocketToQueueFEC(mConfig.ServerSocketTo, mConfig.ServerTunPort, serverSocketToQueueChan)
				} else {
					go serverSocketToQueueNoFEC(mConfig.ServerSocketTo, mConfig.ServerTunPort, serverSocketToQueueChan)
				}
				go serverQueueToTun(tun, serverQueueToTunChan)

				<-serverTuntoSocketChan

				serverStop()
				<-serverSocketToQueueChan
				<-serverQueueToTunChan
				<-serverHeartBeatChan
			}
		} else {
			mClt := newClient(tun)
			//如果心跳出问题了，那么其他的go都要退出，重新开始进入握手状态
			for {
				//握手
				chs := newClientHandshak(time.Duration(2)*time.Second, tun, mClt)
				chs.startListen()
				for {
					chs.sendSYN()
					time.Sleep(time.Duration(1) * time.Second)
					if chs.checkConn() {
						break
					}
					fmt.Println("client handshake retry")
				}
				chs.sendAck()

				//发送和接收
				chbRst := make(chan string)
				clientTunToSocketChan := make(chan string)
				clientSocketToQueueChan := make(chan string)
				clientQueueToTunChan := make(chan string)

				//接收
				if mConfig.EnableFEC {
					go mClt.tunToSocketFEC(clientTunToSocketChan)
				} else {
					go mClt.tunToSocketNoFEC(clientTunToSocketChan)
				}
				//发送
				if mConfig.EnableFEC {
					go mClt.socketToQueueFEC(clientSocketToQueueChan)
				} else {
					go mClt.socketToQueue(clientSocketToQueueChan)
				}
				go mClt.queueToTun(clientQueueToTunChan)
				//心跳
				chb := newClientHeartBeat()
				go chb.start(chbRst)

				//心跳出问题了
				<-chbRst
				//停掉所有的go
				stopClient()
				//等其他go都完事了，回到握手的状态
				<-clientTunToSocketChan
				<-clientSocketToQueueChan
				<-clientQueueToTunChan
			}
		}
	}()
	reader := bufio.NewReader(os.Stdin)
	reader.ReadString('\n')

	if mConfig.Server {
		fmt.Println("server drop ", serverDrop)
		fmt.Println("server send ", serverSendCount)
		fmt.Println("server recieve count ", serverReceiveCount)
	} else {
		// fmt.Println("client drop ", mClt.drop)
		// fmt.Println("client send count ", mClt.sent)
		// fmt.Println("client receive count ", mClt.recv)
	}
	fmt.Println("timeout count ", timeOutCount)
	if mConfig.Report {
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

//Config config
type Config struct {
	Server                 bool   `json:"server"`
	TunInterfaceIP         string `json:"tunInterfaceIP"`
	TunSrcIP               string `json:"TunSrcIP"`
	TunSrcPort             int    `json:"tunSrcPort"`
	QLen                   int    `json:"qLen"`
	EnableFEC              bool   `json:"enableFEC"`
	SegCount               int    `json:"segCount"`
	FecCount               int    `json:"fecCount"`
	Gap                    int    `json:"gap"`
	FecCacheSize           int    `json:"fecCacheSize"`
	StageTimeout           int    `json:"stageTimeout"`
	Report                 bool   `json:"report"`
	ClientSocketListenPort string `json:"clientSocketListenPort"`
	ClientTunToIP          string `json:"clientTunToIP"`
	ClientTunToPort        int    `json:"clientTunToPort"`
	ServerTunPort          int    `json:"serverTunPort"`
	ServerSocketTo         string `json:"serverSocketTo"`
}
