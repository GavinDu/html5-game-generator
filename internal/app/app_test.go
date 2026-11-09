package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"doodlequest/internal/model"
)

func TestPersistentStartupRestoresSavedCreatorGames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store", "state.json")
	original, err := openPersistentStore(path)
	if err != nil {
		t.Fatalf("fresh persistence setup: %v", err)
	}
	original.CreateGame(&model.Game{ID: "creator-game", Title: "Preserved creation"})

	reopened, err := openPersistentStore(path)
	if err != nil {
		t.Fatalf("restore saved snapshot: %v", err)
	}
	got, exists := reopened.GetGame("creator-game")
	if !exists || got.Title != "Preserved creation" {
		t.Fatalf("saved creator game lost during restart: %+v, exists=%v", got, exists)
	}
	enabled, healthy := reopened.PersistenceStatus()
	if !enabled || !healthy {
		t.Fatalf("restored store must have healthy durable persistence: enabled=%v healthy=%v", enabled, healthy)
	}
}

func TestPersistentStartupRefusesCorruptOrUnreadableSnapshot(t *testing.T) {
	for _, tc := range []struct {
		name     string
		setup    func(string) error
		wantHint string
	}{
		{
			name: "corrupt JSON",
			setup: func(path string) error {
				return os.WriteFile(path, []byte(`{"games":`), 0600)
			},
			wantHint: "load durable game store",
		},
		{
			name: "snapshot path is directory",
			setup: func(path string) error {
				return os.Mkdir(path, 0700)
			},
			wantHint: "load durable game store",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			if err := tc.setup(path); err != nil {
				t.Fatal(err)
			}
			st, err := openPersistentStore(path)
			if err == nil || !strings.Contains(err.Error(), tc.wantHint) {
				t.Fatalf("bad snapshot must prevent server startup, got store=%v err=%v", st, err)
			}
			if st != nil {
				t.Fatal("must not expose a fallback memory store after snapshot load failure")
			}
		})
	}
}
