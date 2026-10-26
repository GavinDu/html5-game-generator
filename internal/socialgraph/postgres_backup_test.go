package socialgraph

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeBackupEnvelope(t *testing.T, version int, snapshot string) string {
	t.Helper()
	sumBytes := sha256.Sum256([]byte(snapshot))
	sum := hex.EncodeToString(sumBytes[:])
	envelope := postgresBackupEnvelope{
		FormatVersion: version,
		SHA256:        sum,
		Snapshot:      json.RawMessage(snapshot),
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "backup.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestWritePostgresBackupAtomicallyIsPrivateAndReplacesWholeFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "social-graph.json")
	if err := writePostgresBackupAtomically(path, []byte(`{"formatVersion":4}`)); err != nil {
		t.Fatalf("write initial backup: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("backup mode = %o, want 600", mode)
	}
	if err := writePostgresBackupAtomically(path, []byte(`{"formatVersion":5}`)); err != nil {
		t.Fatalf("replace backup: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"formatVersion":5}` {
		t.Fatalf("backup contents = %q, want complete replacement", data)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temporary backup remains: %v", err)
	}
}

func TestWritePostgresBackupAtomicallyRequiresPath(t *testing.T) {
	if err := writePostgresBackupAtomically("", []byte("x")); err == nil {
		t.Fatal("empty backup path unexpectedly succeeded")
	}
}

func TestReadPostgresBackupV2TreatsMissingNewCollectionsAsEmpty(t *testing.T) {
	path := writeBackupEnvelope(t, 2, `{"follows":[],"blocks":[],"saves":[],"comments":[],"commentIdempotency":[]}`)
	snapshot, err := readPostgresBackup(path)
	if err != nil {
		t.Fatalf("read v2 backup: %v", err)
	}
	if snapshot.NotInterested == nil || snapshot.MutedTopics == nil || snapshot.Reactions == nil || snapshot.Reports == nil {
		t.Fatalf("v2 backup new collections = %v/%v/%v/%v, want empty non-nil slices",
			snapshot.NotInterested, snapshot.MutedTopics, snapshot.Reactions, snapshot.Reports)
	}
}

func TestReadPostgresBackupV3RequiresNewCollections(t *testing.T) {
	path := writeBackupEnvelope(t, 3, `{"follows":[],"blocks":[],"saves":[],"comments":[],"commentIdempotency":[]}`)
	if _, err := readPostgresBackup(path); err == nil || !strings.Contains(err.Error(), "missing required collections") {
		t.Fatalf("read v3 backup without new collections error = %v, want missing required collections", err)
	}
}

func TestReadPostgresBackupV3ValidatesNewCollections(t *testing.T) {
	cases := []struct {
		name     string
		snapshot string
	}{
		{
			"invalid reaction",
			`{"follows":[],"blocks":[],"saves":[],"comments":[],"commentIdempotency":[],"notInterested":[],"mutedTopics":[],"reactions":[{"ViewerID":"v","GameID":"g","Reaction":"hate"}],"reports":[]}`,
		},
		{
			"invalid report status",
			`{"follows":[],"blocks":[],"saves":[],"comments":[],"commentIdempotency":[],"notInterested":[],"mutedTopics":[],"reactions":[],"reports":[{"ID":"r1","GameID":"g","ReporterID":"u","Reason":"spam","Status":"pending"}]}`,
		},
		{
			"invalid muted topic length",
			`{"follows":[],"blocks":[],"saves":[],"comments":[],"commentIdempotency":[],"notInterested":[],"mutedTopics":[{"ViewerID":"v","Topic":"` + strings.Repeat("x", 41) + `"}],"reactions":[],"reports":[]}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeBackupEnvelope(t, 3, tc.snapshot)
			if _, err := readPostgresBackup(path); err == nil {
				t.Fatalf("read v3 backup with %s: got no error, want validation error", tc.name)
			}
		})
	}
}

func TestReadPostgresBackupV3AcceptsAllCollections(t *testing.T) {
	path := writeBackupEnvelope(t, 3, `{"follows":[{"ViewerID":"v","CreatorID":"c"}],"blocks":[],"saves":[],"comments":[],"commentIdempotency":[],"notInterested":[{"ViewerID":"v","GameID":"g"}],"mutedTopics":[{"ViewerID":"v","Topic":"news"}],"reactions":[{"ViewerID":"v","GameID":"g","Reaction":"like"}],"reports":[{"ID":"r1","GameID":"g","ReporterID":"u","Reason":"spam","Status":"open"}]}`)
	snapshot, err := readPostgresBackup(path)
	if err != nil {
		t.Fatalf("read v3 backup: %v", err)
	}
	if len(snapshot.NotInterested) != 1 || len(snapshot.MutedTopics) != 1 || len(snapshot.Reactions) != 1 || len(snapshot.Reports) != 1 {
		t.Fatalf("v3 collections = %d/%d/%d/%d, want 1/1/1/1",
			len(snapshot.NotInterested), len(snapshot.MutedTopics), len(snapshot.Reactions), len(snapshot.Reports))
	}
}

func TestReadPostgresBackupV3TreatsMissingAuditAsEmpty(t *testing.T) {
	path := writeBackupEnvelope(t, 3, `{"follows":[],"blocks":[],"saves":[],"comments":[],"commentIdempotency":[],"notInterested":[],"mutedTopics":[],"reactions":[],"reports":[]}`)
	snapshot, err := readPostgresBackup(path)
	if err != nil {
		t.Fatalf("read v3 backup: %v", err)
	}
	if snapshot.Audit == nil {
		t.Fatalf("v3 backup audit = nil, want empty non-nil slice")
	}
}

func TestReadPostgresBackupV4RequiresAudit(t *testing.T) {
	path := writeBackupEnvelope(t, 4, `{"follows":[],"blocks":[],"saves":[],"comments":[],"commentIdempotency":[],"notInterested":[],"mutedTopics":[],"reactions":[],"reports":[]}`)
	if _, err := readPostgresBackup(path); err == nil || !strings.Contains(err.Error(), "missing required collections") {
		t.Fatalf("read v4 backup without audit error = %v, want missing required collections", err)
	}
}

func TestReadPostgresBackupV4AcceptsAudit(t *testing.T) {
	path := writeBackupEnvelope(t, 4, `{"follows":[],"blocks":[],"saves":[],"comments":[],"commentIdempotency":[],"notInterested":[],"mutedTopics":[],"reactions":[],"reports":[],"audit":[{"ID":"a1","Action":"game_quarantined","GameID":"g","ActorID":"moderation-token","Status":"quarantined"}]}`)
	snapshot, err := readPostgresBackup(path)
	if err != nil {
		t.Fatalf("read v4 backup: %v", err)
	}
	if len(snapshot.Audit) != 1 || snapshot.Audit[0].Action != "game_quarantined" {
		t.Fatalf("v4 audit = %+v, want one game_quarantined event", snapshot.Audit)
	}
}
