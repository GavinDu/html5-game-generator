package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDeliveryMetricsAggregateWithoutIdentityData(t *testing.T) {
	t.Setenv("DQ_TELEMETRY_ADMIN_TOKEN", "test-secret")
	s := &Server{}
	handler := s.observeDelivery("package", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte("{\"ok\":true}"))
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/games/private-game/package", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "{\"ok\":true}" { t.Fatalf("delivery response changed: status=%d body=%q", rec.Code, rec.Body.String()) }
	adminReq := httptest.NewRequest(http.MethodGet, "/v1/telemetry/delivery", nil)
	adminReq.Header.Set("X-Telemetry-Admin-Token", "test-secret")
	adminRec := httptest.NewRecorder()
	s.handleDeliveryMetrics(adminRec, adminReq)
	if adminRec.Code != http.StatusOK { t.Fatalf("admin metrics status=%d body=%s", adminRec.Code, adminRec.Body.String()) }
	var result struct {
		Scope string `json:"scope"`
		Routes map[string]deliveryMetricSnapshot `json:"routes"`
	}
	if err := json.Unmarshal(adminRec.Body.Bytes(), &result); err != nil { t.Fatal(err) }
	m := result.Routes["package"]
	if result.Scope != "process_lifetime" || m.Requests != 1 || m.Successes != 1 || m.ResponseBytes != int64(len("{\"ok\":true}")) { t.Fatalf("unexpected metrics: %#v", result) }
	if strings.Contains(adminRec.Body.String(), "private-game") || strings.Contains(adminRec.Body.String(), "test-secret") { t.Fatalf("metrics leaked identity/path/secret: %s", adminRec.Body.String()) }
}

func TestDeliveryMetricsStatusBucketsAndTokenProtection(t *testing.T) {
	s := &Server{}
	s.recordDeliveryMetric("asset", http.StatusNotModified, 0, 5*time.Millisecond)
	s.recordDeliveryMetric("asset", http.StatusNotFound, 23, 8*time.Millisecond)
	s.recordDeliveryMetric("asset", http.StatusServiceUnavailable, 11, 12*time.Millisecond)
	t.Setenv("DQ_TELEMETRY_ADMIN_TOKEN", "secret")
	unauthorized := httptest.NewRecorder()
	s.handleDeliveryMetrics(unauthorized, httptest.NewRequest(http.MethodGet, "/v1/telemetry/delivery", nil))
	if unauthorized.Code != http.StatusForbidden { t.Fatalf("missing token should be forbidden, got %d", unauthorized.Code) }
	authorizedReq := httptest.NewRequest(http.MethodGet, "/v1/telemetry/delivery", nil)
	authorizedReq.Header.Set("X-Telemetry-Admin-Token", "secret")
	authorized := httptest.NewRecorder()
	s.handleDeliveryMetrics(authorized, authorizedReq)
	var result struct { Routes map[string]deliveryMetricSnapshot `json:"routes"` }
	if err := json.Unmarshal(authorized.Body.Bytes(), &result); err != nil { t.Fatal(err) }
	m := result.Routes["asset"]
	if m.Requests != 3 || m.NotModified != 1 || m.ClientErrors != 1 || m.ServerErrors != 1 || m.ResponseBytes != 34 || m.MaxLatencyMS != 12 { t.Fatalf("unexpected status metrics: %#v", m) }
}