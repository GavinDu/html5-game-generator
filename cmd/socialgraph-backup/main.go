// Command socialgraph-backup exports or restores the shared PostgreSQL social graph.
// Restore is additive and should target an empty database for an exact recovery.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"doodlequest/internal/socialgraph"
)

func main() {
	mode := flag.String("mode", "", "operation: backup or restore")
	backupPath := flag.String("backup", "", "backup file path")
	dsn := flag.String("database-url", os.Getenv("DQ_SOCIAL_DATABASE_URL"), "PostgreSQL URL (defaults to DQ_SOCIAL_DATABASE_URL)")
	flag.Parse()
	if *dsn == "" || *backupPath == "" {
		fail("-database-url (or DQ_SOCIAL_DATABASE_URL) and -backup are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	graph, err := socialgraph.OpenPostgres(ctx, *dsn)
	if err != nil {
		fail("open social graph: %v", err)
	}
	defer graph.Close()
	switch *mode {
	case "backup":
		if err := graph.BackupTo(ctx, *backupPath); err != nil {
			fail("backup failed: %v", err)
		}
		fmt.Println("social graph backup created and checksummed")
	case "restore":
		if err := graph.RestoreFrom(ctx, *backupPath); err != nil {
			fail("restore failed: %v", err)
		}
		fmt.Println("social graph backup validated and restored additively")
	default:
		fail("-mode must be backup or restore")
	}
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
