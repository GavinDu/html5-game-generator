package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime/debug"
	"strings"
	"testing"

	"doodlequest/internal/gameir"
	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

func TestReleaseProvenanceAcceptsOnlyCleanExactGoVCSStamp(t *testing.T) {
	good := strings.Repeat("a1", 20)
	revision := debug.BuildSetting{Key: "vcs.revision", Value: good}
	clean := debug.BuildSetting{Key: "vcs.modified", Value: "false"}
	dirty := debug.BuildSetting{Key: "vcs.modified", Value: "true"}
	tests := []struct {
		name     string
		settings []debug.BuildSetting
		valid    bool
	}{
		{name: "clean_full_sha", settings: []debug.BuildSetting{revision, clean}, valid: true},
		{name: "clean_reordered", settings: []debug.BuildSetting{clean, revision}, valid: true},
		{name: "missing_all"},
		{name: "missing_modified", settings: []debug.BuildSetting{revision}},
		{name: "missing_revision", settings: []debug.BuildSetting{clean}},
		{name: "dirty", settings: []debug.BuildSetting{revision, dirty}},
		{name: "unknown_dirty_flag", settings: []debug.BuildSetting{revision, {Key: "vcs.modified", Value: "maybe"}}},
		{name: "short_prefix", settings: []debug.BuildSetting{{Key: "vcs.revision", Value: good[:12]}, clean}},
		{name: "uppercase", settings: []debug.BuildSetting{{Key: "vcs.revision", Value: strings.ToUpper(good)}, clean}},
		{name: "nonhex", settings: []debug.BuildSetting{{Key: "vcs.revision", Value: strings.Repeat("g", 40)}, clean}},
		{name: "padding", settings: []debug.BuildSetting{{Key: "vcs.revision", Value: good + " "}, clean}},
		{name: "duplicate_revision", settings: []debug.BuildSetting{revision, clean, revision}},
		{name: "duplicate_modified", settings: []debug.BuildSetting{revision, clean, clean}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := cleanGoVCSRevision(tc.settings)
			if ok != tc.valid || (ok && got != good) || (!ok && got != "") {
				t.Fatalf("unexpected VCS acceptance: ok=%v SHA=%q", ok, got)
			}
			report := releaseProvenanceFromBuildInfo(&debug.BuildInfo{Settings: tc.settings})
			if report.PackageFormatVersion != PackageFormatVersion ||
				report.MinRuntimeVersion != MinRuntimeVersion ||
				report.IRVersion != gameir.Version {
				t.Fatal("release constants must match the actual compiled package contract")
			}
			if tc.valid {
				if report.SourceRevision != good || report.SourceRevisionStatus != "reported_clean_vcs" {
					t.Fatal("a clean compiled VCS revision was not surfaced")
				}
			} else if report.SourceRevision != unknownSourceRevision ||
				report.SourceRevisionStatus != "unknown" {
				t.Fatalf("unsafe VCS input was claimed as a release: %+v", report)
			}
		})
	}
	withoutInfo := releaseProvenanceFromBuildInfo(nil)
	if withoutInfo.SourceRevision != unknownSourceRevision ||
		withoutInfo.SourceRevisionStatus != "unknown" {
		t.Fatalf("missing Go build info must fail closed: %+v", withoutInfo)
	}
}

func TestReleaseProvenanceHTTPDoesNotLeakRequestOrClaimUnknownSource(t *testing.T) {
	st := store.NewMemoryStore()
	server := New(st, pipeline.NewRunner(st))
	handler := server.Routes()
	const sensitive = "private-do-not-log-release-token"
	probe := func(target string) (int, map[string]any, http.Header, string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.Header.Set("Authorization", "Bearer "+sensitive)
		req.Header.Set("X-User-ID", "private-viewer")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		var fields map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
			t.Fatalf("invalid release JSON: %v", err)
		}
		return response.Code, fields, response.Header(), response.Body.String()
	}
	code, fields, header, body := probe("/v1/runtime/release")
	if code != http.StatusOK || header.Get("Cache-Control") != "no-store" ||
		header.Get("X-Content-Type-Options") != "nosniff" ||
		!strings.Contains(header.Get("Content-Type"), "application/json") {
		t.Fatalf("untrusted release endpoint headers: HTTP %d, cache=%q", code, header.Get("Cache-Control"))
	}
	if len(fields) != 5 {
		t.Fatalf("release endpoint must expose exactly five safe fields, got %d", len(fields))
	}
	for _, key := range []string{"sourceRevision", "sourceRevisionStatus", "packageFormatVersion", "minRuntimeVersion", "irVersion"} {
		if _, ok := fields[key]; !ok {
			t.Fatalf("required public source/compatibility field missing: %s", key)
		}
	}
	if fields["packageFormatVersion"] != PackageFormatVersion ||
		fields["minRuntimeVersion"] != MinRuntimeVersion ||
		fields["irVersion"] != gameir.Version {
		t.Fatalf("release response differs from canonical source contract: %+v", fields)
	}
	if source, ok := fields["sourceRevision"].(string); !ok {
		t.Fatal("source revision must be an explicit string")
	} else if source == unknownSourceRevision {
		if fields["sourceRevisionStatus"] != "unknown" {
			t.Fatal("an unknown revision must not report a verified source")
		}
	} else if _, clean := cleanGoVCSRevision([]debug.BuildSetting{
		{Key: "vcs.revision", Value: source}, {Key: "vcs.modified", Value: "false"},
	}); !clean || fields["sourceRevisionStatus"] != "reported_clean_vcs" {
		t.Fatal("the release response must not report malformed source metadata")
	}
	if strings.Contains(body, sensitive) || strings.Contains(body, "private-viewer") ||
		strings.Contains(body, "Authorization") {
		t.Fatal("the release endpoint leaked request authentication or viewer metadata")
	}
	// Clients cannot override the compiled revision through untrusted query or headers.
	queryCode, queryFields, _, _ := probe("/v1/runtime/release?sourceRevision="+strings.Repeat("f", 40))
	if queryCode != http.StatusOK || queryFields["sourceRevision"] != fields["sourceRevision"] {
		t.Fatal("a query parameter altered compiled release provenance")
	}
	wrongMethod := httptest.NewRecorder()
	handler.ServeHTTP(wrongMethod, httptest.NewRequest(http.MethodPost, "/v1/runtime/release", nil))
	if wrongMethod.Code != http.StatusMethodNotAllowed {
		t.Fatalf("release endpoint must reject write methods, got %d", wrongMethod.Code)
	}
}
