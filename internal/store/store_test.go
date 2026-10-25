package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"doodlequest/internal/gameir"
	"doodlequest/internal/model"
)

// A persistent store reloads games, jobs, groups and artifacts after a
// "restart" (a fresh MemoryStore on the same snapshot file). Before
// EnablePersistence existed, all of this lived only in RAM.
func TestPersistentStoreSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store-state.json")

	st := NewMemoryStore()
	if err := st.EnablePersistence(path); err != nil {
		t.Fatalf("enable: %v", err)
	}
	st.CreateGame(&model.Game{ID: "g1", Title: "Bear", OwnerID: "parent-1", Status: model.GameReady})
	st.CreateJob(&model.Job{ID: "j1", GameID: "g1", Status: model.JobDone})
	st.CreateGroup(&model.Group{ID: "grp", Name: "Fam", OwnerID: "parent-1"})
	st.PutArtifact("g1", "game-ir.json", []byte(`{"ok":true}`), "application/json")

	fresh := NewMemoryStore()
	if err := fresh.EnablePersistence(path); err != nil {
		t.Fatalf("reload: %v", err)
	}
	g, ok := fresh.GetGame("g1")
	if !ok || g.Title != "Bear" || g.Status != model.GameReady {
		t.Fatalf("game lost across restart: %+v ok=%v", g, ok)
	}
	j, ok := fresh.GetJobByGame("g1")
	if !ok || j.ID != "j1" {
		t.Fatalf("job lost across restart: %+v ok=%v", j, ok)
	}
	if _, ok := fresh.GetGroup("grp"); !ok {
		t.Fatal("group lost across restart")
	}
	data, ct, ok := fresh.GetArtifact("g1", "game-ir.json")
	if !ok || string(data) != `{"ok":true}` || ct != "application/json" {
		t.Fatalf("artifact lost across restart: %q %q ok=%v", data, ct, ok)
	}

	// Mutations after reload keep writing through.
	g.Title = "Bear II"
	fresh.UpdateGame(g)
	third := NewMemoryStore()
	if err := third.EnablePersistence(path); err != nil {
		t.Fatalf("second reload: %v", err)
	}
	g3, _ := third.GetGame("g1")
	if g3.Title != "Bear II" {
		t.Fatalf("post-reload mutation not persisted: %q", g3.Title)
	}
}

// A plain memory store writes nothing and reads nothing from disk.
func TestMemoryStoreStaysVolatile(t *testing.T) {
	st := NewMemoryStore()
	st.CreateGame(&model.Game{ID: "g1", Title: "Ephemeral"})
	if _, ok := st.GetGame("g1"); !ok {
		t.Fatal("memory store should hold its own writes")
	}
	other := NewMemoryStore()
	if _, ok := other.GetGame("g1"); ok {
		t.Fatal("separate memory stores must not share state")
	}
}
func TestPersistenceStatusReportsCorruptSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store-state.json")
	if err := os.WriteFile(path, []byte("{not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	st := NewMemoryStore()
	if err := st.EnablePersistence(path); err == nil {
		t.Fatal("expected corrupt snapshot to fail loading")
	}
	enabled, healthy := st.PersistenceStatus()
	if !enabled || healthy {
		t.Fatalf("persistence status = enabled:%v healthy:%v, want enabled and unhealthy", enabled, healthy)
	}
	st.CreateGame(&model.Game{ID: "g1", Title: "must not overwrite snapshot"})
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "{not-json" {
		t.Fatalf("failed load overwrote original snapshot: data=%q err=%v", data, err)
	}
}

func TestPersistenceStatusRecoversAfterSuccessfulWrite(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "state", "store.json")
	st := NewMemoryStore()
	if err := st.EnablePersistence(path); err != nil {
		t.Fatal(err)
	}
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(parent); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(parent, []byte("block"), 0o600); err != nil {
		t.Fatal(err)
	}
	st.CreateGame(&model.Game{ID: "g1", Title: "write failure"})
	if _, healthy := st.PersistenceStatus(); healthy {
		t.Fatal("expected persistence write failure to mark status unhealthy")
	}
	if err := os.Remove(parent); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	st.CreateGame(&model.Game{ID: "g1", Title: "write restored"})
	if _, healthy := st.PersistenceStatus(); !healthy {
		t.Fatal("successful write should restore persistence health")
	}
}

