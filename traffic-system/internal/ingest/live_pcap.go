//go:build windows && cgo

package ingest

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcap"

	"trafficd/internal/model"
)

// RunLive 实时抓包并输出 PacketRecord 批次
// 需要 Npcap + Windows cgo（libpcap）
// 参数：
//   - ctx: 上下文
//   - iface: 网卡名称
//   - ifaceCh: 网卡选择通道
//   - promiscuous: 是否混杂模式
//   - snapshotLen: 快照长度
//   - readTimeout: 读取超时
//   - bpf: BPF 过滤器
//   - batchSize: 批处理大小
//   - sink: 数据发布函数
//
// 返回：
//   - error: 错误��息
func RunLive(
	ctx context.Context,
	iface string,
	ifaceCh <-chan string,
	promiscuous bool,
	snapshotLen int,
	readTimeout time.Duration,
	bpf string,
	batchSize int,
	sink func(context.Context, []model.PacketRecord) error,
) error {
	// 设置默认参数
	if snapshotLen <= 0 {
		snapshotLen = 1600
	}
	if batchSize <= 0 {
		batchSize = 2048
	}
	if readTimeout <= 0 {
		readTimeout = 1000 * time.Millisecond
	}

	// 打开网卡句柄
	openHandle := func(currentIface string) (*pcap.Handle, *gopacket.PacketSource, error) {
		handle, err := pcap.OpenLive(currentIface, int32(snapshotLen), promiscuous, readTimeout)
		if err != nil {
			return nil, nil, fmt.Errorf("open live device %s: %w", currentIface, err)
		}
		if bpf != "" {
			if err := handle.SetBPFFilter(bpf); err != nil {
				handle.Close()
				return nil, nil, fmt.Errorf("set bpf: %w", err)
			}
		}
		src := gopacket.NewPacketSource(handle, handle.LinkType())
		src.NoCopy = true
		return handle, src, nil
	}

	// 等待下一个网卡选择
	waitNextIface := func() (string, error) {
		if ifaceCh == nil {
			return "", fmt.Errorf("live iface required")
		}
		for {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case next, ok := <-ifaceCh:
				if !ok {
					ifaceCh = nil
					return "", fmt.Errorf("live iface required")
				}
				next = strings.TrimSpace(next)
				if next == "" {
					continue
				}
				return next, nil
			}
		}
	}

	// 等待初始网卡
	currentIface := strings.TrimSpace(iface)
	for currentIface == "" {
		next, waitErr := waitNextIface()
		if waitErr != nil {
			return waitErr
		}
		currentIface = next
	}

	// 打开初始网卡
	var (
		handle *pcap.Handle
		src    *gopacket.PacketSource
		err    error
	)
	for {
		handle, src, err = openHandle(currentIface)
		if err == nil {
			break
		}
		next, waitErr := waitNextIface()
		if waitErr != nil {
			return err
		}
		currentIface = next
	}
	defer handle.Close()

	// 创建批处理缓冲区
	batch := make([]model.PacketRecord, 0, batchSize)
	var pktCount, parsedCount uint64

	// flush 刷新批处理数据到 sink
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		cp := make([]model.PacketRecord, len(batch))
		copy(cp, batch)
		batch = batch[:0]
		return sink(ctx, cp)
	}

	// 定期刷新
	flushInterval := time.NewTicker(500 * time.Millisecond)
	defer flushInterval.Stop()

	slog.Info("start capturing on interface", "iface", currentIface, "bpf", bpf)

	pkts := src.Packets()

	for {
		select {
		case <-ctx.Done():
			slog.Info("capture stopped", "total_pkts", pktCount, "parsed_pkts", parsedCount)
			return flush()
		case <-flushInterval.C:
			if err := flush(); err != nil {
				return err
			}
		case newIface, ok := <-ifaceCh:
			if !ok {
				ifaceCh = nil
				continue
			}
			newIface = strings.TrimSpace(newIface)
			if newIface == "" {
				continue
			}
			if newIface == currentIface {
				continue
			}
			// 切换网卡
			newHandle, newSrc, openErr := openHandle(newIface)
			if openErr != nil {
				slog.Warn("failed to open interface", "iface", newIface, "err", openErr)
				continue
			}
			if err := flush(); err != nil {
				newHandle.Close()
				return err
			}
			handle.Close()
			handle = newHandle
			src = newSrc
			currentIface = newIface
			pkts = src.Packets()
			slog.Info("switched to interface", "iface", currentIface)
		case pkt, ok := <-pkts:
			if !ok {
				return flush()
			}
			pktCount++
			rec, ok := packetToRecord(pkt)
			if !ok {
				continue
			}
			parsedCount++
			batch = append(batch, rec)
			if len(batch) >= batchSize {
				if err := flush(); err != nil {
					return err
				}
			}
		}
	}
}

// packetToRecord 将 gopacket 包转换为 PacketRecord
// 参数：
//   - pkt: gopacket 包
//
// 返回：
//   - model.PacketRecord: 包记录
//   - bool: 是否成功
func packetToRecord(pkt gopacket.Packet) (model.PacketRecord, bool) {
	// 解析 IPv4
	if layer := pkt.Layer(layers.LayerTypeIPv4); layer != nil {
		ip4 := layer.(*layers.IPv4)
		return flowFromL4(pkt, ip4.SrcIP, ip4.DstIP)
	}
	// 解析 IPv6
	if layer := pkt.Layer(layers.LayerTypeIPv6); layer != nil {
		ip6 := layer.(*layers.IPv6)
		return flowFromL4(pkt, ip6.SrcIP, ip6.DstIP)
	}
	return model.PacketRecord{}, false
}

// flowFromL4 从传输层提取流信息
// 参数：
//   - pkt: gopacket 包
//   - sip: 源 IP
//   - dip: 目的 IP
//
// 返回：
//   - model.PacketRecord: 包记录
//   - bool: 是否成功
func flowFromL4(pkt gopacket.Packet, sip, dip net.IP) (model.PacketRecord, bool) {
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

	// 转换为 16 字节 IP 地址
	s16 := sip.To16()
	d16 := dip.To16()
	if s16 == nil || d16 == nil {
		return model.PacketRecord{}, false
	}
	// 构建五元组
	var fk model.FlowKey
	copy(fk.SrcIP[:], s16)
	copy(fk.DstIP[:], d16)
	fk.SrcPort = sport
	fk.DstPort = dport
	fk.Proto = proto
	// 获取时间戳
	ts := pkt.Metadata().Timestamp
	return model.PacketRecord{
		TsNanos: ts.UnixNano(),
		Len:     len(pkt.Data()),
		Flow:    fk,
	}, true
}
