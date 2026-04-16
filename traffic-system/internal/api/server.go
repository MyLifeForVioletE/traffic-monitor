package api

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"trafficd/internal/config"
	"trafficd/internal/ingest"
)

// webFS 嵌入的静态 web 文件（web/ 目录下的所有文件）
//
//go:embed web/*
var webFS embed.FS

// Server HTTP API 服务器结构体
type Server struct {
	cfg          config.Config  // 配置信息
	db           *sql.DB        // MySQL 数据库连接
	log          *slog.Logger   // 日志记录器
	router       *http.ServeMux // HTTP 路由 multiplexer
	ifaceCh      chan<- string  // 网卡选择通道，用于实时切换网卡
	ifaceMu      sync.RWMutex   // 保护 currentIface 的读写锁
	currentIface string         // 当前选中的网卡名称
	autoOpen     bool           // 是否自动打开浏览器
}

// InterfaceInfo 网卡信息结构体，用于 API 返回
type InterfaceInfo struct {
	Name        string `json:"name"`        // 网卡名称
	Description string `json:"description"` // 网卡描述
}

// NewServer 创建新的 API 服务器实例
// 参数：
//   - cfg: 配置信息
//   - db: MySQL 数据库连接
//   - log: 日志记录器
//   - ifaceCh: 网卡选择通道（可选，用于实时切换网卡）
//
// 返回：
//   - *Server: API 服务器实例
//   - error: 错误信息
func NewServer(cfg config.Config, db *sql.DB, log *slog.Logger, ifaceCh chan<- string) (*Server, error) {
	if log == nil {
		log = slog.Default()
	}
	if db == nil {
		log.Warn("mysql db not provided, api queries will fail")
	}

	s := &Server{
		cfg:          cfg,
		db:           db,
		log:          log,
		router:       http.NewServeMux(),
		ifaceCh:      ifaceCh,
		currentIface: cfg.Ingest.LiveIface,
		autoOpen:     cfg.HTTP.AutoOpenBrowser,
	}
	s.routes()
	return s, nil
}

// routes 注册 HTTP 路由处理函数
func (s *Server) routes() {
	s.router.HandleFunc("/", s.handleIndex)                               // 首页
	s.router.HandleFunc("/api/latest", s.handleLatest)                    // 最新聚合数据
	s.router.HandleFunc("/api/top-src", s.handleTopSrc)                   // Top N 源IP
	s.router.HandleFunc("/api/top-src-all", s.handleTopSrcAll)            // 全量 Top N 源IP
	s.router.HandleFunc("/api/src-timeseries", s.handleSrcTimeseries)     // 源IP 时序数据
	s.router.HandleFunc("/api/interfaces", s.handleInterfaces)            // 网卡列表
	s.router.HandleFunc("/api/select-interface", s.handleSelectInterface) // 选择网卡
	// 可选：暴露静态文件
	if sub, err := fs.Sub(webFS, "web"); err == nil {
		s.router.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(sub))))
	}
}

