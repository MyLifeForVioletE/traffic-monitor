package aggregator

import (
	"context"
	"hash/fnv"
	"log/slog"
	"net"
	"sort"
	"time"

	"github.com/axiomhq/hyperloglog"

	"trafficd/internal/config"
	"trafficd/internal/model"
)

// Service 分片聚合 + 窗口切分。
type Service struct {
	cfg     config.AggregatorConfig
	probeID string
	shards  []*shard
	sink    SnapshotSink
	log     *slog.Logger
}

// SnapshotSink 窗口落盘回调（MySQL/Redis/日志等）。
type SnapshotSink interface {
	Persist(ctx context.Context, snap model.WindowSnapshot) error
}

func NewService(cfg config.AggregatorConfig, probeID string, sink SnapshotSink, log *slog.Logger) (*Service, error) {
	if cfg.ShardCount <= 0 {
		cfg.ShardCount = 256
	}
	if cfg.MaxSrcEntries < 0 {
		cfg.MaxSrcEntries = 0
	}
	if log == nil {
		log = slog.Default()
	}
	shards := make([]*shard, cfg.ShardCount)
	for i := range shards {
		shards[i] = newShard()
	}
	return &Service{
		cfg:     cfg,
		probeID: probeID,
		shards:  shards,
		sink:    sink,
		log:     log,
	}, nil
}

func (s *Service) IngestBatch(records []model.PacketRecord) {
	for i := range records {
		idx := shardIndex(records[i].Flow.SrcIP, len(s.shards))
		s.shards[idx].ingest(records[i])
	}
}

// Run 从 channel 读取批次，按窗口边界生成快照。
func (s *Service) Run(ctx context.Context, in <-chan []model.PacketRecord) error {
	wd := s.cfg.WindowDuration()
	t := time.NewTicker(wd)
	defer t.Stop()

	windowStart := time.Now().Truncate(wd)

	for {
		select {
		case <-ctx.Done():
			s.flushWindow(ctx, windowStart, time.Now())
			return ctx.Err()
		case batch, ok := <-in:
			if !ok {
				s.flushWindow(ctx, windowStart, time.Now())
				return nil
			}
			s.IngestBatch(batch)
		case now := <-t.C:
			end := now.Truncate(wd)
			if end.After(windowStart) {
				s.flushWindow(ctx, windowStart, end)
				windowStart = end
			}
		}
	}
}

func (s *Service) flushWindow(ctx context.Context, start time.Time, end time.Time) {
	snap := s.buildSnapshot(start.UnixNano(), end.UnixNano())
	if s.sink == nil {
		preview := snap.SrcFlows
		if len(preview) > 5 {
			preview = preview[:5]
		}
		s.log.Info("window",
			"start", start.Format(time.RFC3339),
			"srcs_unique", snap.FlowsObserved,
			"srcs_emitted", len(snap.SrcFlows),
			"total_flow_size", snap.TotalPackets,
			"top_src_flows", preview,
		)
		return
	}
	if err := s.sink.Persist(ctx, snap); err != nil {
		s.log.Error("persist snapshot", "err", err)
	}
}

func (s *Service) buildSnapshot(startNs, endNs int64) model.WindowSnapshot {
	mergedPkts := make(map[[16]byte]uint64, 1024)
	mergedHLL := make(map[[16]byte]*hyperloglog.Sketch, 1024)

	var totalPkts uint64

	for _, sh := range s.shards {
		p, h := sh.snapshotAndReset()
		for src, v := range p {
			mergedPkts[src] += v
			totalPkts += v
		}
		for src, sk := range h {
			dst := mergedHLL[src]
			if dst == nil {
				mergedHLL[src] = sk
				continue
			}
			_ = dst.Merge(sk)
		}
	}

	snap := model.WindowSnapshot{
		WindowStart:   startNs,
		WindowEnd:     endNs,
		ProbeID:       s.probeID,
		FlowsObserved: int64(len(mergedHLL)),
		TotalPackets:  totalPkts,
		TotalBytes:    0,
	}

	entries := make([]model.SrcFlowEntry, 0, len(mergedHLL))
	for src, sk := range mergedHLL {
		entries = append(entries, model.SrcFlowEntry{
			SrcIP:                 net.IP(src[:]).String(),
			FlowSize:              mergedPkts[src],
			DstPortCardinalityEst: sk.Estimate(),
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].FlowSize > entries[j].FlowSize
	})
	if s.cfg.MaxSrcEntries > 0 && len(entries) > s.cfg.MaxSrcEntries {
		entries = entries[:s.cfg.MaxSrcEntries]
	}

	snap.SrcFlows = entries
	return snap
}

func shardIndex(src [16]byte, n int) int {
	h := fnv.New32a()
	_, _ = h.Write(src[:])
	return int(h.Sum32() % uint32(n))
}
