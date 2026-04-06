package model

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
)

// FlowKey 五元组，IP 以 16 字节存储（IPv4 为 v4-mapped IPv6 形式）。
type FlowKey struct {
	SrcIP   [16]byte
	DstIP   [16]byte
	SrcPort uint16
	DstPort uint16
	Proto   uint8
}

func (k FlowKey) Bytes() []byte {
	b := make([]byte, 16+16+2+2+1)
	copy(b[0:16], k.SrcIP[:])
	copy(b[16:32], k.DstIP[:])
	binary.LittleEndian.PutUint16(b[32:34], k.SrcPort)
	binary.LittleEndian.PutUint16(b[34:36], k.DstPort)
	b[36] = k.Proto
	return b
}

func FlowKeyFromBytes(b []byte) (FlowKey, bool) {
	if len(b) < 37 {
		return FlowKey{}, false
	}
	var k FlowKey
	copy(k.SrcIP[:], b[0:16])
	copy(k.DstIP[:], b[16:32])
	k.SrcPort = binary.LittleEndian.Uint16(b[32:34])
	k.DstPort = binary.LittleEndian.Uint16(b[34:36])
	k.Proto = b[36]
	return k, true
}

func IPv4Key(a, b, c, d byte) [16]byte {
	var buf [16]byte
	buf[10] = 0xff
	buf[11] = 0xff
	buf[12] = a
	buf[13] = b
	buf[14] = c
	buf[15] = d
	return buf
}

func FlowKeyHex(k FlowKey) string {
	return net.IP(k.SrcIP[:]).String() + "|" + net.IP(k.DstIP[:]).String() + "|" +
		strconv.Itoa(int(k.SrcPort)) + "|" + strconv.Itoa(int(k.DstPort)) + "|" + strconv.Itoa(int(k.Proto))
}

type flowKeyJSON struct {
	SIP   string `json:"sip"`
	DIP   string `json:"dip"`
	SP    uint16 `json:"sp"`
	DP    uint16 `json:"dp"`
	Proto uint8  `json:"proto"`
}

func (k FlowKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(flowKeyJSON{
		SIP:   net.IP(k.SrcIP[:]).String(),
		DIP:   net.IP(k.DstIP[:]).String(),
		SP:    k.SrcPort,
		DP:    k.DstPort,
		Proto: k.Proto,
	})
}

func (k *FlowKey) UnmarshalJSON(b []byte) error {
	var j flowKeyJSON
	if err := json.Unmarshal(b, &j); err != nil {
		return err
	}
	sip := net.ParseIP(j.SIP)
	dip := net.ParseIP(j.DIP)
	if sip == nil || dip == nil {
		return fmt.Errorf("flow_key: invalid ip")
	}
	sip = sip.To16()
	dip = dip.To16()
	if sip == nil || dip == nil {
		return fmt.Errorf("flow_key: ip to16")
	}
	copy(k.SrcIP[:], sip)
	copy(k.DstIP[:], dip)
	k.SrcPort = j.SP
	k.DstPort = j.DP
	k.Proto = j.Proto
	return nil
}

// PacketRecord 标准化后的包记录，可 JSON 序列化进 Kafka。
type PacketRecord struct {
	TsNanos int64   `json:"ts_nanos"`
	Len     int     `json:"len"`
	Flow    FlowKey `json:"flow"`
}

func (p PacketRecord) MarshalBinary() ([]byte, error) {
	return json.Marshal(p)
}

func UnmarshalPacketRecord(data []byte) (PacketRecord, error) {
	var p PacketRecord
	err := json.Unmarshal(data, &p)
	return p, err
}

// FlowStats 单流窗口内统计。
type FlowStats struct {
	Bytes uint64
	Pkts  uint64
	First int64
	Last  int64
}

// WindowSnapshot 一个时间窗口的汇总结果（落库/缓存）。
type WindowSnapshot struct {
	WindowStart int64
	WindowEnd   int64
	ProbeID     string

	FlowsObserved int64
	TotalPackets  uint64
	TotalBytes    uint64

	TopFlows []FlowTopEntry

	Spread []SpreadEntry

	// SrcFlows：src 为流标签的每窗口统计
	SrcFlows []SrcFlowEntry
}

type FlowTopEntry struct {
	Key   FlowKey
	Bytes uint64
	Pkts  uint64
	Rank  int
}

type SpreadEntry struct {
	SrcIP               string
	DstCardinalityEst uint64
}

// SrcFlowEntry：流标签 = src（同一 src 的包属于同一条流）。
// FlowSize：流大小 = 该窗口内该 src 的包数（每包对大小 +1）。
// DstPortCardinalityEst：流基数 = 该 src 下不同 dst port 的个数（HyperLogLog 估计）。
type SrcFlowEntry struct {
	SrcIP                   string
	FlowSize                uint64
	DstPortCardinalityEst   uint64
}

