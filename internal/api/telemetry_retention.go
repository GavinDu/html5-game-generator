package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// pruneTelemetryOnStartup is opt-in: operators set DQ_TELEMETRY_RETENTION_DAYS
// to a value from 1 to 3650. Leaving it unset never deletes existing telemetry.
func pruneTelemetryOnStartup() error {
	raw := strings.TrimSpace(os.Getenv("DQ_TELEMETRY_RETENTION_DAYS"))
	if raw == "" {
		return nil
	}
	days, err := strconv.Atoi(raw)
	if err != nil || days < 1 || days > 3650 {
		return fmt.Errorf("DQ_TELEMETRY_RETENTION_DAYS must be an integer from 1 to 3650")
	}
	return pruneTelemetryFiles(telemetryDir(), days, time.Now().UTC())
}

// pruneTelemetryFiles removes expired JSONL records from known operational
// telemetry files. It preserves malformed/undated lines rather than risking
// deletion when a timestamp cannot be interpreted, and atomically replaces
// each file only when at least one expired record was removed.
func pruneTelemetryFiles(dir string, days int, now time.Time) error {
	if days < 1 || days > 3650 {
		return fmt.Errorf("retention days must be from 1 to 3650")
	}
	cutoff := now.Add(-time.Duration(days) * 24 * time.Hour)
	paths := []string{
		filepath.Join(dir, "events.jsonl"),
		filepath.Join(dir, "runtime-errors.jsonl"),
		moderationAuditPath(),
	}
	seen := map[string]bool{}
	telemetryMu.Lock()
	defer telemetryMu.Unlock()
	for _, path := range paths {
		path = filepath.Clean(path)
		if seen[path] {
			continue
		}
		seen[path] = true
		if err := pruneJSONLFile(path, cutoff); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("prune %s: %w", path, err)
		}
	}
	return nil
}

func pruneJSONLFile(path string, cutoff time.Time) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var kept bytes.Buffer
	removed := false
	for _, line := range bytes.Split(raw, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		at, ok := telemetryRecordTime(line)
		if ok && at.Before(cutoff) {
			removed = true
			continue
		}
		kept.Write(line)
		kept.WriteByte('\n')
	}
	if !removed {
		return nil
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".telemetry-retention-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(kept.Bytes()); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func telemetryRecordTime(line []byte) (time.Time, bool) {
	var record map[string]json.RawMessage
	if err := json.Unmarshal(line, &record); err != nil {
		return time.Time{}, false
	}
	for _, key := range []string{"receivedAt", "created_at", "createdAt", "at"} {
		var value string
		if err := json.Unmarshal(record[key], &value); err != nil || value == "" {
			continue
		}
		if at, err := time.Parse(time.RFC3339Nano, value); err == nil {
			return at, true
		}
	}
	return time.Time{}, false
}
