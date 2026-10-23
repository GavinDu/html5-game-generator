package api

import (
	"net/http"
	"time"

	"doodlequest/internal/gameir"
	"doodlequest/internal/repair"
)

// Shadow-mode repair (fused detect-and-fix flow, Phase 3). The repair
// loop may PROPOSE a fixed IR with a full diff; nothing about the live
// game changes until a human calls apply. Proposals live in memory —
// they are suggestions, not state worth persisting — and an applied
// proposal swaps game.GameIR, which is exactly what the package
// endpoint serves, so apply IS the publish of the repaired version.
// The repaired document must pass gameir.Validate or apply refuses.

type repairProposal struct {
	GameID          string           `json:"game_id"`
	CreatedAt       time.Time        `json:"created_at"`
	Status          string           `json:"status"` // proposed | applied | discarded
	Changes         []repair.Change  `json:"changes"`
	Unfixable       []string         `json:"unfixable,omitempty"`
	Attempts        int              `json:"attempts"`
	Remaining       []string         `json:"remaining_failures,omitempty"`
	ValidationError string           `json:"validation_error,omitempty"`
	LLMTier         string           `json:"llm_tier"`
	RepairedIR      *gameir.Document `json:"repaired_ir,omitempty"`
}

func (s *Server) getProposal(gameID string) *repairProposal {
	s.repairMu.Lock()
	defer s.repairMu.Unlock()
	return s.repairProposals[gameID]
}

// POST /v1/games/{id}/repair/propose — run the bounded deterministic
// repair loop over the game's current IR and store the shadow proposal.
func (s *Server) handleRepairPropose(w http.ResponseWriter, r *http.Request) {
	game, ok := s.store.GetGame(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "game not found")
		return
	}
	if game.PublishedAt != nil {
		writeErr(w, http.StatusConflict, "published posts are immutable; create a remix or new draft before repairing")
		return
	}
	if game.GameIR == nil {
		writeErr(w, http.StatusConflict, "game has no IR to repair")
		return
	}
	gate := func(doc *gameir.Document) []string {
		if err := gameir.Validate(doc); err != nil {
			return []string{"ir.schema_valid"}
		}
		return nil
	}
	loop := repair.RunLoop(game.GameIR, nil, gate)
	prop := &repairProposal{
		GameID:    game.ID,
		CreatedAt: time.Now().UTC(),
		Status:    "proposed",
		Changes:   loop.Result.Changes,
		Unfixable: loop.Result.Unfixable,
		Attempts:  loop.Attempts,
		Remaining: loop.Remaining,
		// No LLM repairer is configured in this build: the tier is
		// reported as skipped, never simulated.
		LLMTier:    "skipped: " + repair.ErrLLMUnavailable.Error(),
		RepairedIR: loop.Result.Doc,
	}
	if err := gameir.Validate(loop.Result.Doc); err != nil {
		prop.ValidationError = err.Error()
	}
	s.repairMu.Lock()
	if s.repairProposals == nil {
		s.repairProposals = map[string]*repairProposal{}
	}
	s.repairProposals[game.ID] = prop
	s.repairMu.Unlock()
	writeJSON(w, http.StatusOK, prop)
}

// GET /v1/games/{id}/repair/proposal — the current shadow proposal.
func (s *Server) handleRepairProposal(w http.ResponseWriter, r *http.Request) {
	prop := s.getProposal(r.PathValue("id"))
	if prop == nil {
		writeErr(w, http.StatusNotFound, "no repair proposal for this game")
		return
	}
	writeJSON(w, http.StatusOK, prop)
}

// POST /v1/games/{id}/repair/apply — the human approval step. Publishes
// the proposed IR as the game's current version. Refuses when there is
// no live proposal or the proposed IR does not validate.
func (s *Server) handleRepairApply(w http.ResponseWriter, r *http.Request) {
	if !requireModerationToken(w, r) {
		return
	}
	game, ok := s.store.GetGame(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "game not found")
		return
	}
	if game.PublishedAt != nil {
		writeErr(w, http.StatusConflict, "published posts are immutable; create a remix or new draft before repairing")
		return
	}
	prop := s.getProposal(game.ID)
	if prop == nil || prop.Status != "proposed" {
		writeErr(w, http.StatusConflict, "no live repair proposal for this game")
		return
	}
	if prop.ValidationError != "" {
		writeErr(w, http.StatusConflict, "proposed IR does not validate: "+prop.ValidationError)
		return
	}
	if err := gameir.Validate(prop.RepairedIR); err != nil {
		writeErr(w, http.StatusConflict, "proposed IR does not validate: "+err.Error())
		return
	}
	game.GameIR = prop.RepairedIR
	game.UpdatedAt = time.Now()
	s.store.UpdateGame(game)
	s.repairMu.Lock()
	prop.Status = "applied"
	s.repairMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"game_id": game.ID, "applied": true, "changes": len(prop.Changes),
	})
}

// POST /v1/games/{id}/repair/discard — drop a shadow proposal unapplied.
func (s *Server) handleRepairDiscard(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	prop := s.getProposal(id)
	if prop == nil || prop.Status != "proposed" {
		writeErr(w, http.StatusConflict, "no live repair proposal for this game")
		return
	}
	s.repairMu.Lock()
	prop.Status = "discarded"
	prop.RepairedIR = nil
	s.repairMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"game_id": id, "discarded": true})
}
