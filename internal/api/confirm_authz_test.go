package api

import (
	"net/http"
	"testing"

	"doodlequest/internal/model"
)

// Confirm is an owner-authorized state change, not a public operation like
// reading a published game's package. Knowing a private game ID or holding
// another valid account session must never allow advancing that author's job.
func TestConfirmRejectsCrossUserWithoutAdvancingJob(t *testing.T) {
	srv := newTestServer()
	ingest := do(t, srv, http.MethodPost, "/v1/ingest",
		map[string]any{"keywords": []string{"penguin", "heart"}}, "original-creator")
	if ingest.Code != http.StatusOK {
		t.Fatalf("ingest: %d %s", ingest.Code, ingest.Body.String())
	}
	var source model.IngestResult
	decode(t, ingest, &source)
	created := do(t, srv, http.MethodPost, "/v1/games",
		map[string]any{"ingest_id": source.IngestID}, "original-creator")
	if created.Code != http.StatusAccepted {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	var response struct {
		Game model.Game `json:"game"`
		Job model.Job `json:"job"`
	}
	decode(t, created, &response)
	waitJob(t, srv, response.Job.ID, model.JobAwaitingConfirm)

	for _, unauthorized := range []string{"different-creator", ""} {
		denied := do(t, srv, http.MethodPost,
			"/v1/games/"+response.Game.ID+"/confirm", map[string]any{}, unauthorized)
		if denied.Code != http.StatusForbidden && denied.Code != http.StatusUnauthorized {
			t.Fatalf("%q confirmed another user's game: %d %s", unauthorized, denied.Code, denied.Body.String())
		}
		job := waitJob(t, srv, response.Job.ID, model.JobAwaitingConfirm)
		if job.Status != model.JobAwaitingConfirm {
			t.Fatalf("unauthorized confirmation changed the job to %s", job.Status)
		}
	}
	absent := do(t, srv, http.MethodPost, "/v1/games/nonexistent/confirm",
		map[string]any{}, "original-creator")
	if absent.Code != http.StatusNotFound {
		t.Fatalf("missing game must return 404, got %d", absent.Code)
	}
	approved := do(t, srv, http.MethodPost, "/v1/games/"+response.Game.ID+"/confirm",
		map[string]any{}, "original-creator")
	if approved.Code != http.StatusAccepted {
		t.Fatalf("author could not confirm own design: %d %s", approved.Code, approved.Body.String())
	}
	if job := waitJob(t, srv, response.Job.ID, model.JobDone); job.Status != model.JobDone {
		t.Fatalf("author's job did not complete: %s", job.Status)
	}
}
