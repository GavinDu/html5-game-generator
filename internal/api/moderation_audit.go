package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type moderationAuditEvent struct {
	ID        string    `json:"id"`
	Action    string    `json:"action"`
	GameID    string    `json:"game_id"`
	ReportID  string    `json:"report_id,omitempty"`
	ActorID   string    `json:"actor_id"`
	Status    string    `json:"status,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func moderationAuditPath() string {
	if path := strings.TrimSpace(os.Getenv("DQ_MODERATION_AUDIT_FILE")); path != "" {
		return path
	}
	return filepath.Join(telemetryDir(), "moderation-audit.jsonl")
}

// persistModerationAudit writes an append-only operator-action record.
// Call before applying a moderation mutation so failed audit writes fail closed.
func persistModerationAudit(ev moderationAuditEvent) error {
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	telemetryMu.Lock()
	defer telemetryMu.Unlock()
	path := moderationAuditPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(append(data, '\n'))
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

// GET /v1/moderation/audit — authenticated operator view of append-only
// report and moderation actions. Actor identity remains the development
// X-User-ID until the separate authentication backlog is implemented.
func (s *Server) handleModerationAudit(w http.ResponseWriter, r *http.Request) {
	if !requireModerationToken(w, r) {
		return
	}
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 500 {
			writeErr(w, http.StatusBadRequest, "limit must be between 1 and 500")
			return
		}
		limit = n
	}
	data, err := os.ReadFile(moderationAuditPath())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		writeErr(w, http.StatusInternalServerError, "could not read moderation audit")
		return
	}
	gameID := strings.TrimSpace(r.URL.Query().Get("game_id"))
	events := make([]moderationAuditEvent, 0)
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var ev moderationAuditEvent
		if json.Unmarshal([]byte(line), &ev) != nil || ev.ID == "" || ev.Action == "" {
			// Do not silently present a partial audit trail as complete. A
			// malformed line may indicate truncation or tampering; fail closed
			// so operators investigate the underlying file before acting.
			writeErr(w, http.StatusServiceUnavailable, "moderation audit contains an invalid record")
			return
		}
		if gameID == "" || ev.GameID == gameID {
			events = append(events, ev)
		}
	}
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].CreatedAt.After(events[j].CreatedAt)
	})
	if len(events) > limit {
		events = events[:limit]
	}
	if events == nil {
		events = []moderationAuditEvent{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}
