package store

import (
	"path/filepath"
	"testing"

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
