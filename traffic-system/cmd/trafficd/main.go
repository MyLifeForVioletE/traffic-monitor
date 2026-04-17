package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"trafficd/internal/aggregator"
	trafficdapi "trafficd/internal/api"
	"trafficd/internal/config"
	"trafficd/internal/ingest"
	"trafficd/internal/model"
	"trafficd/internal/parser"
	"trafficd/internal/queue"
	"trafficd/internal/store"
)

func main() {
	// 命令行参数：-config 指定配置文件路径
	// 如果未指定，则按顺序查找 configs/example.yaml, example.yaml, ./configs/example.yaml
	cfgPath := flag.String("config", "", "YAML 配置文件路径（可空，使用默认+环境变量）")
	// 命令行参数：-mode 指定运行模式
	// all: 运行全部功能（ingest + aggregate + api）
	// ingest: 仅运行数据采集，发送到 Kafka
	// aggregate: 仅运行聚合服务，从 Kafka 消费
	// api: 仅运行 HTTP API 服务
	mode := flag.String("mode", "all", "运行模式：all | ingest | aggregate | api")
	// 命令行参数：-list-interfaces 列出可用的网卡设备
	listInterfaces := flag.Bool("list-interfaces", false, "列出可用于 live capture 的网卡设备名（Npcap）")
	flag.Parse()

	// 如果未指定配置文件，则自动查找默认配置文件
	if *cfgPath == "" {
		for _, p := range []string{"configs/example.yaml", "example.yaml", "./configs/example.yaml"} {
			if exists, _ := exists(p); exists {
				*cfgPath = p
				break
			}
		}
	}

	// 列出可用的网卡设备（用于 live capture 模式）
	if *listInterfaces {
		devs, err := ingest.ListDevices()
		if err != nil {
			fmt.Fprintln(os.Stderr, "list interfaces error:", err)
			os.Exit(1)
		}
		for _, d := range devs {
			if d.Description != "" {
				fmt.Printf("%s - %s\n", d.Name, d.Description)
			} else {
				fmt.Println(d.Name)
			}
		}
		return
	}

	// 初始化日志输出，使用 JSON 格式，级别为 Info
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// 加载配置文件，支持 YAML 文件和环境变量
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Error("load config", "err", err)
		os.Exit(1)
	}

	// 创建信号上下文，用于优雅退出（Ctrl+C 或 kill 命令）
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 判断是否配置了 Kafka（用于决定使用本地模式还是 Kafka 模式）
	useKafka := len(cfg.Kafka.Brokers) > 0

	// 根据运行模式分发到不同的处理函数
	switch *mode {
	case "ingest":
		// ingest 模式：仅运行数据采集，需要 Kafka 配置
		if !useKafka || cfg.Kafka.TopicRecords == "" {
			log.Error("ingest 模式需要配置 kafka.brokers 与 kafka.topic_records")
			os.Exit(1)
		}
		runIngest(ctx, cfg, log)
	case "aggregate":
		// aggregate 模式：仅运行聚合服务，需要 Kafka 配置
		if !useKafka || cfg.Kafka.TopicRecords == "" {
			log.Error("aggregate 模式需要 kafka 配置")
			os.Exit(1)
		}
		runAggregateKafka(ctx, cfg, log)
	case "all":
		// all 模式：运行全部功能
		if useKafka {
			// 如果配置了 Kafka，使用 Kafka 模式（分布式）
			runAllKafka(ctx, cfg, log)
		} else {
			// 如果未配置 Kafka，使用本地模式（单机）
			runAllLocal(ctx, cfg, log)
		}
	case "api":
		// api 模式：仅运行 HTTP API 服务
		if cfg.MySQL.DSN == "" {
			log.Error("api 模式需要 mysql.dsn 配置")
			os.Exit(1)
		}
		runAPI(ctx, cfg, log, nil)
	default:
		// 未知模式，报错退出
		log.Error("unknown mode", "mode", *mode)
		os.Exit(1)
	}
}

