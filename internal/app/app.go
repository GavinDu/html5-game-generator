// Package app wires and runs the DoodleQuest server.
package app

import (
	"log"
	"net/http"
	"os"

	"doodlequest/internal/api"
	"doodlequest/internal/inventory"
	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

// Run starts the HTTP server on $PORT (default 8080).
func Run() {
	st := store.NewMemoryStore()
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
