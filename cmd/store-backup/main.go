// Command store-backup creates or restores a checksummed snapshot of the
// file-backed game store. Run it with the same state path configured for the
// service; stop writes to the service during restore.
package main

import (
	"flag"
	"fmt"
	"os"

	"doodlequest/internal/store"
)

func main() {
	mode := flag.String("mode", "", "operation: backup or restore")
	statePath := flag.String("state", "", "live store snapshot path")
	backupPath := flag.String("backup", "", "backup file path")
	flag.Parse()
	if *statePath == "" || *backupPath == "" || *statePath == *backupPath {
		fail("both -state and -backup are required and must differ")
	}
	st := store.NewMemoryStore()
	if err := st.EnablePersistence(*statePath); err != nil && *mode != "restore" {
		fail("load store: %v", err)
	}
	switch *mode {
	case "backup":
		if err := st.BackupTo(*backupPath); err != nil {
			fail("backup failed: %v", err)
		}
		fmt.Println("backup created and checksummed")
	case "restore":
		if err := st.RestoreFrom(*backupPath); err != nil {
			fail("restore failed: %v", err)
		}
		fmt.Println("backup validated and restored atomically")
	default:
		fail("-mode must be backup or restore")
	}
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
