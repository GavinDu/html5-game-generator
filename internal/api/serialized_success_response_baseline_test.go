package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

// Success response bytes are a different budget from inbound JSON,
// compressed image/audio assets, and decoded GPU texture allocation.
// These tests measure the ACTUAL shipping Go HTTP writers before anyone
// chooses a cross-platform WebKit/Chromium transfer ceiling.
// No arbitrary client cutoff or server policy is introduced here.
func TestCanonicalPublishedPackageSerializedSuccessBytes(t *testing.T) {
	st := store.NewMemoryStore()
	srv := New(st, pipeline.NewRunner(st))
	game := seedReadyGame(st, "serialized-response-fixture")
	game.ModerationStatus = "approved"
	st.UpdateGame(game)

	response := getAs(t, srv, "/v1/games/serialized-response-fixture/package", "")
	if response.Code != http.StatusOK {
		t.Fatalf("authentic published package HTTP %d: %s", response.Code, response.Body.String())
	}

	var pkg GamePackage
	if err := json.Unmarshal(response.Body.Bytes(), &pkg); err != nil {
		t.Fatalf("decode actual package body: %v", err)
	}
	if pkg.PackageFormatVersion != PackageFormatVersion || pkg.GameID != game.ID {
		t.Fatalf("unexpected package identity or version")
	}
	if pkg.IR == nil || len(pkg.IRSHA256) != 64 || pkg.IRSHA256 != irSHA256Of(pkg.IR) || pkg.IRHash != pkg.IRSHA256[:12] {
		t.Fatalf("package does not match Go canonical IR integrity contract")
	}
	encoded, err := json.Marshal(pkg)
	if err != nil {
		t.Fatalf("serialize returned GamePackage: %v", err)
	}
	if !bytes.Equal(response.Body.Bytes(), encoded) {
		t.Fatalf("actual Go package response must contain the exact serialized envelope bytes")
	}
	t.Logf("canonical Go GamePackage v2 actual success JSON bytes=%d (asset bodies transferred separately)", len(encoded))
}

// 2,000 is the existing Go *submission* trace-count upper bound. It is NOT
// itself a wire-byte budget and is NOT proof that old persisted replay rows
// obey the modern admission contract. This keeps an actionable measurement
// of output expansion across realistic accepted run-input cardinalities.
func TestCanonicalReplaySerializedSuccessBytesAtAdmittedInputCounts(t *testing.T) {
	srv := newRunsServer(t)
	const gameID = "sample-rainbow-bear"

	for _, inputCount := range []int{1, 256, 2000} {
		t.Run(fmt.Sprintf("inputs_%d", inputCount), func(t *testing.T) {
			inputs := make([]RunInput, inputCount)
			for i := range inputs {
				inputs[i] = RunInput{
					Tick: i,
					Type: "pointer",
					Detail: fmt.Sprintf("%d,%d", i%390, i%844),
				}
			}
			request, err := json.Marshal(struct {
				Name       string     `json:"name"`
				Score      int        `json:"score"`
				Won        bool       `json:"won"`
				DurationMs int        `json:"durationMs"`
				Seed       uint32     `json:"seed"`
				Inputs     []RunInput `json:"inputs"`
			}{
				Name: "wire-size-baseline", Score: 0, Won: false,
				DurationMs: 120000, Seed: 42, Inputs: inputs,
			})
			if err != nil {
				t.Fatalf("encode admitted fixture request: %v", err)
			}
			status, receipt := postRun(t, srv, gameID, string(request))
			if status != http.StatusCreated {
				t.Fatalf("actual published run submission HTTP %d: %v", status, receipt)
			}
			runID, ok := receipt["runId"].(string)
			if !ok || runID == "" {
				t.Fatal("actual server did not mint a run ID")
			}

			response := httptest.NewRecorder()
			srv.Routes().ServeHTTP(
				response,
				httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID, nil),
			)
			if response.Code != http.StatusOK {
				t.Fatalf("authentic replay HTTP %d: %s", response.Code, response.Body.String())
			}
			var actual struct {
				ID           string     `json:"id"`
				Inputs       []RunInput `json:"inputs"`
				RankingScope string     `json:"rankingScope"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &actual); err != nil {
				t.Fatalf("decode actual replay: %v", err)
			}
			if len(actual.Inputs) != inputCount || actual.RankingScope != "provisional" {
				t.Fatalf("replay content/count/authority mismatch: inputs=%d scope=%q",
					len(actual.Inputs), actual.RankingScope)
			}
			t.Logf(
				"canonical Go replay accepted inputs=%d inbound JSON bytes=%d outbound success JSON bytes=%d",
				inputCount, len(request), response.Body.Len(),
			)
		})
	}
}
