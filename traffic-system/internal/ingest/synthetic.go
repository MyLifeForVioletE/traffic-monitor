package ingest

import (
	"context"
	crand "crypto/rand"
	"encoding/binary"
	"math/rand/v2"
	"time"

	"trafficd/internal/model"
)

// RunSynthetic 生成合成 IPv4/TCP 流。按固定节拍批量下发，避免过高 RPS 下过小的 ticker 间隔。
func RunSynthetic(ctx context.Context, rps int, batchSize int, sink func(context.Context, []model.PacketRecord) error) error {
	if batchSize <= 0 {
		batchSize = 2048
	}
	if rps <= 0 {
		rps = 10_000
	}

	var seed [8]byte
	_, _ = crand.Read(seed[:])
	rng := rand.New(rand.NewPCG(binary.NativeEndian.Uint64(seed[:]), 0x9e3779b97f4a7c15))

	tick := 10 * time.Millisecond
	perTick := rps * int(tick) / int(time.Second)
	if perTick < 1 {
		perTick = 1
	}

	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	// 单 goroutine 出口，sink 出错即返回
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			n := perTick
			if n > batchSize {
				n = batchSize
			}
			batch := make([]model.PacketRecord, 0, n)
			for i := 0; i < n; i++ {
				batch = append(batch, randomRecord(rng))
			}
			if err := sink(ctx, batch); err != nil {
				return err
			}
		}
	}
}

func randomRecord(rng *rand.Rand) model.PacketRecord {
	a := byte(rng.IntN(50))
	if rng.IntN(100) < 30 {
		a = 10
	}
	b := byte(rng.IntN(256))
	c := byte(rng.IntN(256))
	d := byte(rng.IntN(256))
	e := byte(rng.IntN(256))

	var fk model.FlowKey
	fk.SrcIP = model.IPv4Key(192, 168, a, b)
	fk.DstIP = model.IPv4Key(10, c, d, e)
	fk.SrcPort = uint16(1024 + rng.IntN(60000))
	fk.DstPort = uint16([]uint16{80, 443, 53, 8080}[rng.IntN(4)])
	fk.Proto = 6

	ln := 64 + rng.IntN(1400)
	return model.PacketRecord{
		TsNanos: time.Now().UnixNano(),
		Len:     ln,
		Flow:    fk,
	}
}
