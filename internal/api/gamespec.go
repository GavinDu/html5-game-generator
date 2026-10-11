package api

import (
	"net/http"

	"doodlequest/internal/gamespec"
)

// POST /v1/spec/decide — route a GameSpec to an engine.
// Body: GameSpec JSON. Returns {engine, rationale, scores}.
func (s *Server) handleSpecDecide(w http.ResponseWriter, r *http.Request) {
	var spec gamespec.GameSpec
	if !decodeJSON(w, r, &spec) {
		return
	}
	spec.ApplyDefaults(gamespec.DefaultPrefs())
	if err := spec.Validate(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, gamespec.Decide(spec))
}

// GET /v1/spec/questionnaire — the factor metadata behind the detailed
// questionnaire: group, prompt, type, options, defaults, and which factors
// are standing user preferences.
func (s *Server) handleSpecQuestionnaire(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"specVersion": gamespec.SpecVersion,
		"factors":     gamespec.Questionnaire(),
	})
}
