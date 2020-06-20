package main

import (
	"flag"
	"log"

	"bufio"
	"fmt"
	"os"

	"github.com/songgao/water"
)

func main() {
	isServer := flag.Bool("s", false, "is server")
	flag.Parse()

	tun := createTUN("faketcp")

	if *isServer {
		go serverTunToSocket(tun)
		go serverSocketToTun(tun, "127.0.0.1:21007", 12271)
	} else {
		go clientTunToSocket(tun)
		go clientSocketToTun("21007", tun, "45.117.103.179", 12271)
	}
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

func checkError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %s\n", err.Error())
		os.Exit(1)
	}
}
