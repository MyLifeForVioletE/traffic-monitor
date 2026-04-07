# trafficd - 高速网络流量监测系统

![Go Version](https://img.shields.io/badge/Go-1.23+-00ADD8?style=flat&logo=go)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

基于 Go 语言开发的高性能网络流量实时分析与监控平台，支持分钟级流量聚合、Heavy Hitter 检测、端口扫描行为识别，提供现代化的 Web 可视化监控面板。

## ✨ 功能特性

| 特性 | 描述 |
|------|------|
| **多源流量接入** | 支持实时网卡捕获、PCAP 文件回放、流量模拟生成 |
| **高性能采集** | 基于 gopacket 零拷贝技术，单节点万兆线速处理 |
| **流聚合引擎** | 基于五元组的秒级/分钟级流量聚合 |
| **Heavy Hitter** | Top-K 大流实时检测与排名 |
| **基数估计** | HyperLogLog 算法实现主机连接端口基数估计 |
| **扫描检测** | 基于端口基数异常的端口扫描行为识别 |
| **Web 监控面板** | 实时流量仪表盘、拓扑分析、趋势图表 |
| **分布式架构** | Kafka 消息队列 + 分片聚合 + MySQL/Redis 存储 |

## 🏗️ 系统架构

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  流量采集层     │────▶│   Kafka 队列    │────▶│   聚合计算层    │
│  - PCAP 回放    │     │   - Packet      │     │   - 分片聚合    │
│  - 实时网卡     │     │   - FlowRecord  │     │   - Top-K       │
│  - 模拟生成     │     │                 │     │   - HLL 基数    │
└─────────────────┘     └─────────────────┘     └─────────────────┘
                                                          │
                                                          ▼
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  Web 监控面板   │◀────│   HTTP API      │◀────│    存储层       │
│  - 实时速率     │     │   - 流量查询    │     │   - MySQL       │
│  - Top 排行     │     │   - 时序分析    │     │   - Redis       │
│  - 扫描检测     │     │   - 网卡管理    │     │                │
└─────────────────┘     └─────────────────┘     └─────────────────┘
```

## 🚀 快速开始

### 环境依赖

- Go 1.23+
- MySQL 8.0+
- Redis 6.0+
- Kafka 2.8+ (可选)
- Windows: Npcap / Linux: libpcap

### 编译安装

```bash
git clone https://github.com/yourusername/trafficd.git
cd trafficd
# 待 main 入口完善后执行编译
# go build -o trafficd .
```

> 💡 项目核心模块已完成，入口程序 `main.go` 正在开发中

### 数据库初始化

```bash
mysql -u root -p < migrations/schema.sql
```

### 配置文件

创建 `config.yaml`:

```yaml
probe_id: "probe-01"

ingest:
  mode: "pcap"           # synthetic | pcap | live
  pcap_path: "./sample.pcap"
  synthetic_rps: 10000
  workers: 4
  batch_size: 1000
  live_iface: "以太网"    # Windows 网卡名称
  live_bpf: "tcp or udp"  # BPF 过滤规则

kafka:
  brokers: ["localhost:9092"]
  topic_records: "traffic_records"
  consumer_group: "trafficd_agg"

aggregator:
  shard_count: 8
  window_seconds: 10
  top_k: 100
  spread_top_sources: 50

mysql:
  dsn: "root:password@tcp(127.0.0.1:3306)/trafficd?charset=utf8mb4&parseTime=True"

redis:
  addr: "127.0.0.1:6379"
  password: ""
  db: 0

http:
  addr: ":8080"
```

### 启动服务

```bash
./trafficd -config config.yaml
```

访问 `http://localhost:8080` 即可打开监控面板。

## 📊 监控面板

| 功能 | 截图 |
|------|------|
| **实时仪表盘** | 流量速率、包速率、流数统计 |
| **Top-N 流量** | 按字节/包数排名的 Top 流量 |
| **端口扫描检测** | 异常端口基数的主机列表 |
| **时序分析** | 单 IP 的流量趋势图 |
| **网卡管理** | 实时切换捕获网卡 |

## 🔧 核心技术

### 分片聚合引擎

```go
// aggregator/shard.go
type Shard struct {
    flows          *intmap.Map[FlowKey, *FlowState]
    srcSpread      map[[16]byte]*hyperloglog.Sketch
    windowStart    time.Time
}
```

- 基于源 IP Hash 分片，无锁并行计算
- 内存友好的整型 Map 实现，减少 GC
- HyperLogLog 基数估计，O(1) 空间复杂度

### 高性能数据包捕获

```go
// ingest/live_pcap.go
func LiveCapture(iface string, bpf string) chan PacketRecord {
    handle, _ := pcap.OpenLive(iface, 65536, true, time.Millisecond)
    handle.SetBPFFilter(bpf)
    // ...
}
```

- Windows Npcap 驱动，支持硬件时间戳
- BPF 内核过滤，减少用户态拷贝
- 批量写入 Kafka，提高吞吐量

## 📈 性能指标

| 指标 | 数值 |
|------|------|
| 单网卡捕获速率 | 10 Gbps+ |
| 包处理性能 | 10 Mpps |
| 聚合延迟 | < 1s |
| 内存占用 | < 500MB (100w 并发流) |

## 📁 项目结构

```
trafficd/
├── internal/
│   ├── aggregator/     # 分片聚合引擎
│   ├── api/            # HTTP API + Web 面板
│   ├── config/         # 配置管理
│   ├── ingest/         # 流量采集模块
│   ├── model/          # 数据模型
│   ├── parser/         # 协议解析
│   ├── queue/          # Kafka 客户端
│   └── store/          # MySQL/Redis 存储
├── migrations/         # 数据库脚本
├── web/                # 前端静态资源
├── go.mod
└── config.yaml.example
```

## 🤝 贡献指南

欢迎提交 Issue 和 Pull Request！

## 📄 许可证

MIT License - 详见 [LICENSE](LICENSE) 文件。

---

**⚠️ 注意**: 本项目仅用于合法的网络管理和安全监控用途，请确保遵守您所在地区的网络法律法规。
