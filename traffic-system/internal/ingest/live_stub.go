//go:build !windows || !cgo

package ingest

import (
	"context"
	"fmt"
	"time"

	"trafficd/internal/model"
)

// RunLive 在非 Windows 或未启用 cgo 的环境不可用
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
//   - error: 错误信息
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
	return fmt.Errorf("live capture requires Windows + cgo + Npcap (libpcap) installed")
}
