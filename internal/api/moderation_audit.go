package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"doodlequest/internal/socialgraph"
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

// persistAudit writes an append-only operator-action record to exactly one
// sink: PostgreSQL when a social graph store that implements AuditStore is
// configured, otherwise the JSONL file. Call before applying a moderation
// mutation so failed audit writes fail closed. A configured-but-broken
// store fails closed too, rather than splitting the trail across sinks.
func (s *Server) persistAudit(ctx context.Context, ev moderationAuditEvent) error {
	if s.socialGraphErr != nil {
		return errors.New("shared social storage is unavailable")
	}
	if s.socialGraph != nil {
		if auditStore, ok := s.socialGraph.(socialgraph.AuditStore); ok {
			if strings.TrimSpace(ev.ActorID) == "" {
				ev.ActorID = "moderation-token"
			}
			ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			return auditStore.AppendAudit(ctx, socialgraph.AuditEvent{
				ID: ev.ID, Action: ev.Action, GameID: ev.GameID, ReportID: ev.ReportID,
				ActorID: ev.ActorID, Status: ev.Status, Reason: ev.Reason, CreatedAt: ev.CreatedAt,
			})
		}
	}
	return persistModerationAudit(ev)
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
	record := append(data, '\n')
	n, writeErr := f.Write(record)
	if writeErr == nil && n != len(record) {
		writeErr = io.ErrShortWrite
	}
	if writeErr == nil {
		// A successful Write may only have reached the OS page cache.
		// Sync before acknowledging the moderation mutation so a crash
		// is less likely to lose the audit record.
		writeErr = f.Sync()
	}
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
	// PostgreSQL sink, when configured, is the audit source of truth;
	// the JSONL file below is only the fallback for deployments without
	// a social graph store.
	if s.socialGraphErr == nil && s.socialGraph != nil {
		if auditStore, ok := s.socialGraph.(socialgraph.AuditStore); ok {
			var cursor int64
			if raw := strings.TrimSpace(r.URL.Query().Get("cursor")); raw != "" {
				n, err := strconv.ParseInt(raw, 10, 64)
				if err != nil || n < 0 {
					writeErr(w, http.StatusBadRequest, "cursor must be a non-negative sequence number")
					return
				}
				cursor = n
			}
			ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
			defer cancel()
			rows, next, err := auditStore.ListAudit(ctx, socialgraph.AuditFilter{
				GameID:  strings.TrimSpace(r.URL.Query().Get("game_id")),
				ActorID: strings.TrimSpace(r.URL.Query().Get("actor_id")),
				Action:  strings.TrimSpace(r.URL.Query().Get("action")),
				Limit:   limit,
				Cursor:  cursor,
			})
			if err != nil {
				writeErr(w, http.StatusServiceUnavailable, "could not read moderation audit")
				return
			}
			events := make([]moderationAuditEvent, 0, len(rows))
			for _, row := range rows {
				events = append(events, moderationAuditEvent{
					ID: row.ID, Action: row.Action, GameID: row.GameID, ReportID: row.ReportID,
					ActorID: row.ActorID, Status: row.Status, Reason: row.Reason, CreatedAt: row.CreatedAt,
				})
			}
			writeJSON(w, http.StatusOK, map[string]any{"events": events, "next_cursor": next})
			return
		}
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
