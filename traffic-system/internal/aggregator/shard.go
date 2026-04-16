package aggregator

import (
	"sync"

	"trafficd/internal/model"
)

// shard 分片结构体
// 用于并行聚合数据包记录
type shard struct {
	mu sync.Mutex // 互斥锁，保护共享数据

	// pktsBySrc 源 IP -> 包计数
	pktsBySrc map[[16]byte]uint64
	// dstPortsBySrc 源 IP -> 目的端口集合（用于计算目的端口基数）
	dstPortsBySrc map[[16]byte]map[uint16]struct{}
}

// newShard 创建新的分片实例
// 返回：
//   - *shard: 分片实例
func newShard() *shard {
	return &shard{
		pktsBySrc:     make(map[[16]byte]uint64),
		dstPortsBySrc: make(map[[16]byte]map[uint16]struct{}),
	}
}

// ingest 摄入单个数据包记录
// 参数：
//   - rec: 数据包记录
func (s *shard) ingest(rec model.PacketRecord) {
	src := rec.Flow.SrcIP       // 源 IP
	dstPort := rec.Flow.DstPort // 目的端口

	s.mu.Lock()
	defer s.mu.Unlock()

	// 增加源 IP 的包计数
	s.pktsBySrc[src]++

	// 记录目的端口（用于计算目的端口基数）
	ports, ok := s.dstPortsBySrc[src]
	if !ok {
		ports = make(map[uint16]struct{})
		s.dstPortsBySrc[src] = ports
	}
	ports[dstPort] = struct{}{}
}

// srcStats 源流统计信息
type srcStats struct {
	pktCount uint64              // 包计数
	dstPorts map[uint16]struct{} // 目的端口集合
}

// snapshotAndReset 快照并重置
// 将当前分片的数据生成快照，并重置分片状态
// 返回：
//   - map[[16]byte]srcStats: 源 IP -> 统计信息
func (s *shard) snapshotAndReset() map[[16]byte]srcStats {
	s.mu.Lock()
	defer s.mu.Unlock()

	p := make(map[[16]byte]srcStats, len(s.pktsBySrc))
	for src, cnt := range s.pktsBySrc {
		p[src] = srcStats{
			pktCount: cnt,
			dstPorts: s.dstPortsBySrc[src],
		}
	}
	// 重置分片状态
	s.pktsBySrc = make(map[[16]byte]uint64)
	s.dstPortsBySrc = make(map[[16]byte]map[uint16]struct{})
	return p
}
