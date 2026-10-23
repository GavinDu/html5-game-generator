// Package app wires and runs the DoodleQuest server.
package app

import (
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
func Run() {
	st := store.NewMemoryStore()
	// Durable store snapshot (games, jobs, groups, ingests, artifacts):
	// without it a restart lost every user-generated game while the
	// social state and run records on disk survived (architecture
	// review 2026-10-23). Inventory loads after the snapshot and
	// re-seeds authoritatively (see inventory.seedGame), so shipped
	// content on disk wins over a stale snapshot copy.
	storePath := os.Getenv("DQ_STORE_FILE")
	if storePath == "" {
		storePath = filepath.Join("telemetry", "store-state.json")
	}
	if err := st.EnablePersistence(storePath); err != nil {
		log.Printf("store persistence unavailable (%v); continuing in-memory", err)
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
