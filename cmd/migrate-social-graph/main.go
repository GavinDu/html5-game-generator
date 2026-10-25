// migrate-social-graph imports local follow edges into the configured shared
// PostgreSQL social graph. It is resumable and never deletes destination rows.
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
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
