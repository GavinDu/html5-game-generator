// Command validate runs the step-4 Game IR validation gate over one game
// directory and a design-input JSON file, printing the ValidationReport.
//
// Usage:
//
//	go run ./cmd/validate --game inventory/13ui1k \
//	    --design internal/validator/testdata/bobos-rainbow-design-input.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"doodlequest/internal/gamespec"
	"doodlequest/internal/validator"
)

func main() {
	gameDir := flag.String("game", "", "game directory (e.g. inventory/13ui1k)")
	designPath := flag.String("design", "", "GameDesignInput v1 JSON file")
	attempt := flag.Int("attempt", 0, "repair attempt number (0-based)")
	flag.Parse()
	if *gameDir == "" || *designPath == "" {
		flag.Usage()
		os.Exit(2)
	}
	di, err := gamespec.LoadRequiredInputs(*designPath)
	if err != nil {
		fatal(err)
	}
	report, err := validator.Validate(validator.ValidateInput{
		GameID:      filepath.Base(*gameDir),
		GameDir:     *gameDir,
		DesignInput: di,
		Attempt:     *attempt,
		MaxAttempts: validator.DefaultMaxAttempts,
	})
	if err != nil {
		fatal(err)
	}
	out, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fatal(err)
	}
	fmt.Println(string(out))
	fmt.Printf("\n%s: next=%s\n", report.Summary, report.Repair.NextAction)
	if report.Verdict == validator.VerdictFail {
		os.Exit(1)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "validate:", err)
	os.Exit(2)
}
