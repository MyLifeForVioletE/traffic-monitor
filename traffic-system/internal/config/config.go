package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config 运行配置，可通过 YAML 文件与环境变量覆盖。
type Config struct {
	ProbeID string `yaml:"probe_id"`

	Ingest IngestConfig `yaml:"ingest"`

	Kafka KafkaConfig `yaml:"kafka"`

	Aggregator AggregatorConfig `yaml:"aggregator"`

	MySQL MySQLConfig `yaml:"mysql"`
	Redis RedisConfig `yaml:"redis"`

	HTTP HTTPConfig `yaml:"http"`
}

type IngestConfig struct {
	Mode         string `yaml:"mode"` // synthetic | pcap | live
	PCAPPath     string `yaml:"pcap_path"`
	SyntheticRPS int    `yaml:"synthetic_rps"`
	Workers      int    `yaml:"workers"`
	BatchSize    int    `yaml:"batch_size"`

	// live 仅 Windows + Npcap + cgo 可用（libpcap 依赖）。
	LiveIface       string `yaml:"live_iface"`
	LivePromiscuous bool   `yaml:"live_promiscuous"`
	LiveSnapshotLen int    `yaml:"live_snapshot_len"`
	LiveReadTimeoutMs int  `yaml:"live_read_timeout_ms"`
	LiveBPFFilter   string `yaml:"live_bpf"`
}

type KafkaConfig struct {
	Brokers       []string `yaml:"brokers"`
	TopicRecords  string   `yaml:"topic_records"`
	ConsumerGroup string   `yaml:"consumer_group"`
}

type AggregatorConfig struct {
	ShardCount       int `yaml:"shard_count"`
	// WindowSeconds 聚合窗口（秒）；YAML 中用整型，避免 duration 字符串解析问题。
	WindowSeconds    int `yaml:"window_seconds"`
	TopK             int `yaml:"top_k"`
	SpreadTopSources int `yaml:"spread_top_sources"`
	FlushWorkers     int `yaml:"flush_workers"`

	// MaxSrcEntries 控制每个窗口写出的 src 行数，避免窗口内 src 基数过大导致写放大。
	MaxSrcEntries int `yaml:"max_src_entries"`
}

func (c AggregatorConfig) WindowDuration() time.Duration {
	w := c.WindowSeconds
	if w <= 0 {
		w = 10
	}
	return time.Duration(w) * time.Second
}

type MySQLConfig struct {
	DSN string `yaml:"dsn"`
}

type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

type HTTPConfig struct {
	Addr string `yaml:"addr"` // e.g. ":8080"
}

func Load(path string) (Config, error) {
	c := Default()
	if path == "" {
		applyEnv(&c)
		return c, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return c, fmt.Errorf("read config: %w", err)
	}
	if err := yaml.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("yaml: %w", err)
	}
	applyEnv(&c)
	return c, nil
}

func Default() Config {
	return Config{
		ProbeID: "probe-local",
		Ingest: IngestConfig{
			Mode:         "synthetic",
			SyntheticRPS: 200_000,
			Workers:      4,
			BatchSize:    2048,
			// live defaults
			LiveSnapshotLen: 1600,
			LiveReadTimeoutMs: 1000,
			// 同时覆盖 IPv4/IPv6（纯 ip 过滤器会丢掉大量 IPv6 流量）
			LiveBPFFilter: "(ip or ip6) and (tcp or udp)",
		},
		Kafka: KafkaConfig{
			TopicRecords:  "trafficd.records",
			ConsumerGroup: "trafficd-agg",
		},
		Aggregator: AggregatorConfig{
			ShardCount:       256,
			WindowSeconds:    1,
			TopK:             20,
			SpreadTopSources: 50,
			FlushWorkers:     4,
			MaxSrcEntries:    1000,
		},
		HTTP: HTTPConfig{
			Addr: ":8080",
		},
	}
}

func applyEnv(c *Config) {
	if v := os.Getenv("TRAFFICD_PROBE_ID"); v != "" {
		c.ProbeID = v
	}
	if v := os.Getenv("TRAFFICD_MYSQL_DSN"); v != "" {
		c.MySQL.DSN = v
	}
	if v := os.Getenv("TRAFFICD_REDIS_ADDR"); v != "" {
		c.Redis.Addr = v
	}
	if v := os.Getenv("TRAFFICD_KAFKA_BROKERS"); v != "" {
		var brokers []string
		for _, p := range strings.Split(v, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				brokers = append(brokers, p)
			}
		}
		c.Kafka.Brokers = brokers
	}

	if v := os.Getenv("TRAFFICD_LIVE_IFACE"); v != "" {
		c.Ingest.LiveIface = v
	}
	if v := os.Getenv("TRAFFICD_LIVE_BPF"); v != "" {
		c.Ingest.LiveBPFFilter = v
	}
}