// Run 启动 HTTP 服务器并阻塞直到上下文取消
// 参数：
//   - ctx: 上下文，用于优雅退出
//
// 返回：
//   - error: 错误信息
func (s *Server) Run(ctx context.Context) error {
	addr := s.cfg.HTTP.Addr
	if strings.TrimSpace(addr) == "" {
		addr = ":8080"
	}

	srv := &http.Server{
		Addr:    addr,
		Handler: s.router,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	// 如果配置了自动打开浏览器，则在服务器启动后自动打开
	if s.autoOpen {
		time.Sleep(500 * time.Millisecond)
		go func() {
			u := "http://localhost"
			if !strings.Contains(addr, ":") {
				u += addr
			} else if strings.HasPrefix(addr, ":") {
				u += addr
			} else {
				u = "http://" + addr
			}
			s.log.Info("opening browser", "url", u)
			exec.Command("cmd", "/c", "start", u).Start()
		}()
	}

	select {
	case <-ctx.Done():
		_ = srv.Shutdown(context.Background())
		return nil
	case err := <-errCh:
		return err
	}
}

// handleIndex 处理首页请求，返回 index.html
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	// 先给 index.html，其他路径由 /static 处理
	b, err := webFS.ReadFile("web/index.html")
	if err != nil {
		http.Error(w, "index not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(b)
}

// getCurrentIface 获取当前选中的网卡名称（线程安全）
func (s *Server) getCurrentIface() string {
	s.ifaceMu.RLock()
	defer s.ifaceMu.RUnlock()
	return s.currentIface
}

// setCurrentIface 设置当前选中的网卡名称（线程安全）
func (s *Server) setCurrentIface(iface string) {
	s.ifaceMu.Lock()
	defer s.ifaceMu.Unlock()
	s.currentIface = iface
}

// handleInterfaces 处理网卡列表请求，返回可用的网卡设备
func (s *Server) handleInterfaces(w http.ResponseWriter, r *http.Request) {
	devices, err := ingest.ListDevices()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	infos := make([]InterfaceInfo, 0, len(devices))
	for _, d := range devices {
		infos = append(infos, InterfaceInfo{Name: d.Name, Description: d.Description})
	}
	resp := struct {
		Current    string          `json:"current"`
		Interfaces []InterfaceInfo `json:"interfaces"`
	}{
		Current:    s.getCurrentIface(),
		Interfaces: infos,
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(resp)
}

// handleSelectInterface 处理网卡选择请求
func (s *Server) handleSelectInterface(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	iface := r.URL.Query().Get("iface")
	if iface == "" {
		http.Error(w, "iface is required", http.StatusBadRequest)
		return
	}
	if s.ifaceCh == nil {
		http.Error(w, "interface selection unavailable", http.StatusServiceUnavailable)
		return
	}
	select {
	case s.ifaceCh <- iface:
	default:
		select {
		case <-time.After(1 * time.Second):
			http.Error(w, "failed to send interface change", http.StatusInternalServerError)
			return
		case s.ifaceCh <- iface:
		}
	}
	s.setCurrentIface(iface)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(struct {
		Iface string `json:"iface"`
	}{Iface: iface})
}

// LatestResp 最新聚合数据响应结构体
type LatestResp struct {
	ProbeID       string      `json:"probe_id"`        // 探针 ID
	SecTs         time.Time   `json:"sec_ts"`          // 秒级时间戳
	SrcsUnique    int64       `json:"srcs_unique"`     // 唯一源 IP 数量
	TotalFlowSize uint64      `json:"total_flow_size"` // 该秒内所有 src 的包数之和（每包计 1）
	TopSrcs       []SrcRowTop `json:"top_srcs"`        // Top N 源 IP
}

// SrcRowTop 源 IP 聚合数据行
type SrcRowTop struct {
	SrcIP           string `json:"src_ip"`           // 源 IP 地址
	FlowSize        uint64 `json:"flow_size"`        // 流大小（包数）
	FlowCardinality uint64 `json:"flow_cardinality"` // 目的端口基数估计
}

// handleLatest 处理最新聚合数据请求
func (s *Server) handleLatest(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		http.Error(w, "database not available", http.StatusServiceUnavailable)
		return
	}
	probeID := s.cfg.ProbeID
	if q := r.URL.Query().Get("probe_id"); q != "" {
		probeID = q
	}
	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	// 查询最新的秒级时间戳
	var secTs sql.NullTime
	if err := s.db.QueryRow(`SELECT MAX(sec_ts) FROM agg_src_second WHERE probe_id=?`, probeID).Scan(&secTs); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !secTs.Valid {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(LatestResp{
			ProbeID:       probeID,
			SecTs:         time.Time{},
			SrcsUnique:    0,
			TotalFlowSize: 0,
			TopSrcs:       nil,
		})
		return
	}

	// 查询该秒内的唯一源 IP 数量和总包数
	var srcsUnique int64
	var totalFlowSize uint64
	if err := s.db.QueryRow(`
		SELECT COUNT(*) AS srcs_unique, COALESCE(SUM(flow_size),0)
		FROM agg_src_second
		WHERE probe_id=? AND sec_ts=?
	`, probeID, secTs.Time).Scan(&srcsUnique, &totalFlowSize); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 查询 Top N 源 IP
	rows, err := s.db.Query(`
		SELECT src_ip, flow_size, dst_port_cardinality_est
		FROM agg_src_second
		WHERE probe_id=? AND sec_ts=?
		ORDER BY flow_size DESC
		LIMIT ?
	`, probeID, secTs.Time, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	top := make([]SrcRowTop, 0, limit)
	for rows.Next() {
		var srcIP string
		var flowSize uint64
		var dstCard uint64
		if err := rows.Scan(&srcIP, &flowSize, &dstCard); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		top = append(top, SrcRowTop{
			SrcIP:           srcIP,
			FlowSize:        flowSize,
			FlowCardinality: dstCard,
		})
	}

	resp := LatestResp{
		ProbeID:       probeID,
		SecTs:         secTs.Time,
		SrcsUnique:    srcsUnique,
		TotalFlowSize: totalFlowSize,
		TopSrcs:       top,
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(resp)
}

// TopSrcResp Top N 源 IP 响应结构体
type TopSrcResp struct {
	ProbeID string         `json:"probe_id"` // 探针 ID
	FromTs  time.Time      `json:"from_ts"`  // 起始时间
	ToTs    time.Time      `json:"to_ts"`    // 结束时间
	Top     []SrcRowTopSum `json:"top"`      // Top N 源 IP
}

// SrcRowTopSum 源 IP 聚合数据行（用于 Top N 查询）
type SrcRowTopSum struct {
	SrcIP             string `json:"src_ip"`           // 源 IP 地址
	TotalFlowSize     uint64 `json:"total_flow_size"`  // 总流大小（包数）
	DstCardinalityEst uint64 `json:"flow_cardinality"` // 目的端口基数估计
}

// handleTopSrc 处理 Top N 源 IP 请求（时间窗口内）
func (s *Server) handleTopSrc(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		http.Error(w, "database not available", http.StatusServiceUnavailable)
		return
	}
	probeID := s.cfg.ProbeID
	if q := r.URL.Query().Get("probe_id"); q != "" {
		probeID = q
	}
	windowSec := 60
	if v := r.URL.Query().Get("window_sec"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			windowSec = n
		}
	}
	topN := 10
	if v := r.URL.Query().Get("top_n"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			topN = n
		}
	}

	to := time.Now().Truncate(time.Second)
	var from time.Time
	if windowSec == 60 {
		from = to.Truncate(time.Minute).Add(-time.Minute)
	} else {
		from = to.Add(-time.Duration(windowSec) * time.Second)
	}

	rows, err := s.db.Query(`
		   SELECT src_ip,
				  COALESCE(SUM(flow_size),0) AS total_flow_size,
				  COALESCE(SUM(dst_port_cardinality_est),0) AS total_cardinality
		   FROM agg_src_second
		   WHERE probe_id=? AND sec_ts>=? AND sec_ts<=?
		   GROUP BY src_ip
		   ORDER BY total_flow_size DESC
		   LIMIT ?
	   `, probeID, from, to, topN)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	top := make([]SrcRowTopSum, 0, topN)
	for rows.Next() {
		var srcIP string
		var tf uint64
		var card uint64
		if err := rows.Scan(&srcIP, &tf, &card); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		top = append(top, SrcRowTopSum{SrcIP: srcIP, TotalFlowSize: tf, DstCardinalityEst: card})
	}

	// 如果当前分钟无数据且为60秒窗口，用上一分钟的源IP列表（流大小/基数显示为0）
	if len(top) == 0 && windowSec == 60 {
		prevFrom := from.Add(-time.Minute)
		prevRows, err := s.db.Query(`
			SELECT DISTINCT src_ip
			FROM agg_src_second
			WHERE probe_id=? AND sec_ts>=? AND sec_ts<?
			ORDER BY src_ip
			LIMIT ?
		`, probeID, prevFrom, from, topN)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer prevRows.Close()

		for prevRows.Next() {
			var srcIP string
			if err := prevRows.Scan(&srcIP); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			top = append(top, SrcRowTopSum{SrcIP: srcIP, TotalFlowSize: 0})
		}
	}

	resp := TopSrcResp{
		ProbeID: probeID,
		FromTs:  from,
		ToTs:    to,
		Top:     top,
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(resp)
}

// handleTopSrcAll 处理全量 Top N 源 IP 请求（所有历史数据）
func (s *Server) handleTopSrcAll(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		http.Error(w, "database not available", http.StatusServiceUnavailable)
		return
	}
	probeID := s.cfg.ProbeID
	if q := r.URL.Query().Get("probe_id"); q != "" {
		probeID = q
	}
	topN := 20
	if v := r.URL.Query().Get("top_n"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			topN = n
		}
	}

	rows, err := s.db.Query(`
		SELECT src_ip,
		       COALESCE(SUM(flow_size),0) AS total_flow_size,
		       COALESCE(SUM(dst_port_cardinality_est),0) AS total_cardinality
		FROM agg_src_second
		WHERE probe_id=?
		GROUP BY src_ip
		ORDER BY total_flow_size DESC
		LIMIT ?
	`, probeID, topN)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	top := make([]SrcRowTopSum, 0, topN)
	for rows.Next() {
		var srcIP string
		var tf uint64
		var card uint64
		if err := rows.Scan(&srcIP, &tf, &card); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		top = append(top, SrcRowTopSum{SrcIP: srcIP, TotalFlowSize: tf, DstCardinalityEst: card})
	}

	resp := TopSrcResp{
		ProbeID: probeID,
		FromTs:  time.Time{},
		ToTs:    time.Time{},
		Top:     top,
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(resp)
}

// SeriesResp 时序数据响应结构体
type SeriesResp struct {
	ProbeID string            `json:"probe_id"` // 探针 ID
	SrcIP   string            `json:"src_ip"`   // 源 IP 地址
	FromTs  time.Time         `json:"from_ts"`  // 起始时间
	ToTs    time.Time         `json:"to_ts"`    // 结束时间
	Points  []SeriesPointResp `json:"points"`   // 数据点列表
}

// SeriesPointResp 时序数据点
type SeriesPointResp struct {
	SecTs           time.Time `json:"sec_ts"`           // 秒级时间戳
	FlowSize        uint64    `json:"flow_size"`        // 流大小（包数）
	FlowCardinality uint64    `json:"flow_cardinality"` // 目的端口基数估计
}

// handleSrcTimeseries 处理源 IP 时序数据请求
func (s *Server) handleSrcTimeseries(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		http.Error(w, "database not available", http.StatusServiceUnavailable)
		return
	}
	probeID := s.cfg.ProbeID
	if q := r.URL.Query().Get("probe_id"); q != "" {
		probeID = q
	}
	srcIP := r.URL.Query().Get("src_ip")
	if srcIP == "" {
		http.Error(w, "src_ip required", http.StatusBadRequest)
		return
	}

	windowSec := 60
	if v := r.URL.Query().Get("window_sec"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			windowSec = n
		}
	}
	align := r.URL.Query().Get("align")

	to := time.Now().Truncate(time.Second)
	var from time.Time
	if align == "minute" {
		from = to.Truncate(time.Minute)
	} else {
		from = to.Add(-time.Duration(windowSec) * time.Second)
	}
	_ = align

	rows, err := s.db.Query(`
		SELECT sec_ts, flow_size, dst_port_cardinality_est
		FROM agg_src_second
		WHERE probe_id=? AND src_ip=? AND sec_ts>=? AND sec_ts<=?
		ORDER BY sec_ts ASC
	`, probeID, srcIP, from, to)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	points := make([]SeriesPointResp, 0, windowSec)
	for rows.Next() {
		var secTs time.Time
		var flowSize uint64
		var dstEst float64
		if err := rows.Scan(&secTs, &flowSize, &dstEst); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		points = append(points, SeriesPointResp{
			SecTs:           secTs,
			FlowSize:        flowSize,
			FlowCardinality: uint64(dstEst),
		})
	}

	resp := SeriesResp{
		ProbeID: probeID,
		SrcIP:   srcIP,
		FromTs:  from,
		ToTs:    to,
		Points:  points,
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(resp)
}
