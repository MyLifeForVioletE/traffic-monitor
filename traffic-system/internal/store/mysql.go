package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"trafficd/internal/model"
)

type MySQL struct {
	db *sql.DB
}

func (m *MySQL) DB() *sql.DB {
	return m.db
}

func OpenMySQL(dsn string) (*MySQL, error) {
	if dsn == "" {
		return nil, fmt.Errorf("mysql: empty dsn")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(16)
	db.SetMaxIdleConns(8)
	db.SetConnMaxLifetime(5 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &MySQL{db: db}, nil
}

func (m *MySQL) Close() error {
	return m.db.Close()
}

func (m *MySQL) Persist(ctx context.Context, snap model.WindowSnapshot) error {
	sec := time.Unix(0, snap.WindowStart).UTC().Truncate(time.Second)
	if snap.ProbeID == "" {
		return fmt.Errorf("mysql: empty probe_id")
	}

	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO dim_probe(probe_id) VALUES(?)
		ON DUPLICATE KEY UPDATE probe_id = probe_id
	`, snap.ProbeID); err != nil {
		return fmt.Errorf("dim_probe: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM agg_src_second WHERE sec_ts = ? AND probe_id = ?`,
		sec, snap.ProbeID); err != nil {
		return fmt.Errorf("agg_src_second delete: %w", err)
	}

	for _, e := range snap.SrcFlows {
		est := float64(e.DstPortCardinalityEst)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO agg_src_second(sec_ts, probe_id, src_ip, flow_size, dst_port_cardinality_est)
			VALUES(?,?,?,?,?)
			ON DUPLICATE KEY UPDATE
				flow_size = VALUES(flow_size),
				dst_port_cardinality_est = VALUES(dst_port_cardinality_est)
		`, sec, snap.ProbeID, e.SrcIP, e.FlowSize, est); err != nil {
			return fmt.Errorf("agg_src_second insert: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}
