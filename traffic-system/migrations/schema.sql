-- trafficd: 分钟级聚合、Heavy Hitter、Flow spread 估计

CREATE TABLE IF NOT EXISTS dim_probe (
    probe_id      VARCHAR(64) NOT NULL PRIMARY KEY,
    site          VARCHAR(128) DEFAULT '',
    parser_ver    VARCHAR(32)  DEFAULT '1',
    sampling_rate DOUBLE       DEFAULT 1.0,
    updated_at    TIMESTAMP    DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS agg_flow_minute (
    minute_ts     DATETIME(0) NOT NULL,
    probe_id      VARCHAR(64)  NOT NULL,
    flows_observed BIGINT UNSIGNED NOT NULL DEFAULT 0,
    total_bytes    BIGINT UNSIGNED NOT NULL DEFAULT 0,
    total_pkts     BIGINT UNSIGNED NOT NULL DEFAULT 0,
    PRIMARY KEY (minute_ts, probe_id),
    KEY idx_probe_time (probe_id, minute_ts)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS heavy_hitter_minute (
    minute_ts      DATETIME(0) NOT NULL,
    probe_id       VARCHAR(64)  NOT NULL,
    rank_pos       INT UNSIGNED NOT NULL,
    flow_key_hex   VARCHAR(80) NOT NULL,
    src_ip         VARCHAR(45) NOT NULL,
    dst_ip         VARCHAR(45) NOT NULL,
    src_port       INT UNSIGNED NOT NULL,
    dst_port       INT UNSIGNED NOT NULL,
    proto          TINYINT UNSIGNED NOT NULL,
    bytes_sum      BIGINT UNSIGNED NOT NULL,
    pkts_sum       BIGINT UNSIGNED NOT NULL,
    PRIMARY KEY (minute_ts, probe_id, rank_pos),
    KEY idx_time (minute_ts)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS spread_minute (
    minute_ts           DATETIME(0) NOT NULL,
    probe_id            VARCHAR(64)  NOT NULL,
    src_ip              VARCHAR(45)  NOT NULL,
    dst_cardinality_est DOUBLE       NOT NULL,
    PRIMARY KEY (minute_ts, probe_id, src_ip),
    KEY idx_time (minute_ts)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- agg_src_second：流标签=src；flow_size=该秒该 src 包数；dst_port_cardinality_est=不同 dst port 的 HLL 估计
CREATE TABLE IF NOT EXISTS agg_src_second (
    sec_ts                      DATETIME(0) NOT NULL,
    probe_id                    VARCHAR(64)  NOT NULL,
    src_ip                      VARCHAR(45)  NOT NULL,
    flow_size                   BIGINT UNSIGNED NOT NULL DEFAULT 0,
    dst_port_cardinality_est    DOUBLE NOT NULL,
    PRIMARY KEY (sec_ts, probe_id, src_ip),
    KEY idx_probe_time (probe_id, sec_ts)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
