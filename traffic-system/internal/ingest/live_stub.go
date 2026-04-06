//go:build !windows || !cgo

package ingest

import (
	"context"
	"fmt"
	"time"

	"trafficd/internal/model"
)

// RunLive 在非 Windows 或未启用 cgo 的环境不可用。
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

