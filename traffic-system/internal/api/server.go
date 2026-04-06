package api

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"trafficd/internal/config"
	"trafficd/internal/ingest"
)

//go:embed web/*
var webFS embed.FS

type Server struct {
	cfg         config.Config
	db          *sql.DB
	log         *slog.Logger
	router      *http.ServeMux
	ifaceCh     chan<- string
	ifaceMu     sync.RWMutex
	currentIface string
}

type InterfaceInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func NewServer(cfg config.Config, db *sql.DB, log *slog.Logger, ifaceCh chan<- string) (*Server, error) {
	if db == nil {
		return nil, errors.New("mysql db is required for api")
	}
	if log == nil {
		log = slog.Default()
	}

	s := &Server{
		cfg:         cfg,
		db:          db,
		log:         log,
		router:      http.NewServeMux(),
		ifaceCh:     ifaceCh,
		currentIface: cfg.Ingest.LiveIface,
	}
	s.routes()
	return s, nil
}

func (s *Server) routes() {
	s.router.HandleFunc("/", s.handleIndex)
	s.router.HandleFunc("/api/latest", s.handleLatest)
	s.router.HandleFunc("/api/top-src", s.handleTopSrc)
	s.router.HandleFunc("/api/top-src-all", s.handleTopSrcAll)
	s.router.HandleFunc("/api/src-timeseries", s.handleSrcTimeseries)
	s.router.HandleFunc("/api/interfaces", s.handleInterfaces)
	s.router.HandleFunc("/api/select-interface", s.handleSelectInterface)
	// 可选：暴露静态文件
	if sub, err := fs.Sub(webFS, "web"); err == nil {
		s.router.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(sub))))
	}
}

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

	select {
	case <-ctx.Done():
		_ = srv.Shutdown(context.Background())
		return nil
	case err := <-errCh:
		return err
	}
}

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

func (s *Server) getCurrentIface() string {
	s.ifaceMu.RLock()
	defer s.ifaceMu.RUnlock()
	return s.currentIface
}

func (s *Server) setCurrentIface(iface string) {
	s.ifaceMu.Lock()
	defer s.ifaceMu.Unlock()
	s.currentIface = iface
}

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

type LatestResp struct {
	ProbeID       string      `json:"probe_id"`
	SecTs         time.Time   `json:"sec_ts"`
	SrcsUnique    int64       `json:"srcs_unique"`
	TotalFlowSize uint64      `json:"total_flow_size"` // 该秒内所有 src 的包数之和（每包计 1）
	TopSrcs       []SrcRowTop `json:"top_srcs"`
}

type SrcRowTop struct {
	SrcIP                 string `json:"src_ip"`
	FlowSize              uint64 `json:"flow_size"`
	DstPortCardinalityEst uint64 `json:"dst_port_cardinality_est"`
}

func (s *Server) handleLatest(w http.ResponseWriter, r *http.Request) {
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
		var dstPortEst float64
		if err := rows.Scan(&srcIP, &flowSize, &dstPortEst); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		top = append(top, SrcRowTop{
			SrcIP:                 srcIP,
			FlowSize:              flowSize,
			DstPortCardinalityEst: uint64(dstPortEst),
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

type TopSrcResp struct {
	ProbeID string         `json:"probe_id"`
	FromTs  time.Time      `json:"from_ts"`
	ToTs    time.Time      `json:"to_ts"`
	Top     []SrcRowTopSum `json:"top"`
}

type SrcRowTopSum struct {
	SrcIP                 string `json:"src_ip"`
	TotalFlowSize         uint64 `json:"total_flow_size"`
	DstPortCardinalityEst uint64 `json:"dst_port_cardinality_est"`
}

func (s *Server) handleTopSrc(w http.ResponseWriter, r *http.Request) {
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

	to := time.Now().UTC().Truncate(time.Second)
	var from time.Time
	if windowSec == 60 {
		from = to.Truncate(time.Minute)
	} else {
		from = to.Add(-time.Duration(windowSec) * time.Second)
	}

	rows, err := s.db.Query(`
		SELECT src_ip,
		       COALESCE(SUM(flow_size),0) AS total_flow_size,
		       COALESCE(SUM(dst_port_cardinality_est),0) AS dst_port_cardinality_est
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
		var dstPortEst float64
		if err := rows.Scan(&srcIP, &tf, &dstPortEst); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		top = append(top, SrcRowTopSum{SrcIP: srcIP, TotalFlowSize: tf, DstPortCardinalityEst: uint64(dstPortEst)})
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
			top = append(top, SrcRowTopSum{SrcIP: srcIP, TotalFlowSize: 0, DstPortCardinalityEst: 0})
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

func (s *Server) handleTopSrcAll(w http.ResponseWriter, r *http.Request) {
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
		       COALESCE(SUM(dst_port_cardinality_est),0) AS dst_port_cardinality_est
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
		var dstPortEst float64
		if err := rows.Scan(&srcIP, &tf, &dstPortEst); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		top = append(top, SrcRowTopSum{SrcIP: srcIP, TotalFlowSize: tf, DstPortCardinalityEst: uint64(dstPortEst)})
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

type SeriesResp struct {
	ProbeID string            `json:"probe_id"`
	SrcIP   string            `json:"src_ip"`
	FromTs  time.Time         `json:"from_ts"`
	ToTs    time.Time         `json:"to_ts"`
	Points  []SeriesPointResp `json:"points"`
}

type SeriesPointResp struct {
	SecTs                 time.Time `json:"sec_ts"`
	FlowSize              uint64    `json:"flow_size"`
	DstPortCardinalityEst uint64    `json:"dst_port_cardinality_est"`
}

func (s *Server) handleSrcTimeseries(w http.ResponseWriter, r *http.Request) {
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

	to := time.Now().UTC().Truncate(time.Second)
	var from time.Time
	if align == "minute" {
		from = to.Truncate(time.Minute)
	} else {
		from = to.Add(-time.Duration(windowSec) * time.Second)
	}

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
			SecTs:                 secTs,
			FlowSize:              flowSize,
			DstPortCardinalityEst: uint64(dstEst),
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
