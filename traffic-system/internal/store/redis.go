package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"trafficd/internal/model"
)

type Redis struct {
	client *redis.Client
}

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

func (r *Redis) Close() error {
	return r.client.Close()
}

type snapshotJSON struct {
	WindowStart   int64                 `json:"window_start_ns"`
	WindowEnd     int64                 `json:"window_end_ns"`
	ProbeID       string                `json:"probe_id"`
	FlowsObserved int64                 `json:"flows_observed"`
	TotalPackets  uint64                `json:"total_flow_size_window"` // 窗口内总包数
	SrcFlows      []model.SrcFlowEntry `json:"src_flows"`
}

func (r *Redis) Persist(ctx context.Context, snap model.WindowSnapshot) error {
	sec := time.Unix(0, snap.WindowStart).UTC().Truncate(time.Second).Unix()
	key := fmt.Sprintf("trafficd:last_sec:%s:%d", snap.ProbeID, sec)

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

	pipe := r.client.Pipeline()
	pipe.Set(ctx, key, b, 48*time.Hour)
	pipe.Set(ctx, fmt.Sprintf("trafficd:latest:%s", snap.ProbeID), b, 48*time.Hour)
	_, err = pipe.Exec(ctx)
	return err
}
