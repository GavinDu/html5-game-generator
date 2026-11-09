// Package app wires and runs the DoodleQuest server.
package app

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"doodlequest/internal/api"
	"doodlequest/internal/inventory"
	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

// Run starts the HTTP server on $PORT (default 8080).
// openPersistentStore is deliberately all-or-nothing: if a snapshot is
// unreadable or corrupt, serving with a fresh in-memory state would let
// creators publish into a state that will disappear on restart. Operators
// must restore the snapshot (or explicitly relocate it after backing it up)
// before accepting writes again.
func openPersistentStore(path string) (*store.MemoryStore, error) {
	st := store.NewMemoryStore()
	if err := st.EnablePersistence(path); err != nil {
		return nil, fmt.Errorf("load durable game store: %w", err)
	}
	return st, nil
}

func Run() {
	// Inventory loads after the durable snapshot and re-seeds authoritatively.
	// Never silently replace an unreadable/corrupt creator store with empty
	// memory; /healthz alone cannot prove creations remain durable.
	storePath := os.Getenv("DQ_STORE_FILE")
	if storePath == "" {
		storePath = filepath.Join("telemetry", "store-state.json")
	}
	st, err := openPersistentStore(storePath)
	if err != nil {
		log.Fatalf("refusing to serve without durable game store: %v", err)
	}
	invDir := os.Getenv("INVENTORY_DIR")
	if invDir == "" {
		invDir = "inventory"
	}
	if err := inventory.Load(st, invDir); err != nil {
		log.Fatalf("inventory: %v", err)
	}
	runner := pipeline.NewRunner(st)
	srv := api.New(st, runner)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port
	log.Printf("doodlequest listening on %s", addr)
	if err := http.ListenAndServe(addr, srv.Routes()); err != nil {
		log.Fatal(err)
	}
}
