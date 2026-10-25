// migrate-social-graph imports local follow and block relations into the configured
// shared PostgreSQL social graph. It is resumable and never deletes destination rows.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"doodlequest/internal/socialgraph"
	_ "github.com/lib/pq"
)

func main() {
	dsn := strings.TrimSpace(os.Getenv("DQ_SOCIAL_DATABASE_URL"))
	if dsn == "" {
		fatal("DQ_SOCIAL_DATABASE_URL must be configured")
	}
	path := strings.TrimSpace(os.Getenv("DQ_SOCIAL_STATE_FILE"))
	if path == "" {
		path = filepath.Join("telemetry", "social-state.json")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fatal("read source snapshot: " + err.Error())
	}
	var snapshot struct {
		Follows map[string]map[string]struct{} `json:"follows"`
		Blocks  map[string]map[string]bool     `json:"blocks"`
		Saves   map[string]map[string]bool     `json:"saves"`
	}
	if err := json.Unmarshal(data, &snapshot); err != nil {
		fatal("decode source snapshot: " + err.Error())
	}
	edges := make([]socialgraph.Edge, 0)
	for viewer, creators := range snapshot.Follows {
		for creator := range creators {
			edges = append(edges, socialgraph.Edge{ViewerID: viewer, CreatorID: creator})
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := socialgraph.OpenPostgres(ctx, dsn)
	if err != nil {
		fatal("open shared social graph: " + err.Error())
	}
	defer db.Close()
	inserted, err := db.ImportFollows(ctx, edges)
	if err != nil {
		fatal("import follow graph: " + err.Error())
	}
	fmt.Printf("source_edges=%d inserted_edges=%d already_present=%d\n", len(edges), inserted, len(edges)-inserted)
	blocks := make([]socialgraph.BlockRelation, 0)
	for viewer, targets := range snapshot.Blocks {
		for blocked, enabled := range targets {
			if enabled {
				blocks = append(blocks, socialgraph.BlockRelation{ViewerID: viewer, BlockedID: blocked})
			}
		}
	}
	insertedBlocks, err := db.ImportBlocks(ctx, blocks)
	if err != nil {
		fatal("import block relations: " + err.Error())
	}
	fmt.Printf("source_blocks=%d inserted_blocks=%d already_present=%d\n", len(blocks), insertedBlocks, len(blocks)-insertedBlocks)
	saves := make([]socialgraph.SaveRelation, 0)
	for viewer, games := range snapshot.Saves {
		for gameID, enabled := range games {
			if enabled {
				saves = append(saves, socialgraph.SaveRelation{ViewerID: viewer, GameID: gameID})
			}
		}
	}
	insertedSaves, err := db.ImportSaves(ctx, saves)
	if err != nil {
		fatal("import saved games: " + err.Error())
	}
	fmt.Printf("source_saves=%d inserted_saves=%d already_present=%d\n", len(saves), insertedSaves, len(saves)-insertedSaves)
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
