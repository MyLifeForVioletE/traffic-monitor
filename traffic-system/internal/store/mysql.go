package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"trafficd/internal/model"
)

// MySQL MySQL 存储结构体
type MySQL struct {
	db *sql.DB // 数据库连接
}

// DB 获取数据库连接
// 返回：
//   - *sql.DB: 数据库连接
func (m *MySQL) DB() *sql.DB {
	return m.db
}

// OpenMySQL 打开 MySQL 数据库连接
// 参数：
//   - dsn: 数据源名称（连接字符串）
//
// 返回：
//   - *MySQL: MySQL 存储实例
//   - error: 错误信息
func OpenMySQL(dsn string) (*MySQL, error) {
	if dsn == "" {
		return nil, fmt.Errorf("mysql: empty dsn")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	// 设置连接池参数
	db.SetMaxOpenConns(16)                 // 最大打开连接数
	db.SetMaxIdleConns(8)                  // 最大空闲连接数
	db.SetConnMaxLifetime(5 * time.Minute) // 连接最大生命周期
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// 测试连接
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &MySQL{db: db}, nil
}

// Close 关闭数据库连接
// 返回：
//   - error: 错误信息
func (m *MySQL) Close() error {
	return m.db.Close()
}

// Persist 持久化窗口快照到 MySQL
// 参数：
//   - ctx: 上下文
//   - snap: 窗口快照
//
// 返回：
//   - error: 错误信息
func (m *MySQL) Persist(ctx context.Context, snap model.WindowSnapshot) error {
	// 将窗口起始时间转换为秒级时间戳（UTC）
	sec := time.Unix(0, snap.WindowStart).UTC().Truncate(time.Second)
	if snap.ProbeID == "" {
		return fmt.Errorf("mysql: empty probe_id")
	}

	// 开始事务
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// 确保探针存在
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO dim_probe(probe_id) VALUES(?)
		ON DUPLICATE KEY UPDATE probe_id = probe_id
	`, snap.ProbeID); err != nil {
		return fmt.Errorf("dim_probe: %w", err)
	}

	// 删除该秒的旧数据
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM agg_src_second WHERE sec_ts = ? AND probe_id = ?`,
		sec, snap.ProbeID); err != nil {
		return fmt.Errorf("agg_src_second delete: %w", err)
	}

	// 插入新的聚合数据
	for _, e := range snap.SrcFlows {
		if _, err := tx.ExecContext(ctx, `
		   INSERT INTO agg_src_second(sec_ts, probe_id, src_ip, flow_size, dst_port_cardinality_est)
		   VALUES(?,?,?,?,?)
		   ON DUPLICATE KEY UPDATE
			   flow_size = VALUES(flow_size),
			   dst_port_cardinality_est = VALUES(dst_port_cardinality_est)
	   `, sec, snap.ProbeID, e.SrcIP, e.FlowSize, e.DstCardinalityEst); err != nil {
			return fmt.Errorf("agg_src_second insert: %w", err)
		}
	}

	// 提交事务
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}
