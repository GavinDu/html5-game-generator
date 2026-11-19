package api

import (
	"net/http"
	"runtime/debug"

	"doodlequest/internal/gameir"
)

// runtimeReleaseProvenance is read-only operational evidence, NOT a signed
// release attestation. Unknown is intentional when VCS stamping is absent,
// malformed, duplicated or indicates a modified build.
type runtimeReleaseProvenance struct {
	SourceRevision       string `json:"sourceRevision"`
	SourceRevisionStatus string `json:"sourceRevisionStatus"`
	PackageFormatVersion string `json:"packageFormatVersion"`
	MinRuntimeVersion    string `json:"minRuntimeVersion"`
	IRVersion            string `json:"irVersion"`
}

const unknownSourceRevision = "unknown"

// cleanGoVCSRevision accepts only Go's own clean-build VCS stamp. Neither an
// HTTP request nor an environment variable can supply a claimed revision.
func cleanGoVCSRevision(settings []debug.BuildSetting) (string, bool) {
	var revision, modified string
	var seenRevision, seenModified bool
	for _, setting := range settings {
		switch setting.Key {
		case "vcs.revision":
			if seenRevision {
				return "", false
			}
			seenRevision = true
			revision = setting.Value
		case "vcs.modified":
			if seenModified {
				return "", false
			}
			seenModified = true
			modified = setting.Value
		}
	}
	if !seenRevision || !seenModified || modified != "false" || len(revision) != 40 {
		return "", false
	}
	for i := 0; i < len(revision); i++ {
		c := revision[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return "", false
		}
	}
	return revision, true
}

func releaseProvenanceFromBuildInfo(info *debug.BuildInfo) runtimeReleaseProvenance {
	result := runtimeReleaseProvenance{
		SourceRevision:       unknownSourceRevision,
		SourceRevisionStatus: "unknown",
		PackageFormatVersion: PackageFormatVersion,
		MinRuntimeVersion:    MinRuntimeVersion,
		IRVersion:            gameir.Version,
	}
	if info == nil {
		return result
	}
	if sha, ok := cleanGoVCSRevision(info.Settings); ok {
		result.SourceRevision = sha
		result.SourceRevisionStatus = "reported_clean_vcs"
	}
	return result
}

// GET /v1/runtime/release is a privacy-neutral compatibility probe. Its VCS
// stamp describes the compiled binary; it cannot prove who published it or
// authorize a game, a seed, a rank or an iOS document callback.
func (s *Server) handleRuntimeRelease(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	info, ok := debug.ReadBuildInfo()
	if !ok {
		info = nil
	}
	writeJSON(w, http.StatusOK, releaseProvenanceFromBuildInfo(info))
}
