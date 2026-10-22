// Command playtest is the Phase-1 publish gate of the fused detect-and-fix
// flow: it runs the browser playtest worker against a live server and,
// when given a static ValidationReport (-merge), folds the measurements
// into it (validator.MergePlaytest) so harness-gated checks carry real
// results instead of skips.
//
// Usage:
//
//	go run ./cmd/playtest -base http://localhost:8080 [-games a,b]
//	    [-browser chromium|webkit] [-worker tools/playtest/probe.mjs]
//	    [-merge report.json] [-out merged.json]
//
// Exit code is 1 when any game fails the gate checks or a merged report's
// verdict is fail, 2 on usage/worker errors.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"doodlequest/internal/playtest"
	"doodlequest/internal/validator"
)

func main() {
	base := flag.String("base", "", "base URL of the running server (required)")
	games := flag.String("games", "", "comma-separated game ids (default: whole feed)")
	browser := flag.String("browser", "chromium", "chromium | webkit")
	worker := flag.String("worker", "tools/playtest/probe.mjs", "playtest worker script")
	merge := flag.String("merge", "", "path to a static ValidationReport JSON to merge results into")
	out := flag.String("out", "", "write the (merged) report here instead of stdout")
	shots := flag.String("shots", "", "directory for worker screenshots + its raw report")
	timeout := flag.Duration("timeout", 12*time.Minute, "overall worker timeout")
	flag.Parse()
	if *base == "" {
		fmt.Fprintln(os.Stderr, "playtest: -base is required")
		os.Exit(2)
	}
	var ids []string
	if *games != "" {
		ids = strings.Split(*games, ",")
	}
	report, err := playtest.Run(context.Background(), playtest.Config{
		WorkerCmd: []string{"node", *worker},
		BaseURL:   *base,
		Browser:   *browser,
		ShotsDir:  *shots,
		Timeout:   *timeout,
	}, ids)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	exit := 0
	if report.Failed() {
		exit = 1
	}
	var payload any = report
	if *merge != "" {
		raw, err := os.ReadFile(*merge)
		if err != nil {
			fmt.Fprintln(os.Stderr, "playtest: read -merge:", err)
			os.Exit(2)
		}
		var vr validator.ValidationReport
		if err := json.Unmarshal(raw, &vr); err != nil {
			fmt.Fprintln(os.Stderr, "playtest: parse -merge:", err)
			os.Exit(2)
		}
		outcome, ok := report.OutcomeFor(vr.Subject.GameID)
		if !ok {
			fmt.Fprintf(os.Stderr, "playtest: worker did not cover game %s; report left unmerged\n", vr.Subject.GameID)
			os.Exit(2)
		}
		if !validator.MergePlaytest(&vr, outcome) {
			fmt.Fprintln(os.Stderr, "playtest: merge produced no changes")
			os.Exit(2)
		}
		if vr.Verdict == validator.VerdictFail {
			exit = 1
		}
		payload = &vr
	}
	enc, _ := json.MarshalIndent(payload, "", "  ")
	if *out != "" {
		if err := os.WriteFile(*out, enc, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "playtest: write -out:", err)
			os.Exit(2)
		}
	} else {
		fmt.Println(string(enc))
	}
	os.Exit(exit)
}
