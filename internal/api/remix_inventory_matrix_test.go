package api

import (
	"net/http"
	"path/filepath"
	"reflect"
	"testing"

	"doodlequest/internal/gameir"
	"doodlequest/internal/inventory"
	"doodlequest/internal/model"
	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

// Every current source-art/questionnaire game must support the SAME
// canonical IR preserving remix path, not a Bobo-only compiler. Test through
// HTTP create/confirm/package with a populated in-memory asset store, then
// repeat one more generation to catch lineage/asset propagation defects.
func TestEveryShippedGameSupportsTwoGenerationsOfCanonicalRemixes(t *testing.T) {
	for _, sourceID := range []string{
		"sample-rainbow-bear",
		"sample-ribbon-sky",
		"penguin-heartlight-rescue",
		"escape-from-injection",
	} {
		t.Run(sourceID, func(t *testing.T) {
			st := store.NewMemoryStore()
			if err := inventory.Load(st, filepath.Join("..", "..", "inventory")); err != nil {
				t.Fatal(err)
			}
			srv := New(st, pipeline.NewRunner(st))
			parent, ok := st.GetGame(sourceID)
			if !ok || parent.Manifest == nil || parent.GameIR == nil {
				t.Fatalf("%s must be a playable canonical inventory parent", sourceID)
			}
			originalIR := parent.GameIR
			current := parent
			for generation := 1; generation <= 2; generation++ {
				created := do(t, srv, http.MethodPost, "/v1/games", map[string]any{
					"parent_game_id": current.ID,
					"intent":         "Remix the authored story without replacing its mechanics",
				}, "remix-matrix-author")
				if created.Code != http.StatusAccepted {
					t.Fatalf("generation %d create status %d: %s", generation, created.Code, created.Body.String())
				}
				var response struct {
					Game model.Game `json:"game"`
					Job  model.Job  `json:"job"`
				}
				decode(t, created, &response)
				if response.Game.ParentGameID != current.ID {
					t.Fatalf("generation %d lost immediate parent %s", generation, current.ID)
				}
				job := waitJob(t, srv, response.Job.ID, model.JobAwaitingConfirm)
				if job.DesignDoc == nil {
					t.Fatalf("generation %d produced no questionnaire", generation)
				}
				if current.GameIR.Experience != nil {
					if job.DesignDoc.WinCondition != current.GameIR.Experience.Objective.Label ||
						len(job.DesignDoc.Controls) != 1 ||
						job.DesignDoc.Controls[0] != current.GameIR.Experience.Controls.Scheme {
						t.Fatalf("generation %d questionnaire changed parent's objective or controls: %+v", generation, job.DesignDoc)
					}
				}
				confirm := do(t, srv, http.MethodPost, "/v1/games/"+response.Game.ID+"/confirm", map[string]any{}, "remix-matrix-author")
				if confirm.Code != http.StatusAccepted {
					t.Fatalf("generation %d confirm status %d: %s", generation, confirm.Code, confirm.Body.String())
				}
				job = waitJob(t, srv, response.Job.ID, model.JobDone)
				if job.Status != model.JobDone {
					t.Fatalf("generation %d did not complete: %+v", generation, job)
				}
				child, ok := st.GetGame(response.Game.ID)
				if !ok || child.GameIR == nil || child.Manifest == nil {
					t.Fatalf("generation %d did not publish a playable Game IR", generation)
				}
				if err := gameir.Validate(child.GameIR); err != nil {
					t.Fatalf("generation %d published invalid IR: %v", generation, err)
				}
				if child.GameIR.Metadata.ParentGameID != current.ID ||
					child.GameIR.Metadata.ParentVersion != current.Manifest.Version {
					t.Fatalf("generation %d lineage wrong: %+v", generation, child.GameIR.Metadata)
				}
				if !reflect.DeepEqual(child.GameIR.Scenes, current.GameIR.Scenes) ||
					!reflect.DeepEqual(child.GameIR.Assets, current.GameIR.Assets) ||
					!reflect.DeepEqual(child.GameIR.InputBindings, current.GameIR.InputBindings) ||
					!reflect.DeepEqual(child.GameIR.Rules, current.GameIR.Rules) ||
					!reflect.DeepEqual(child.GameIR.Experience, current.GameIR.Experience) {
					t.Fatalf("generation %d changed canonical gameplay contract of %s", generation, current.ID)
				}
				pkgRec := do(t, srv, http.MethodGet, "/v1/games/"+child.ID+"/package", nil, "remix-matrix-author")
				if pkgRec.Code != http.StatusOK {
					t.Fatalf("generation %d package status %d: %s", generation, pkgRec.Code, pkgRec.Body.String())
				}
				var pkg GamePackage
				decode(t, pkgRec, &pkg)
				if pkg.GameID != child.ID || pkg.IR.Metadata.ParentGameID != current.ID ||
					len(pkg.Assets) != len(current.GameIR.Assets) {
					t.Fatalf("generation %d package lost lineage or assets", generation)
				}
				for _, asset := range pkg.Assets {
					assetRec := do(t, srv, http.MethodGet, asset.URL, nil, "remix-matrix-author")
					if assetRec.Code != http.StatusOK || assetRec.Body.Len() == 0 {
						t.Fatalf("generation %d cannot serve asset %q: %d", generation, asset.ID, assetRec.Code)
					}
				}
				unchanged, ok := st.GetGame(current.ID)
				if !ok || unchanged.GameIR.Metadata.ParentGameID != current.GameIR.Metadata.ParentGameID ||
					!reflect.DeepEqual(unchanged.GameIR.Rules, current.GameIR.Rules) {
					t.Fatalf("generation %d mutated the immediate parent", generation)
				}
				current = child
			}
			originalAgain, ok := st.GetGame(sourceID)
			if !ok || !reflect.DeepEqual(originalAgain.GameIR, originalIR) {
				t.Fatal("second-generation remix mutated the original source game")
			}
		})
	}
}
