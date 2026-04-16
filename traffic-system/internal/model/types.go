package model

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
)

// FlowKey 五元组结构体
// IP 以 16 字节存储（IPv4 为 v4-mapped IPv6 形式）
type FlowKey struct {
	SrcIP   [16]byte // 源 IP 地址
	DstIP   [16]byte // 目的 IP 地址
	SrcPort uint16   // 源端口
	DstPort uint16   // 目的端口
	Proto   uint8    // 协议号（6=TCP, 17=UDP）
}

// Bytes 将 FlowKey 序列化为字节数组
// 返回：
//   - []byte: 序列化后的字节数组
func (k FlowKey) Bytes() []byte {
	b := make([]byte, 16+16+2+2+1)
	copy(b[0:16], k.SrcIP[:])
	copy(b[16:32], k.DstIP[:])
	binary.LittleEndian.PutUint16(b[32:34], k.SrcPort)
	binary.LittleEndian.PutUint16(b[34:36], k.DstPort)
	b[36] = k.Proto
	return b
}

// FlowKeyFromBytes 从字节数组反序列化为 FlowKey
// 参数：
//   - b: 字节数组
//
// 返回：
//   - FlowKey: 五元组
//   - bool: 是否成功
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

// IPv4Key 创建 IPv4 地址的 16 字节表示（v4-mapped IPv6 形式）
// 参数：
//   - a, b, c, d: IPv4 地址的四个字节
//
// 返回：
//   - [16]byte: 16 字节表示
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

// FlowKeyHex 将 FlowKey 转换为十六进制字符串格式
// 格式：SIP|DIP|SP|DP|Proto
// 参数：
//   - k: 五元组
//
// 返回：
//   - string: 十六进制字符串
func FlowKeyHex(k FlowKey) string {
	return net.IP(k.SrcIP[:]).String() + "|" + net.IP(k.DstIP[:]).String() + "|" +
		strconv.Itoa(int(k.SrcPort)) + "|" + strconv.Itoa(int(k.DstPort)) + "|" + strconv.Itoa(int(k.Proto))
}

// flowKeyJSON JSON 序列化结构体
type flowKeyJSON struct {
	SIP   string `json:"sip"`   // 源 IP
	DIP   string `json:"dip"`   // 目的 IP
	SP    uint16 `json:"sp"`    // 源端口
	DP    uint16 `json:"dp"`    // 目的端口
	Proto uint8  `json:"proto"` // 协议号
}

// MarshalJSON 将 FlowKey 序列化为 JSON
// 返回：
//   - []byte: JSON 字节数组
//   - error: 错误信息
func (k FlowKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(flowKeyJSON{
		SIP:   net.IP(k.SrcIP[:]).String(),
		DIP:   net.IP(k.DstIP[:]).String(),
		SP:    k.SrcPort,
		DP:    k.DstPort,
		Proto: k.Proto,
	})
}

// UnmarshalJSON 从 JSON 反序列化为 FlowKey
// 参数：
//   - b: JSON 字节数组
//
// 返回：
//   - error: 错误信息
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

// PacketRecord 标准化后的包记录结构体
// 可 JSON 序列化进 Kafka
type PacketRecord struct {
	TsNanos int64   `json:"ts_nanos"` // 时间戳（纳秒）
	Len     int     `json:"len"`      // 包长度
	Flow    FlowKey `json:"flow"`     // 五元组
}

// MarshalBinary 将 PacketRecord 序列化为二进制
// 返回：
//   - []byte: 二进制数据
//   - error: 错误信息
func (p PacketRecord) MarshalBinary() ([]byte, error) {
	return json.Marshal(p)
}

// UnmarshalPacketRecord 从二进制反序列化为 PacketRecord
// 参数：
//   - data: 二进制数据
//
// 返回：
//   - PacketRecord: 包记录
//   - error: 错误信息
func UnmarshalPacketRecord(data []byte) (PacketRecord, error) {
	var p PacketRecord
	err := json.Unmarshal(data, &p)
	return p, err
}

// FlowStats 单流窗口内统计信息
type FlowStats struct {
	Bytes uint64 // 字节数
	Pkts  uint64 // 包数
	First int64  // 首个包的时间戳
	Last  int64  // 最后一个包的时间戳
}

// WindowSnapshot 一个时间窗口的汇总结果结构体
// 用于落库/缓存
type WindowSnapshot struct {
	WindowStart int64  // 窗口起始时间（纳秒）
	WindowEnd   int64  // 窗口结束时间（纳秒）
	ProbeID     string // 探针 ID

	FlowsObserved int64  // 观察到的流数量
	TotalPackets  uint64 // 总包数
	TotalBytes    uint64 // 总字节数

	TopFlows []FlowTopEntry // Top 流（已废弃）

	// ...已移除异常流量相关字段...

	// SrcFlows：src 为流标签的每窗口统计
	SrcFlows []SrcFlowEntry // 源流条目列表
}

// FlowTopEntry Top 流条目（已废弃）
type FlowTopEntry struct {
	Key   FlowKey // 五元组
	Bytes uint64  // 字节数
	Pkts  uint64  // 包数
	Rank  int     // 排名
}

// SrcFlowEntry 源流条目结构体
// 流标签 = src（同一 src 的包属于同一条流）
// FlowSize：流大小 = 该窗口内该 src 的包数（每包对大小 +1）
type SrcFlowEntry struct {
	SrcIP             string // 源 IP 地址
	FlowSize          uint64 // 流大小（包数）
	DstCardinalityEst uint64 // 目的端口基数估计
}
