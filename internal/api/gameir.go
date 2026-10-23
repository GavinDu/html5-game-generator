package api

import (
	"errors"
	"io"
	"net/http"

	"doodlequest/internal/gameir"
)

const maxGameIRDocumentBytes = 2 << 20

// POST /v1/game-ir/validate validates one canonical Game IR document.
// It is separate from /v1/ir/validate, which validates creator-facing design input.
func (s *Server) handleGameIRValidate(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxGameIRDocumentBytes)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeErr(w, http.StatusRequestEntityTooLarge, "Game IR document exceeds 2 MiB")
			return
		}
		writeErr(w, http.StatusBadRequest, "could not read Game IR document")
		return
	}
	doc, err := gameir.DecodeAndValidate(data)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"valid": true, "irVersion": doc.IRVersion, "gameId": doc.Game.ID,
		"sceneCount": len(doc.Scenes), "assetCount": len(doc.Assets), "ruleCount": len(doc.Rules),
	})
}

func (s *Server) handleGetGameIR(w http.ResponseWriter, r *http.Request) {
	game, ok := s.deliverableGame(w, r)
	if !ok {
		return
	}
	if game.GameIR == nil {
		writeErr(w, http.StatusConflict, "Game IR has not been published")
		return
	}
	writeJSON(w, http.StatusOK, game.GameIR)
}
