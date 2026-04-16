package store

import (
	"context"
	"fmt"

	"trafficd/internal/aggregator"
	"trafficd/internal/model"
)

// Multi 多个 SnapshotSink 的组合
// 依次调用多个 SnapshotSink；任一失败即中止并返回错误
type Multi []aggregator.SnapshotSink

// Persist 持久化窗口快照到所有存储
// 依次调用每个 SnapshotSink 的 Persist 方法
// 任一失败即中止并返回错误
// 参数：
//   - ctx: 上下文
//   - snap: 窗口快照
//
// 返回：
//   - error: 错误信息
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
