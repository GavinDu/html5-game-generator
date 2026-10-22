package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Runtime-error telemetry (auto detect-and-fix flow). The player page is
// bridgeless in the iOS WebView, so it reports its own uncaught errors via
// navigator.sendBeacon. Reports land as JSONL, grouped by a stable
// fingerprint: the same failure on many sessions — or on 2+ games, which
// marks it a runtime bug rather than a content bug — is one incident.

type runtimeErrorReport struct {
	GameID         string   `json:"gameId"`
	RuntimeVersion string   `json:"runtimeVersion"`
	UserAgent      string   `json:"userAgent"`
	Kind           string   `json:"kind"`
	Name           string   `json:"name"`
	Message        string   `json:"message"`
	Stack          string   `json:"stack"`
	At             string   `json:"at"`
	LogTail        []string `json:"logTail"`
}

var (
	telemetryMu  sync.Mutex
	digitsRe     = regexp.MustCompile(`\d+`)
	hexIDRe      = regexp.MustCompile(`\b[0-9a-f]{8,}\b`)
	whitespaceRe = regexp.MustCompile(`\s+`)
)

// fingerprintRuntimeError hashes the error's identity with volatile detail
// (numbers, hex ids) stripped, plus the runtime version: the same bug on
// the same build maps to one fingerprint wherever it happens.
func fingerprintRuntimeError(rep *runtimeErrorReport) string {
	msg := hexIDRe.ReplaceAllString(strings.ToLower(rep.Message), "#")
	msg = digitsRe.ReplaceAllString(msg, "#")
	msg = whitespaceRe.ReplaceAllString(strings.TrimSpace(msg), " ")
	frames := []string{}
	for _, line := range strings.Split(rep.Stack, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Keep the tail of each frame (source file + position); drop the
		// URL scheme/host so the same build fingerprints identically
		// wherever it is hosted.
		if i := strings.LastIndex(line, "/"); i >= 0 {
			line = line[i+1:]
		}
		frames = append(frames, line)
		if len(frames) == 3 {
			break
		}
	}
	h := sha256.Sum256([]byte(strings.Join([]string{rep.Name, msg, strings.Join(frames, "|"), rep.RuntimeVersion}, "\n")))
	return hex.EncodeToString(h[:])[:16]
}

// POST /v1/telemetry/runtime-error — beacon target; accepts, fingerprints,
// appends one JSONL record, and answers 202. Storage is a file for now
// (DQ_TELEMETRY_DIR, default ./telemetry); the incident store is a later
// phase of the flow.
func (s *Server) handleRuntimeError(w http.ResponseWriter, r *http.Request) {
	var rep runtimeErrorReport
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	if err := dec.Decode(&rep); err != nil {
		http.Error(w, "bad report", http.StatusBadRequest)
		return
	}
	rec := map[string]any{
		"receivedAt":  time.Now().UTC().Format(time.RFC3339),
		"fingerprint": fingerprintRuntimeError(&rep),
		"report":      rep,
	}
	if line, err := json.Marshal(rec); err == nil {
		telemetryMu.Lock()
		dir := os.Getenv("DQ_TELEMETRY_DIR")
		if dir == "" {
			dir = "telemetry"
		}
		if mkErr := os.MkdirAll(dir, 0o755); mkErr == nil {
			if f, ferr := os.OpenFile(filepath.Join(dir, "runtime-errors.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); ferr == nil {
				_, _ = f.Write(append(line, '\n'))
				_ = f.Close()
			}
		}
		telemetryMu.Unlock()
	}
	w.WriteHeader(http.StatusAccepted)
}
