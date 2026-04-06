package store

import (
	"context"
	"fmt"

	"trafficd/internal/aggregator"
	"trafficd/internal/model"
)

// Multi 依次调用多个 SnapshotSink；任一失败即中止并返回错误。
type Multi []aggregator.SnapshotSink

func (m Multi) Persist(ctx context.Context, snap model.WindowSnapshot) error {
	for i, s := range m {
		if s == nil {
			continue
		}
		if err := s.Persist(ctx, snap); err != nil {
			return fmt.Errorf("sink %d: %w", i, err)
		}
	}
	return nil
}