// runAPI 启动 HTTP API 服务器，提供查询接口
// 参数：
//   - ctx: 上下文，用于优雅退出
//   - cfg: 配置信息
//   - log: 日志记录器
//   - ifaceCh: 网卡选择通道（可选，用于实时切换网卡）
func runAPI(ctx context.Context, cfg config.Config, log *slog.Logger, ifaceCh chan<- string) {
	// 打开 MySQL 数据库连接
	mysql, err := store.OpenMySQL(cfg.MySQL.DSN)
	if err != nil {
		log.Error("mysql open", "err", err)
		os.Exit(1)
	}
	defer mysql.Close()

	// 创建 API 服务器实例
	srv, err := trafficdapi.NewServer(cfg, mysql.DB(), log, ifaceCh)
	if err != nil {
		log.Error("api new server", "err", err)
		os.Exit(1)
	}
	// 启动 HTTP 服务器
	log.Info("api listening", "addr", cfg.HTTP.Addr)
	if err := srv.Run(ctx); err != nil && err != context.Canceled {
		log.Error("api run", "err", err)
		os.Exit(1)
	}
}

// buildSink 构建数据存储 sink，用于存储聚合后的数据
// 支持 MySQL 和 Redis 两种存储方式
// 返回：
//   - aggregator.SnapshotSink: 数据存储接口
func buildSink(cfg config.Config, log *slog.Logger) aggregator.SnapshotSink {
	var parts store.Multi

	// 如果配置了 MySQL DSN，则连接 MySQL
	if cfg.MySQL.DSN != "" {
		log.Info("buildSink: connecting to mysql", "dsn", cfg.MySQL.DSN)
		db, err := store.OpenMySQL(cfg.MySQL.DSN)
		if err != nil {
			log.Warn("mysql disabled", "err", err)
		} else {
			parts = append(parts, db)
			log.Info("mysql connected")
		}
	} else {
		log.Info("buildSink: mysql dsn is empty")
	}

	// 如果配置了 Redis 地址，则连接 Redis
	if cfg.Redis.Addr != "" {
		rdb, err := store.OpenRedis(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
		if err != nil {
			log.Warn("redis disabled", "err", err)
		} else {
			parts = append(parts, rdb)
			log.Info("redis connected")
		}
	}

	// 如果没有配置任何存储，则返回 nil
	if len(parts) == 0 {
		return nil
	}
	return parts
}

// runAllLocal 本地模式运行（不使用 Kafka）
// 数据流程：ingest → 内存 channel → aggregator → MySQL/Redis
func runAllLocal(ctx context.Context, cfg config.Config, log *slog.Logger) {
	// 创建内存 channel，用于 ingest 和 aggregator 之间的数据传递
	ch := make(chan []model.PacketRecord, 4096)

	// 构建数据存储 sink
	sink := buildSink(cfg, log)

	// 创建聚合服务实例
	svc, err := aggregator.NewService(cfg.Aggregator, cfg.ProbeID, sink, log)
	if err != nil {
		log.Error("aggregator", "err", err)
		os.Exit(1)
	}

	// 创建网卡选择通道，用于实时切换网卡
	ifaceCh := make(chan string, 1)
	// 如果配置了网卡，则预先设置
	if cfg.Ingest.LiveIface != "" {
		ifaceCh <- cfg.Ingest.LiveIface
	}

	// 如果配置了 MySQL，则启动 API 服务（协程）
	if cfg.MySQL.DSN != "" {
		go func() {
			runAPI(ctx, cfg, log, ifaceCh)
		}()
	}

	// 启动聚合服务（协程）
	go func() {
		if err := svc.Run(ctx, ch); err != nil && err != context.Canceled {
			log.Error("aggregator stopped", "err", err)
		}
	}()

	// 定义数据发布函数：将数据发送到内存 channel
	publish := func(ctx context.Context, batch []model.PacketRecord) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ch <- batch:
			return nil
		}
	}

	// 启动数据采集
	startIngest(ctx, cfg, log, publish, ifaceCh)

	// 等待退出信号
	<-ctx.Done()
	close(ch)
}

