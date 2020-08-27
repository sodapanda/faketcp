package main

/*
客户端心跳处理
*/
type clientHearBeat struct {
}

func newClientHeartBeat() *clientHearBeat {
	chb := new(clientHearBeat)
	return chb
}

func (chb *clientHearBeat) start(rst chan string) {
	//todo 调用channel
}

/*
服务端心跳处理
*/
type serverHeartBeat struct {
}

func newServerHeartBeat() *serverHeartBeat {
	shb := new(serverHeartBeat)
	return shb
}

func (shb *serverHeartBeat) start(rst chan string) {
	//todo 调用channel
}
