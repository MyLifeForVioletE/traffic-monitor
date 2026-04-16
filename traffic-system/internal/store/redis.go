package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"trafficd/internal/model"
)

// Redis Redis 存储结构体
type Redis struct {
	client *redis.Client // Redis 客户端
}

// OpenRedis 打开 Redis 连接
// 参数：
//   - addr: Redis 地址
//   - password: Redis 密码
//   - db: Redis 数据库编号
//
// 返回：
//   - *Redis: Redis 存储实例
//   - error: 错误信息
func OpenRedis(addr, password string, db int) (*Redis, error) {
	if addr == "" {
		return nil, fmt.Errorf("redis: empty addr")
	}
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		return nil, err
	}
	return &Redis{client: rdb}, nil
}

// Close 关闭 Redis 连接
// 返回：
//   - error: 错误信息
func (r *Redis) Close() error {
	return r.client.Close()
}

// snapshotJSON Redis 存储的快照 JSON 结构体
type snapshotJSON struct {
	WindowStart   int64                `json:"window_start_ns"`        // 窗口起始时间（纳秒）
	WindowEnd     int64                `json:"window_end_ns"`          // 窗口结束时间（纳秒）
	ProbeID       string               `json:"probe_id"`               // 探针 ID
	FlowsObserved int64                `json:"flows_observed"`         // 观察到的流数量
	TotalPackets  uint64               `json:"total_flow_size_window"` // 窗口内总包数
	SrcFlows      []model.SrcFlowEntry `json:"src_flows"`              // 源流条目列表
}

// Persist 持久化窗口快照到 Redis
// 参数：
//   - ctx: 上下文
//   - snap: 窗口快照
//
// 返回：
//   - error: 错误信息
func (r *Redis) Persist(ctx context.Context, snap model.WindowSnapshot) error {
	// 将窗口起始时间转换为秒级时间戳
	sec := time.Unix(0, snap.WindowStart).UTC().Truncate(time.Second).Unix()
	key := fmt.Sprintf("trafficd:last_sec:%s:%d", snap.ProbeID, sec)

	// 构建 JSON 结构体
	p := snapshotJSON{
		WindowStart:   snap.WindowStart,
		WindowEnd:     snap.WindowEnd,
		ProbeID:       snap.ProbeID,
		FlowsObserved: snap.FlowsObserved,
		TotalPackets:  snap.TotalPackets,
		SrcFlows:      snap.SrcFlows,
	}
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}

	// 使用 Pipeline 批量执行
	pipe := r.client.Pipeline()
	// 设置该秒的数据（48 小时过期）
	pipe.Set(ctx, key, b, 48*time.Hour)
	// 设置最新数据（48 小时过期）
	pipe.Set(ctx, fmt.Sprintf("trafficd:latest:%s", snap.ProbeID), b, 48*time.Hour)
	_, err = pipe.Exec(ctx)
	return err
}