// runAllKafka Kafka 模式运行（分布式）
// 数据流程：ingest → Kafka → consumer → aggregator → MySQL/Redis
func runAllKafka(ctx context.Context, cfg config.Config, log *slog.Logger) {
	// 创建内存 channel
	ch := make(chan []model.PacketRecord, 4096)

	// 构建数据存储 sink
	sink := buildSink(cfg, log)

	// 创建聚合服务实例
	svc, err := aggregator.NewService(cfg.Aggregator, cfg.ProbeID, sink, log)
	if err != nil {
		log.Error("aggregator", "err", err)
		os.Exit(1)
	}

	// 启动聚合服务（协程）
	go func() {
		if err := svc.Run(ctx, ch); err != nil && err != context.Canceled {
			log.Error("aggregator stopped", "err", err)
		}
	}()

	// 创建 Kafka consumer（从 Kafka 消费数据）
	cons, err := queue.NewConsumer(cfg.Kafka.Brokers, cfg.Kafka.TopicRecords, cfg.Kafka.ConsumerGroup)
	if err != nil {
		log.Error("kafka consumer", "err", err)
		os.Exit(1)
	}
	defer cons.Close()

	// 启动 Kafka consumer（协程）
	go func() {
		err := cons.Run(ctx, func(ctx context.Context, batch []model.PacketRecord) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case ch <- batch:
				return nil
			}
		})
		if err != nil && err != context.Canceled {
			log.Error("kafka consume", "err", err)
		}
	}()

	// 创建 Kafka producer（发送数据到 Kafka）
	prod, err := queue.NewProducer(cfg.Kafka.Brokers, cfg.Kafka.TopicRecords)
	if err != nil {
		log.Error("kafka producer", "err", err)
		os.Exit(1)
	}
	defer prod.Close()

	// 创建网卡选择通道
	ifaceCh := make(chan string, 1)
	if cfg.Ingest.LiveIface != "" {
		ifaceCh <- cfg.Ingest.LiveIface
	}

	// 如果配置了 MySQL，则启动 API 服务（协程）
	if cfg.MySQL.DSN != "" {
		go func() {
			runAPI(ctx, cfg, log, ifaceCh)
		}()
	}

	// 定义数据发布函数：通过 Kafka 发送数据
	publish := func(ctx context.Context, batch []model.PacketRecord) error {
		return prod.PublishBatch(ctx, batch)
	}

	// 启动数据采集
	startIngest(ctx, cfg, log, publish, ifaceCh)

	// 等待退出信号
	<-ctx.Done()
	close(ch)
}

// runAggregateKafka 仅运行聚合服务模式
// 从 Kafka 消费数据，进行聚合，然后存��
func runAggregateKafka(ctx context.Context, cfg config.Config, log *slog.Logger) {
	// 创建内存 channel
	ch := make(chan []model.PacketRecord, 4096)

	// 构建数据存储 sink
	sink := buildSink(cfg, log)

	// 创建聚合服务实例
	svc, err := aggregator.NewService(cfg.Aggregator, cfg.ProbeID, sink, log)
	if err != nil {
		log.Error("aggregator", "err", err)
		os.Exit(1)
	}

	// 创建 Kafka consumer
	cons, err := queue.NewConsumer(cfg.Kafka.Brokers, cfg.Kafka.TopicRecords, cfg.Kafka.ConsumerGroup)
	if err != nil {
		log.Error("kafka consumer", "err", err)
		os.Exit(1)
	}
	defer cons.Close()

	// 启动 Kafka consumer（协程）
	go func() {
		err := cons.Run(ctx, func(ctx context.Context, batch []model.PacketRecord) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case ch <- batch:
				return nil
			}
		})
		if err != nil && err != context.Canceled {
			log.Error("kafka consume", "err", err)
		}
	}()

	// 运行聚合服务
	if err := svc.Run(ctx, ch); err != nil && err != context.Canceled {
		log.Error("aggregator", "err", err)
	}
	close(ch)
}

