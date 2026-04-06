-- 若你之前已按旧版建过 agg_src_second，请执行本脚本升级表结构（会删除旧表数据）。
-- 新口径：flow_size=包数；dst_port_cardinality_est=不同 dst port 的 HLL 估计。

DROP TABLE IF EXISTS agg_src_second;

CREATE TABLE IF NOT EXISTS agg_src_second (
    sec_ts                      DATETIME(0) NOT NULL,
    probe_id                    VARCHAR(64)  NOT NULL,
    src_ip                      VARCHAR(45)  NOT NULL,
    flow_size                   BIGINT UNSIGNED NOT NULL DEFAULT 0,
    dst_port_cardinality_est    DOUBLE NOT NULL,
    PRIMARY KEY (sec_ts, probe_id, src_ip),
    KEY idx_probe_time (probe_id, sec_ts)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
