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
	payload []byte
}

func craftPacket(packet []byte, fPacket *FPacket) int {
	payload := packet[(header.IPv4MinimumSize + header.TCPMinimumSize):]
	ipPacket := header.IPv4(packet[0:])
	//IPv4 header
	ipHeader := header.IPv4Fields{}
	ipHeader.IHL = header.IPv4MinimumSize
	ipHeader.TOS = 0
	ipHeader.TotalLength = uint16(len(packet))
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
	tcpPacket := header.TCP(packet[header.IPv4MinimumSize:])
	tcpHeader := header.TCPFields{}
	tcpHeader.SrcPort = fPacket.srcPort
	tcpHeader.DstPort = fPacket.dstPort
	tcpHeader.SeqNum = fPacket.seqNum
	tcpHeader.AckNum = fPacket.ackNum
	tcpHeader.DataOffset = header.TCPMinimumSize
	tcpHeader.Flags = 0
	if fPacket.syn {
		tcpHeader.Flags = tcpHeader.Flags | header.TCPFlagSyn
	}
	if fPacket.ack {
		tcpHeader.Flags = tcpHeader.Flags | header.TCPFlagAck
	}
	tcpHeader.WindowSize = 65000
	tcpHeader.Checksum = 0
	tcpHeader.UrgentPointer = 0

	tcpPacket.Encode(&tcpHeader)
	xsum := header.PseudoHeaderChecksum(tcp.ProtocolNumber,
		tcpip.Address(fPacket.srcIP),
		tcpip.Address(fPacket.dstIP),
		uint16(len(packet)-header.IPv4MinimumSize))
	xsum = header.Checksum(payload, xsum)
	tcpPacket.SetChecksum(^tcpPacket.CalculateChecksum(xsum))

	//payload
	return len(packet)
}

func unpacket(data []byte, fPacket *FPacket) {
	ipHeader := header.IPv4(data)
	tcpHeader := header.TCP(data[header.IPv4MinimumSize:])

	copy(fPacket.srcIP, ipHeader.SourceAddress().To4())
	copy(fPacket.dstIP, ipHeader.DestinationAddress().To4())

	fPacket.srcPort = tcpHeader.SourcePort()
	fPacket.dstPort = tcpHeader.DestinationPort()
	fPacket.syn = tcpHeader.Flags()&header.TCPFlagSyn != 0
	fPacket.ack = tcpHeader.Flags()&header.TCPFlagAck != 0
	fPacket.seqNum = tcpHeader.SequenceNumber()
	fPacket.ackNum = tcpHeader.AckNumber()
	fPacket.payload = data[header.IPv4MinimumSize+header.TCPMinimumSize:]
}
