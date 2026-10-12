package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"

	"doodlequest/internal/gamespec"
	"doodlequest/internal/validator"
)

// inventoryDirName guards the gameId path join: plain directory names only,
// no separators or traversal.
var inventoryDirName = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// POST /v1/ir/validate — run the step-4 validation gate over one game.
// Body: {gameId, designInput, attempt?}. Returns a ValidationReport.
// gameId is an inventory directory name (e.g. "13ui1k").
func (s *Server) handleIRValidate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		GameID      string          `json:"gameId"`
		DesignInput json.RawMessage `json:"designInput"`
		Attempt     int             `json:"attempt"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.GameID == "" || len(req.DesignInput) == 0 {
		writeErr(w, http.StatusBadRequest, "gameId and designInput are required")
		return
	}
	if !inventoryDirName.MatchString(req.GameID) {
		writeErr(w, http.StatusBadRequest, "gameId must be a plain inventory directory name")
		return
	}
	var di gamespec.RequiredInputs
	if err := json.Unmarshal(req.DesignInput, &di); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid designInput: "+err.Error())
		return
	}
	invDir := os.Getenv("INVENTORY_DIR")
	if invDir == "" {
		invDir = "inventory"
	}
	report, err := validator.Validate(validator.ValidateInput{
		GameID:      req.GameID,
		GameDir:     filepath.Join(invDir, req.GameID),
		DesignInput: &di,
		Attempt:     req.Attempt,
		MaxAttempts: validator.DefaultMaxAttempts,
	})
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, report)
}
