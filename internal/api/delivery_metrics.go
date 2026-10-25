package api

import (
	"net/http"
	"time"
)

// deliveryMetricSnapshot contains aggregate delivery health only. It deliberately
// excludes game IDs, user IDs, URLs, and request headers.
type deliveryMetricSnapshot struct {
	Requests       int64 `json:"requests"`
	Successes      int64 `json:"successes"`
	NotModified    int64 `json:"not_modified"`
	ClientErrors   int64 `json:"client_errors"`
	ServerErrors   int64 `json:"server_errors"`
	ResponseBytes  int64 `json:"response_bytes"`
	TotalLatencyMS int64 `json:"total_latency_ms"`
	MaxLatencyMS   int64 `json:"max_latency_ms"`
}

type deliveryResponseWriter struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (w *deliveryResponseWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *deliveryResponseWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(p)
	w.bytes += int64(n)
	return n, err
}

func (s *Server) observeDelivery(route string, next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		observed := &deliveryResponseWriter{ResponseWriter: w}
		next.ServeHTTP(observed, r)
		if observed.status == 0 {
			observed.status = http.StatusOK
		}
		s.recordDeliveryMetric(route, observed.status, observed.bytes, time.Since(start))
	})
}

func (s *Server) recordDeliveryMetric(route string, status int, bytes int64, elapsed time.Duration) {
	s.deliveryMu.Lock()
	defer s.deliveryMu.Unlock()
	if s.deliveryMetrics == nil {
		s.deliveryMetrics = map[string]*deliveryMetricSnapshot{}
	}
	m := s.deliveryMetrics[route]
	if m == nil {
		m = &deliveryMetricSnapshot{}
		s.deliveryMetrics[route] = m
	}
	m.Requests++
	m.ResponseBytes += bytes
	ms := elapsed.Milliseconds()
	m.TotalLatencyMS += ms
	if ms > m.MaxLatencyMS {
		m.MaxLatencyMS = ms
	}
	switch {
	case status == http.StatusNotModified:
		m.NotModified++
	case status >= 500:
		m.ServerErrors++
	case status >= 400:
		m.ClientErrors++
	default:
		m.Successes++
	}
}

func (s *Server) handleDeliveryMetrics(w http.ResponseWriter, r *http.Request) {
	if !requireTelemetryAdminToken(w, r) {
		return
	}
	s.deliveryMu.Lock()
	snapshot := make(map[string]deliveryMetricSnapshot, len(s.deliveryMetrics))
	for route, metrics := range s.deliveryMetrics {
		snapshot[route] = *metrics
	}
	s.deliveryMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"scope": "process_lifetime", "routes": snapshot})
}
