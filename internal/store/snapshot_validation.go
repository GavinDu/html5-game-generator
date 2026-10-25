package store

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// validateStoreSnapshot checks referential consistency and declared asset
// hashes before a restore can replace active data.
func validateStoreSnapshot(snapshot storeSnapshot) error {
	for id, ingest := range snapshot.Ingests {
		if ingest == nil || id == "" || ingest.IngestID != id {
			return fmt.Errorf("invalid ingest record %q", id)
		}
	}
	for id, game := range snapshot.Games {
		if game == nil || id == "" || game.ID != id {
			return fmt.Errorf("invalid game record %q", id)
		}
	}
	for id, job := range snapshot.Jobs {
		if job == nil || id == "" || job.ID != id {
			return fmt.Errorf("invalid job record %q", id)
		}
		if job.GameID != "" {
			if _, ok := snapshot.Games[job.GameID]; !ok {
				return fmt.Errorf("job %q references missing game %q", id, job.GameID)
			}
		}
	}
	for gameID, jobID := range snapshot.JobsByGame {
		job, ok := snapshot.Jobs[jobID]
		if gameID == "" || !ok || job.GameID != gameID {
			return fmt.Errorf("invalid job index for game %q", gameID)
		}
	}
	for id, group := range snapshot.Groups {
		if group == nil || id == "" || group.ID != id {
			return fmt.Errorf("invalid group record %q", id)
		}
	}
	for gameID, assets := range snapshot.Artifacts {
		if gameID == "" {
			return fmt.Errorf("artifact collection has empty game ID")
		}
		for name := range assets {
			if strings.TrimSpace(name) == "" {
				return fmt.Errorf("game %q has an artifact with an empty name", gameID)
			}
		}
	}
	for gameID, game := range snapshot.Games {
		if game.GameIR == nil {
			continue
		}
		for _, asset := range game.GameIR.Assets {
			if asset.SHA256 == "" {
				continue
			}
			artifact, ok := snapshot.Artifacts[gameID][asset.URI]
			if !ok {
				return fmt.Errorf("game %q asset %q is missing from the artifact snapshot", gameID, asset.ID)
			}
			sum := sha256.Sum256(artifact.Data)
			if !strings.EqualFold(asset.SHA256, hex.EncodeToString(sum[:])) {
				return fmt.Errorf("game %q asset %q SHA-256 mismatch", gameID, asset.ID)
			}
		}
	}
	return nil
}