func TestBackupRestorePreservesGamesJobsGroupsAndArtifacts(t *testing.T) {
	root := t.TempDir()
	statePath := filepath.Join(root, "state", "store.json")
	backupPath := filepath.Join(root, "backups", "store.backup.json")
	source := NewMemoryStore()
	if err := source.EnablePersistence(statePath); err != nil {
		t.Fatal(err)
	}
	source.CreateGame(&model.Game{ID: "g-backup", Title: "Bobo", OwnerID: "parent-1", Status: model.GameReady})
	source.CreateJob(&model.Job{ID: "j-backup", GameID: "g-backup", Status: model.JobDone})
	source.CreateGroup(&model.Group{ID: "grp-backup", Name: "Family", OwnerID: "parent-1"})
	source.PutArtifact("g-backup", "assets/bear.png", []byte("image-bytes"), "image/png")
	if err := source.BackupTo(backupPath); err != nil {
		t.Fatalf("backup: %v", err)
	}

	restored := NewMemoryStore()
	if err := restored.EnablePersistence(statePath + ".restored"); err != nil {
		t.Fatal(err)
	}
	if err := restored.RestoreFrom(backupPath); err != nil {
		t.Fatalf("restore: %v", err)
	}
	game, ok := restored.GetGame("g-backup")
	if !ok || game.Title != "Bobo" {
		t.Fatalf("restored game = %+v, ok=%v", game, ok)
	}
	if job, ok := restored.GetJobByGame("g-backup"); !ok || job.ID != "j-backup" {
		t.Fatalf("restored job = %+v, ok=%v", job, ok)
	}
	if _, ok := restored.GetGroup("grp-backup"); !ok {
		t.Fatal("group missing after restore")
	}
	data, contentType, ok := restored.GetArtifact("g-backup", "assets/bear.png")
	if !ok || string(data) != "image-bytes" || contentType != "image/png" {
		t.Fatalf("restored artifact = %q %q, ok=%v", data, contentType, ok)
	}
	reloaded := NewMemoryStore()
	if err := reloaded.EnablePersistence(statePath + ".restored"); err != nil {
		t.Fatalf("reload restored state: %v", err)
	}
	if _, ok := reloaded.GetGame("g-backup"); !ok {
		t.Fatal("restored state was not written to the configured snapshot")
	}
}

func TestRestoreRejectsCorruptBackupWithoutMutatingStore(t *testing.T) {
	root := t.TempDir()
	backupPath := filepath.Join(root, "store.backup.json")
	source := NewMemoryStore()
	source.CreateGame(&model.Game{ID: "g-source", Title: "Source"})
	if err := source.BackupTo(backupPath); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)-2] ^= 1
	if err := os.WriteFile(backupPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	target := NewMemoryStore()
	target.CreateGame(&model.Game{ID: "g-existing", Title: "Keep me"})
	if err := target.RestoreFrom(backupPath); err == nil {
		t.Fatal("expected checksum/format error for corrupted backup")
	}
	if game, ok := target.GetGame("g-existing"); !ok || game.Title != "Keep me" {
		t.Fatalf("failed restore mutated target: %+v, ok=%v", game, ok)
	}
}

func TestRestoreRejectsSnapshotWithAssetHashMismatch(t *testing.T) {
	root := t.TempDir()
	backupPath := filepath.Join(root, "bad-asset.backup.json")
	targetPath := filepath.Join(root, "restored", "store.json")
	source := NewMemoryStore()
	game := &model.Game{
		ID:     "g-bad-asset",
		GameIR: &gameir.Document{Assets: []gameir.Asset{{ID: "asset-1", URI: "assets/asset-1.png", SHA256: strings.Repeat("0", 64)}}},
	}
	source.CreateGame(game)
	source.PutArtifact(game.ID, "assets/asset-1.png", []byte("actual-bytes"), "image/png")
	if err := source.BackupTo(backupPath); err != nil {
		t.Fatal(err)
	}

	target := NewMemoryStore()
	target.CreateGame(&model.Game{ID: "g-existing", Title: "Keep existing state"})
	if err := target.EnablePersistence(targetPath); err != nil {
		t.Fatal(err)
	}
	if err := target.RestoreFrom(backupPath); err == nil || !strings.Contains(err.Error(), "SHA-256 mismatch") {
		t.Fatalf("restore error = %v, want asset SHA-256 mismatch", err)
	}
	if got, ok := target.GetGame("g-existing"); !ok || got.Title != "Keep existing state" {
		t.Fatalf("invalid backup mutated target state: game=%+v ok=%v", got, ok)
	}
}
