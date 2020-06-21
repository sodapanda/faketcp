package main

import (
	"net"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

func makePacket(pBuff gopacket.SerializeBuffer, srcIP net.IP, dstIP net.IP, srcPort int, dstPort int, syn bool, ack bool, seq uint32, ackNum uint32, payLoad []byte) []byte {
	opts := gopacket.SerializeOptions{
		ComputeChecksums: true,
		FixLengths:       true,
	}
	ipL := &layers.IPv4{
		Version:  4,
		TOS:      0,
		TTL:      60,
		Id:       10,
		Protocol: 6,
		Flags:    0b010,
		SrcIP:    srcIP,
		DstIP:    dstIP,
	}
	tcpL := &layers.TCP{
		SrcPort: layers.TCPPort(srcPort),
		DstPort: layers.TCPPort(dstPort),
		Seq:     seq,
		Ack:     ackNum,
		NS:      false,
		CWR:     false,
		ECE:     false,
		URG:     false,
		ACK:     ack,
		PSH:     false,
		RST:     false,
		SYN:     syn,
		FIN:     false,
		Window:  1600,
	}
	tcpL.SetNetworkLayerForChecksum(ipL)
	err := gopacket.SerializeLayers(pBuff, opts, ipL, tcpL, gopacket.Payload(payLoad))
	checkError(err)

	outPacket := pBuff.Bytes()
	return outPacket
}

func parsePacket(data []byte) *layers.TCP {
	packet := gopacket.NewPacket(data, layers.LayerTypeIPv4, gopacket.Default)

	if tcpLayer := packet.Layer(layers.LayerTypeTCP); tcpLayer != nil {
		tcp, _ := tcpLayer.(*layers.TCP)
		return tcp
	}
	return nil
}

func nextSeq() uint32 {
	if seqNum < 4294967295 {
		seqNum = seqNum + 1
		return seqNum
	}
	seqNum = 0
	return seqNum
}

//Linux的SNAT和DNAT要跟踪连接状态，序号是其中一个判断依据
func addAck(ack uint32) uint32 {
	if ack < 4294967295 {
		next := ack + 1
		return next
	}

	return 0
}

func getSrcIP(data []byte) net.IP {
	packet := gopacket.NewPacket(data, layers.LayerTypeIPv4, gopacket.Default)
	if ipLayer := packet.Layer(layers.LayerTypeIPv4); ipLayer != nil {
		ip := ipLayer.(*layers.IPv4)
		return ip.SrcIP
	}
	return nil
}
