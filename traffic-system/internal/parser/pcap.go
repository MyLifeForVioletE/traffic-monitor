package parser

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"

	"trafficd/internal/model"
)

// RunFile 使用纯 Go 的 pcapgo 读取离线 PCAP（无需 libpcap/CGO）
// 参数：
//   - ctx: 上下文
//   - path: pcap 文件路径
//   - out: 输出 channel
//   - batchSize: 批处理大小
//
// 返回：
//   - error: 错误信息
func RunFile(ctx context.Context, path string, out chan<- []model.PacketRecord, batchSize int) error {
	if batchSize <= 0 {
		batchSize = 1024
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open pcap: %w", err)
	}
	defer f.Close()

	r, err := pcapgo.NewReader(f)
	if err != nil {
		return fmt.Errorf("pcap reader: %w", err)
	}

	batch := make([]model.PacketRecord, 0, batchSize)
	// flush 刷新批处理数据
	flush := func() {
		if len(batch) == 0 {
			return
		}
		cp := make([]model.PacketRecord, len(batch))
		copy(cp, batch)
		select {
		case <-ctx.Done():
			return
		case out <- cp:
		}
		batch = batch[:0]
	}

	for {
		if ctx.Err() != nil {
			flush()
			return ctx.Err()
		}
		data, ci, err := r.ReadPacketData()
		if err == io.EOF {
			flush()
			return nil
		}
		if err != nil {
			flush()
			return fmt.Errorf("read packet: %w", err)
		}
		pkt := gopacket.NewPacket(data, layers.LayerTypeEthernet, gopacket.Default)
		rec, ok := packetToRecord(pkt, ci.Timestamp, len(data))
		if !ok {
			continue
		}
		batch = append(batch, rec)
		if len(batch) >= batchSize {
			flush()
		}
	}
}

// packetToRecord 将 gopacket 包转换为 PacketRecord
// 支持 IPv4/IPv6：将源/目的地址统一成 16 bytes（IPv4 为 v4-mapped IPv6）
// 参数：
//   - pkt: gopacket 包
//   - ts: 时间戳
//   - capLen: 捕获长度
//
// 返回：
//   - model.PacketRecord: 包记录
//   - bool: 是否成功
func packetToRecord(pkt gopacket.Packet, ts time.Time, capLen int) (model.PacketRecord, bool) {
	var sip, dip []byte
	// 解析 IPv4
	if ip4Layer := pkt.Layer(layers.LayerTypeIPv4); ip4Layer != nil {
		ip4, _ := ip4Layer.(*layers.IPv4)
		sip = ip4.SrcIP.To16()
		dip = ip4.DstIP.To16()
	} else if ip6Layer := pkt.Layer(layers.LayerTypeIPv6); ip6Layer != nil {
		// 解析 IPv6
		ip6, _ := ip6Layer.(*layers.IPv6)
		sip = ip6.SrcIP.To16()
		dip = ip6.DstIP.To16()
	} else {
		return model.PacketRecord{}, false
	}
	if sip == nil || dip == nil {
		return model.PacketRecord{}, false
	}

	var proto uint8
	var sport, dport uint16

	// 解析 TCP
	if tcpLayer := pkt.Layer(layers.LayerTypeTCP); tcpLayer != nil {
		tcp, _ := tcpLayer.(*layers.TCP)
		proto = uint8(layers.IPProtocolTCP)
		sport = uint16(tcp.SrcPort)
		dport = uint16(tcp.DstPort)
	} else if udpLayer := pkt.Layer(layers.LayerTypeUDP); udpLayer != nil {
		// 解析 UDP
		udp, _ := udpLayer.(*layers.UDP)
		proto = uint8(layers.IPProtocolUDP)
		sport = uint16(udp.SrcPort)
		dport = uint16(udp.DstPort)
	} else {
		return model.PacketRecord{}, false
	}

	// 构建五元组
	var fk model.FlowKey
	copy(fk.SrcIP[:], sip)
	copy(fk.DstIP[:], dip)
	fk.SrcPort = sport
	fk.DstPort = dport
	fk.Proto = proto

	return model.PacketRecord{
		TsNanos: ts.UnixNano(),
		Len:     capLen,
		Flow:    fk,
	}, true
}
