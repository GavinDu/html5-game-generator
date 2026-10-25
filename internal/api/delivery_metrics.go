package api

import (
	"net/http"
	"sort"
	"sync"
	"time"
)

type deliveryMetric struct {
	Requests       uint64 `json:"requests"`
	Responses2xx   uint64 `json:"responses_2xx"`
	NotFound       uint64 `json:"not_found"`
	NotModified    uint64 `json:"not_modified"`
	Errors         uint64 `json:"errors"`
	BytesWritten   uint64 `json:"bytes_written"`
	TotalLatencyMS uint64 `json:"total_latency_ms"`
	MaxLatencyMS   uint64 `json:"max_latency_ms"`
}

type deliveryResponseWriter struct {
	http.ResponseWriter
	status int
	bytes  uint64
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
	w.bytes += uint64(n)
	return n, err
}

// observeDelivery records only fixed endpoint classes, never game IDs or URLs.
// Metrics are process-local by design and reset on restart; they guide measurement,
// not billing, security decisions, or a durable analytics system.
func (s *Server) observeDelivery(kind string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &deliveryResponseWriter{ResponseWriter: w}
		next(recorder, r)
		if recorder.status == 0 {
			recorder.status = http.StatusOK
		}
		elapsed := uint64(time.Since(start).Milliseconds())
		s.deliveryMu.Lock()
		if s.deliveryStats == nil {
			s.deliveryStats = map[string]deliveryMetric{}
		}
		m := s.deliveryStats[kind]
		m.Requests++
		m.BytesWritten += recorder.bytes
		m.TotalLatencyMS += elapsed
		if elapsed > m.MaxLatencyMS {
			m.MaxLatencyMS = elapsed
		}
		switch {
		case recorder.status == http.StatusNotModified:
			m.NotModified++
		case recorder.status == http.StatusNotFound:
			m.NotFound++
		case recorder.status >= 500:
			m.Errors++
		case recorder.status >= 200 && recorder.status < 400:
			m.Responses2xx++
		default:
			m.Errors++
		}
		s.deliveryStats[kind] = m
		s.deliveryMu.Unlock()
	}
}

// GET /v1/telemetry/delivery returns bounded aggregate package/asset delivery
// counters. It is protected by the same admin token as other telemetry summaries.
func (s *Server) handleDeliveryMetrics(w http.ResponseWriter, r *http.Request) {
	if !requireTelemetryAdminToken(w, r) {
		return
	}
	s.deliveryMu.Lock()
	out := make(map[string]deliveryMetric, len(s.deliveryStats))
	for kind, metric := range s.deliveryStats {
		out[kind] = metric
	}
	s.deliveryMu.Unlock()
	kinds := make([]string, 0, len(out))
	for kind := range out {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	writeJSON(w, http.StatusOK, map[string]any{"metrics": out, "endpoint_kinds": kinds, "scope": "process-local-since-startup"})
}
