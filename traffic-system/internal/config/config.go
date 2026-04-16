package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config 运行配置，可通过 YAML 文件与环境变量覆盖。
// 支持从 YAML 文件加载配置，同时环境变量可以覆盖 YAML 中的配置
type Config struct {
	ProbeID string `yaml:"probe_id"` // 探针 ID，用于标识数据来源

	Ingest IngestConfig `yaml:"ingest"` // 数据采集配置

	Kafka KafkaConfig `yaml:"kafka"` // Kafka 配置

	Aggregator AggregatorConfig `yaml:"aggregator"` // 聚合服务配置

	MySQL MySQLConfig `yaml:"mysql"` // MySQL 数据库配置
	Redis RedisConfig `yaml:"redis"` // Redis 配置

	HTTP HTTPConfig `yaml:"http"` // HTTP 服务配置
}

// IngestConfig 数据采集配置
type IngestConfig struct {
	Mode         string `yaml:"mode"`          // 采集模式：synthetic（模拟数据）| pcap（离线文件）| live（实时抓包）
	PCAPPath     string `yaml:"pcap_path"`     // pcap 文件路径（当 mode 为 pcap 时使用）
	SyntheticRPS int    `yaml:"synthetic_rps"` // 模拟数据生成速率（每秒请求数）
	Workers      int    `yaml:"workers"`       // 工作协程数
	BatchSize    int    `yaml:"batch_size"`    // 批处理大小

	// live 仅 Windows + Npcap + cgo 可用（libpcap 依赖）。
	LiveIface         string `yaml:"live_iface"`           // 网卡名称（当 mode 为 live 时使用）
	LivePromiscuous   bool   `yaml:"live_promiscuous"`     // 是否开启混杂模式
	LiveSnapshotLen   int    `yaml:"live_snapshot_len"`    // 快照长度（捕获的包数据部分的最大长度）
	LiveReadTimeoutMs int    `yaml:"live_read_timeout_ms"` // 读取超时时间（毫秒）
	LiveBPFFilter     string `yaml:"live_bpf"`             // BPF 过滤器表达式
}

// KafkaConfig Kafka 配置
type KafkaConfig struct {
	Brokers       []string `yaml:"brokers"`        // Kafka broker 地址列表
	TopicRecords  string   `yaml:"topic_records"`  // 记录主题名称
	ConsumerGroup string   `yaml:"consumer_group"` // 消费者组名称
}

// AggregatorConfig 聚合服务配置
type AggregatorConfig struct {
	ShardCount int `yaml:"shard_count"` // 分片数量
	// WindowSeconds 聚合窗口（秒）；YAML 中用整型，避免 duration 字符串解析问题。
	WindowSeconds    int `yaml:"window_seconds"`     // 窗口大小（秒）
	TopK             int `yaml:"top_k"`              // Top K 数量
	SpreadTopSources int `yaml:"spread_top_sources"` // 扩展 Top 源 IP 数量
	FlushWorkers     int `yaml:"flush_workers"`      // 刷新工作协程数

	// MaxSrcEntries 控制每个窗口写出的 src 行数，避免窗口内 src 基数过大导致写放大。
	MaxSrcEntries int `yaml:"max_src_entries"` // 最大源 IP 条目数
}

// WindowDuration 返回窗口时长
// 返回：
//   - time.Duration: 窗口时长
func (c AggregatorConfig) WindowDuration() time.Duration {
	w := c.WindowSeconds
	if w <= 0 {
		w = 10
	}
	return time.Duration(w) * time.Second
}

// MySQLConfig MySQL 数据库配置
type MySQLConfig struct {
	DSN string `yaml:"dsn"` // 数据源名称（连接字符串）
}

// RedisConfig Redis 配置
type RedisConfig struct {
	Addr     string `yaml:"addr"`     // Redis 地址
	Password string `yaml:"password"` // Redis 密码
	DB       int    `yaml:"db"`       // Redis 数据库编号
}

// HTTPConfig HTTP 服务配置
type HTTPConfig struct {
	Addr            string `yaml:"addr"`              // HTTP 服务地址，例如 ":8080"
	AutoOpenBrowser bool   `yaml:"auto_open_browser"` // 是否自动打开浏览器
}

// Load 加载配置文件
// 参数：
//   - path: 配置文件路径，如果为空则只使用环境变量
//
// 返回：
//   - Config: 配置信息
//   - error: 错误信息
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

// Default 返回默认配置
// 返回：
//   - Config: 默认配置信息
func Default() Config {
	return Config{
		ProbeID: "probe-local",
		Ingest: IngestConfig{
			Mode:         "synthetic", // 默认使用模拟数据模式
			SyntheticRPS: 200_000,     // 默认每秒 200000 请求
			Workers:      4,           // 默认 4 个工作协程
			BatchSize:    2048,        // 默认批处理大小 2048
			// live defaults
			LiveSnapshotLen:   1600, // 默认快照长度 1600 字节
			LiveReadTimeoutMs: 1000, // 默认读取超时 1000 毫秒
			// 同时覆盖 IPv4/IPv6（纯 ip 过滤器会丢掉大量 IPv6 流量）
			LiveBPFFilter: "(ip or ip6) and (tcp or udp)", // 默认 BPF 过滤器
		},
		Kafka: KafkaConfig{
			TopicRecords:  "trafficd.records", // 默认主题名称
			ConsumerGroup: "trafficd-agg",     // 默认消费者组
		},
		Aggregator: AggregatorConfig{
			ShardCount:       256,  // 默认分片数量
			WindowSeconds:    1,    // 默认窗口 1 秒
			TopK:             20,   // 默认 Top 20
			SpreadTopSources: 50,   // 默认扩展 Top 50
			FlushWorkers:     4,    // 默认 4 个刷新工作协程
			MaxSrcEntries:    1000, // 默认最大源 IP 条目数 1000
		},
		HTTP: HTTPConfig{
			Addr:            ":8080", // 默认 HTTP 地址
			AutoOpenBrowser: true,    // 默认自动打开浏览器
		},
	}
}

// applyEnv 应用环境变量覆盖配置
// 参数：
//   - c: 配置指针
func applyEnv(c *Config) {
	// TRAFFICD_PROBE_ID: 覆盖探针 ID
	if v := os.Getenv("TRAFFICD_PROBE_ID"); v != "" {
		c.ProbeID = v
	}
	// TRAFFICD_MYSQL_DSN: 覆盖 MySQL DSN
	if v := os.Getenv("TRAFFICD_MYSQL_DSN"); v != "" {
		c.MySQL.DSN = v
	}
	// TRAFFICD_REDIS_ADDR: 覆盖 Redis 地址
	if v := os.Getenv("TRAFFICD_REDIS_ADDR"); v != "" {
		c.Redis.Addr = v
	}
	// TRAFFICD_KAFKA_BROKERS: 覆盖 Kafka broker 地址（逗号分隔）
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

	// TRAFFICD_LIVE_IFACE: 覆盖网卡名称
	if v := os.Getenv("TRAFFICD_LIVE_IFACE"); v != "" {
		c.Ingest.LiveIface = v
	}
	// TRAFFICD_LIVE_BPF: 覆盖 BPF 过滤器
	if v := os.Getenv("TRAFFICD_LIVE_BPF"); v != "" {
		c.Ingest.LiveBPFFilter = v
	}
}
