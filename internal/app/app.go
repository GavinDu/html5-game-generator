// Package app wires and runs the DoodleQuest server.
package app

import (
	"log"
	"net/http"
	"os"

	"doodlequest/internal/api"
	"doodlequest/internal/pipeline"
	"doodlequest/internal/sample"
	"doodlequest/internal/store"
)

// Run starts the HTTP server on $PORT (default 8080).
func Run() {
	st := store.NewMemoryStore()
	sample.Seed(st) // built-in sample game: "Bobo's Rainbow"
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
