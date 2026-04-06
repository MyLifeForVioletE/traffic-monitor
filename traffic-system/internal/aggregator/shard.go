package aggregator

import (
	"encoding/binary"
	"sync"

	"github.com/axiomhq/hyperloglog"

	"trafficd/internal/model"
)

type shard struct {
	mu sync.Mutex

	// 流大小：按 src 计包数（每包 +1）
	pktsBySrc map[[16]byte]uint64

	// 流基数：按 src 对 dst port 做 distinct 估计（HLL，元素为 2 字节 port）
	hllBySrc map[[16]byte]*hyperloglog.Sketch
}

func newShard() *shard {
	return &shard{
		pktsBySrc: make(map[[16]byte]uint64),
		hllBySrc:  make(map[[16]byte]*hyperloglog.Sketch),
	}
}

func (s *shard) ingest(rec model.PacketRecord) {
	src := rec.Flow.SrcIP

	s.mu.Lock()
	defer s.mu.Unlock()

	s.pktsBySrc[src]++

	sk := s.hllBySrc[src]
	if sk == nil {
		sk = hyperloglog.New14()
		s.hllBySrc[src] = sk
	}
	var portLE [2]byte
	binary.LittleEndian.PutUint16(portLE[:], rec.Flow.DstPort)
	sk.Insert(portLE[:])
}

func (s *shard) snapshotAndReset() (map[[16]byte]uint64, map[[16]byte]*hyperloglog.Sketch) {
	s.mu.Lock()
	defer s.mu.Unlock()

	p := s.pktsBySrc
	h := s.hllBySrc

	s.pktsBySrc = make(map[[16]byte]uint64)
	s.hllBySrc = make(map[[16]byte]*hyperloglog.Sketch)

	return p, h
}
