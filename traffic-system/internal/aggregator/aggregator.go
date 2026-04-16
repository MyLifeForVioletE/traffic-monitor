package aggregator

import (
	"context"
	"hash/fnv"
	"log/slog"
	"net"
	"sort"
	"time"

	"trafficd/internal/config"
	"trafficd/internal/model"
)

// Service 分片聚合服务结构体
// 实现分片聚合 + 窗口切分
type Service struct {
	cfg     config.AggregatorConfig // 聚合配置
	probeID string                  // 探针 ID
	shards  []*shard                // 分片列表
	sink    SnapshotSink            // 窗口落盘回调（MySQL/Redis/日志等）
	log     *slog.Logger            // 日志记录器
}

// SnapshotSink 窗口落盘回调接口（MySQL/Redis/日志等）
// 用于将聚合后的窗口快照持久化到存储
type SnapshotSink interface {
	Persist(ctx context.Context, snap model.WindowSnapshot) error
}

// NewService 创建新的聚合服务实例
// 参数：
//   - cfg: 聚合配置
//   - probeID: 探针 ID
//   - sink: 窗口落盘回调
//   - log: 日志记录器
//
// 返回：
//   - *Service: 聚合服务实例
//   - error: 错误信息
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
	// 初始化分片
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

// IngestBatch 摄入一批数据包记录
// 参数：
//   - records: 数据包记录列表
func (s *Service) IngestBatch(records []model.PacketRecord) {
	s.log.Info("ingest batch", "count", len(records))
	for i := range records {
		// 根据源 IP 哈希计算分片索引
		idx := shardIndex(records[i].Flow.SrcIP, len(s.shards))
		s.shards[idx].ingest(records[i])
	}
}

// Run 从 channel 读取批次，按窗口边界生成快照
// 参数：
//   - ctx: 上下文
//   - in: 数据包记录 channel
//
// 返回：
//   - error: 错误信息
func (s *Service) Run(ctx context.Context, in <-chan []model.PacketRecord) error {
	wd := s.cfg.WindowDuration() // 窗口时长
	t := time.NewTicker(wd)
	defer t.Stop()

	windowStart := time.Now().Truncate(wd) // 当前窗口起始时间

	for {
		select {
		case <-ctx.Done():
			// 上下文取消，刷新当前窗口并返回
			s.flushWindow(ctx, windowStart, time.Now())
			return ctx.Err()
		case batch, ok := <-in:
			if !ok {
				// channel 关闭，刷新当前窗口并返回
				s.flushWindow(ctx, windowStart, time.Now())
				return nil
			}
			s.IngestBatch(batch)
		case now := <-t.C:
			// 窗口到期，刷新窗口
			end := now.Truncate(wd)
			if end.After(windowStart) {
				s.flushWindow(ctx, windowStart, end)
				windowStart = end
			}
		}
	}
}

// flushWindow 刷新窗口数据
// 参数：
//   - ctx: 上下文
//   - start: 窗口起始时间
//   - end: 窗口结束时间
func (s *Service) flushWindow(ctx context.Context, start time.Time, end time.Time) {
	snap := s.buildSnapshot(start.UnixNano(), end.UnixNano())
	if s.sink == nil {
		// 没有配置 sink，只打印日志
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
	// 持久化到存储
	if err := s.sink.Persist(ctx, snap); err != nil {
		s.log.Error("persist snapshot", "err", err)
	}
}

// buildSnapshot 构建窗口快照
// 参数：
//   - startNs: 窗口起始时间（纳秒）
//   - endNs: 窗口结束时间（纳秒）
//
// 返回：
//   - model.WindowSnapshot: 窗口快照
func (s *Service) buildSnapshot(startNs, endNs int64) model.WindowSnapshot {
	// 合并所有分片的数据
	mergedPkts := make(map[[16]byte]uint64, 1024)
	mergedPorts := make(map[[16]byte]map[uint16]struct{}, 1024)
	var totalPkts uint64
	for _, sh := range s.shards {
		p := sh.snapshotAndReset()
		for src, st := range p {
			mergedPkts[src] += st.pktCount
			totalPkts += st.pktCount
			if st.dstPorts != nil {
				if existing := mergedPorts[src]; existing != nil {
					for port := range st.dstPorts {
						existing[port] = struct{}{}
					}
				} else {
					mergedPorts[src] = st.dstPorts
				}
			}
		}
	}

	// 构建快照
	snap := model.WindowSnapshot{
		WindowStart:   startNs,
		WindowEnd:     endNs,
		ProbeID:       s.probeID,
		FlowsObserved: int64(len(mergedPkts)),
		TotalPackets:  totalPkts,
		TotalBytes:    0,
	}

	// 构建源流条目列表
	entries := make([]model.SrcFlowEntry, 0, len(mergedPkts))
	for src := range mergedPkts {
		var cardEst uint64
		if ports := mergedPorts[src]; ports != nil {
			cardEst = uint64(len(ports))
		}
		entries = append(entries, model.SrcFlowEntry{
			SrcIP:             net.IP(src[:]).String(),
			FlowSize:          mergedPkts[src],
			DstCardinalityEst: cardEst,
		})
	}

	// 按流大小降序排序
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].FlowSize > entries[j].FlowSize
	})
	// 限制最大条目数
	if s.cfg.MaxSrcEntries > 0 && len(entries) > s.cfg.MaxSrcEntries {
		entries = entries[:s.cfg.MaxSrcEntries]
	}

	snap.SrcFlows = entries
	return snap
}

// shardIndex 根据源 IP 计算分片索引
// 参数：
//   - src: 源 IP（16 字节）
//   - n: 分片数量
//
// 返回：
//   - int: 分片索引
func shardIndex(src [16]byte, n int) int {
	h := fnv.New32a()
	_, _ = h.Write(src[:])
	return int(h.Sum32() % uint32(n))
}