// runIngest 仅运行数据采集模式
// 启动数据采集，发送到 Kafka
func runIngest(ctx context.Context, cfg config.Config, log *slog.Logger) {
	// 创建 Kafka producer
	prod, err := queue.NewProducer(cfg.Kafka.Brokers, cfg.Kafka.TopicRecords)
	if err != nil {
		log.Error("kafka producer", "err", err)
		os.Exit(1)
	}
	defer prod.Close()

	// 定义数据发布函数：通过 Kafka 发送数据
	publish := func(ctx context.Context, batch []model.PacketRecord) error {
		return prod.PublishBatch(ctx, batch)
	}

	// 启动数据采集
	startIngest(ctx, cfg, log, publish, nil)

	// 等待退出信号
	<-ctx.Done()
}

// startIngest 根据配置启动数据采集
// 支持三种模式：live（实时网卡抓包）、pcap（离线文件）、synthetic（模拟数据）
// 参数：
//   - ctx: 上下文
//   - cfg: 配置信息
//   - log: 日志记录器
//   - publish: 数据发布函数
//   - ifaceCh: 网卡选择通道（可选）
func startIngest(ctx context.Context, cfg config.Config, log *slog.Logger, publish func(context.Context, []model.PacketRecord) error, ifaceCh <-chan string) {
	// 如果指定了 live 模式或网卡选择通道，则使用 live capture 模式
	if ifaceCh != nil || cfg.Ingest.Mode == "live" {
		// 读取超时时间
		readTimeout := time.Duration(cfg.Ingest.LiveReadTimeoutMs) * time.Millisecond
		if readTimeout <= 0 {
			readTimeout = 1000 * time.Millisecond
		}
		// 快照长度
		snapshotLen := cfg.Ingest.LiveSnapshotLen
		if snapshotLen <= 0 {
			snapshotLen = 1600
		}
		// BPF 过滤器
		bpf := cfg.Ingest.LiveBPFFilter
		if bpf == "" {
			bpf = "(ip or ip6) and (tcp or udp)"
		}

		// 启动 live capture（协程）
		go func() {
			log.Info("live capture mode: waiting for interface selection")
			if err := ingest.RunLive(
				ctx,
				cfg.Ingest.LiveIface,
				ifaceCh,
				cfg.Ingest.LivePromiscuous,
				snapshotLen,
				readTimeout,
				bpf,
				cfg.Ingest.BatchSize,
				publish,
			); err != nil && err != context.Canceled {
				log.Error("live", "err", err)
			}
		}()
		return
	}

	// 根据配置选择采集模式
	switch cfg.Ingest.Mode {
	case "pcap":
		// pcap 模式：读取离线 pcap 文件
		if cfg.Ingest.PCAPPath == "" {
			log.Error("pcap_path required")
			os.Exit(1)
		}
		ch := make(chan []model.PacketRecord, 256)
		// 启动 pcap 文件解析（协程）
		go func() {
			if err := parser.RunFile(ctx, cfg.Ingest.PCAPPath, ch, cfg.Ingest.BatchSize); err != nil && err != context.Canceled {
				log.Error("pcap", "err", err)
			}
			close(ch)
		}()
		// 启动数据发布（协程）
		go func() {
			for batch := range ch {
				if err := publish(ctx, batch); err != nil {
					return
				}
			}
		}()
	default:
		// synthetic 模式：生成模拟测试流量
		go func() {
			log.Info("synthetic traffic generation mode")
			if err := ingest.RunSynthetic(ctx, cfg.Ingest.SyntheticRPS, cfg.Ingest.BatchSize, publish); err != nil && err != context.Canceled {
				log.Error("synthetic", "err", err)
			}
		}()
	}
}

// exists 检查文件是否存在
// 参数：
//   - path: 文件路径
//
// 返回：
//   - bool: 文件是否存在
//   - error: 错误信息
func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
