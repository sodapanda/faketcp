package main

import (
	"github.com/google/netstack/tcpip"
	"github.com/google/netstack/tcpip/header"
	"github.com/google/netstack/tcpip/transport/tcp"
)

var ipHeaderLength = 20

//FPacket IP and TCP header info
type FPacket struct {
	srcIP   []byte
	dstIP   []byte
	srcPort uint16
	dstPort uint16
	syn     bool
	ack     bool
	seqNum  uint32
	ackNum  uint32
}

func craftPacket(payload []byte, result []byte, fPacket *FPacket) int {
	ipPacket := header.IPv4(result[0:])
	//IPv4 header
	ipHeader := header.IPv4Fields{}
	ipHeader.IHL = header.IPv4MinimumSize
	ipHeader.TOS = 0
	ipHeader.TotalLength = uint16(header.IPv4MinimumSize + header.TCPMinimumSize + len(payload))
	ipHeader.ID = 10
	ipHeader.Flags = 0b010
	ipHeader.FragmentOffset = 0
	ipHeader.TTL = 60
	ipHeader.Protocol = 6
	ipHeader.Checksum = 0
	ipHeader.SrcAddr = tcpip.Address(fPacket.srcIP)
	ipHeader.DstAddr = tcpip.Address(fPacket.dstIP)

	ipPacket.Encode(&ipHeader)
	ipPacket.SetChecksum(^ipPacket.CalculateChecksum())

	//TCP header
	tcpPacket := header.TCP(result[header.IPv4MinimumSize:])
	tcpHeader := header.TCPFields{}
	tcpHeader.SrcPort = fPacket.srcPort
	tcpHeader.DstPort = fPacket.dstPort
	tcpHeader.SeqNum = fPacket.seqNum
	tcpHeader.AckNum = fPacket.ackNum
	tcpHeader.DataOffset = header.TCPMinimumSize
	var tcpFlag uint8
	if fPacket.syn {
		tcpFlag = header.TCPFlagSyn
	}
	if fPacket.ack {
		tcpFlag = tcpFlag | header.TCPFlagAck
	}
	tcpHeader.Flags = tcpFlag
	tcpHeader.WindowSize = 16000
	tcpHeader.Checksum = 0
	tcpHeader.UrgentPointer = 0

	tcpPacket.Encode(&tcpHeader)
	xsum := header.PseudoHeaderChecksum(tcp.ProtocolNumber,
		tcpip.Address(fPacket.srcIP),
		tcpip.Address(fPacket.dstIP),
		uint16(header.TCPMinimumSize+len(payload)))
	xsum = header.Checksum(payload, xsum)
	tcpPacket.SetChecksum(^tcpPacket.CalculateChecksum(xsum))

	//payload
	copy(result[header.IPv4MinimumSize+header.TCPMinimumSize:], payload)
	return header.IPv4MinimumSize + header.TCPMinimumSize + len(payload)
}
