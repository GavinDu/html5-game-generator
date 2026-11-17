package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"doodlequest/internal/api"
	"doodlequest/internal/gameir"
)

const releaseSmokeGameID = "escape-from-injection"

func realGoPackageFixture(t *testing.T) api.GamePackage {
	t.Helper()
	raw, err := os.ReadFile("../../internal/attestation/testdata/start_score_ir_v1.json")
	if err != nil {
		t.Fatalf("read real shared Go/Pixi IR fixture: %v", err)
	}
	doc, err := gameir.DecodeAndValidate(raw)
	if err != nil {
		t.Fatalf("strict decode shared Go/Pixi fixture: %v", err)
	}
	ir, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("canonical Go IR marshal: %v", err)
	}
	sum := sha256.Sum256(ir)
	sha := hex.EncodeToString(sum[:])
	return api.GamePackage{
		PackageFormatVersion: api.PackageFormatVersion,
		MinRuntimeVersion:    api.MinRuntimeVersion,
		IRVersion:            doc.IRVersion,
		GameID:               doc.Game.ID,
		IR:                   doc,
		Assets:               []api.GamePackageAsset{},
		IRHash:               sha[:12],
		IRSHA256:             sha,
	}
}

func TestLiveReleaseSmokeAcceptsOnlyExactGoPackageContract(t *testing.T) {
	fixture := realGoPackageFixture(t)
	if fixture.GameID != releaseSmokeGameID {
		t.Fatalf("shared golden game identity drift")
	}
	cases := []struct {
		name        string
		mutate      func(*api.GamePackage)
		status      int
		contentType string
		redirect    bool
		wantError   string
	}{
		{name: "valid"},
		{name: "missing_format", mutate: func(p *api.GamePackage) { p.PackageFormatVersion = "" }, wantError: "missing_package_format_version"},
		{name: "wrong_format", mutate: func(p *api.GamePackage) { p.PackageFormatVersion = "1.0" }, wantError: "unexpected_package_format_version"},
		{name: "wrong_min_runtime", mutate: func(p *api.GamePackage) { p.MinRuntimeVersion = "legacy" }, wantError: "missing_or_wrong_min_runtime_version"},
		{name: "foreign_game", mutate: func(p *api.GamePackage) { p.GameID = "foreign-game" }, wantError: "package_game_id_mismatch"},
		{name: "missing_ir", mutate: func(p *api.GamePackage) { p.IR = nil }, wantError: "missing_game_ir"},
		{name: "wrong_ir_version", mutate: func(p *api.GamePackage) { p.IRVersion = "0.9" }, wantError: "wrong_game_ir_version"},
		{name: "missing_assets_array", mutate: func(p *api.GamePackage) { p.Assets = nil }, wantError: "missing_assets_array"},
		{name: "bad_full_hash", mutate: func(p *api.GamePackage) { p.IRSHA256 = strings.Repeat("a", 64) }, wantError: "package_full_ir_sha256_mismatch"},
		{name: "bad_short_hash", mutate: func(p *api.GamePackage) { p.IRHash = "000000000000" }, wantError: "package_short_ir_hash_mismatch"},
		{name: "missing_asset_hash", mutate: func(p *api.GamePackage) { p.Assets = []api.GamePackageAsset{{ID: "sprite"}} }, wantError: "missing_asset_sha256"},
		{name: "bad_asset_hash", mutate: func(p *api.GamePackage) {
			p.Assets = []api.GamePackageAsset{{ID: "sprite", SHA256: strings.Repeat("z", 64)}}
		}, wantError: "invalid_asset_sha256"},
		{name: "not_published", status: http.StatusNotFound, wantError: "package_http_status_404"},
		{name: "cross_origin_redirect", redirect: true, wantError: "package_http_status_302"},
		{name: "not_json_content_type", contentType: "text/html", wantError: "non_json_package"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pkg := fixture
			if tc.mutate != nil {
				tc.mutate(&pkg)
			}
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/games/"+releaseSmokeGameID+"/package" {
					t.Errorf("release smoke unexpectedly contacted %s", r.URL.Path)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
					t.Error("release smoke must never send credentials")
				}
				if tc.redirect {
					http.Redirect(w, r, "https://different.example/package", http.StatusFound)
					return
				}
				if tc.status != 0 {
					w.WriteHeader(tc.status)
					return
				}
				mime := tc.contentType
				if mime == "" {
					mime = "application/json; charset=utf-8"
				}
				w.Header().Set("Content-Type", mime)
				_ = json.NewEncoder(w).Encode(pkg)
			}))
			defer srv.Close()
			result, err := verifyLivePackage(context.Background(), srv.Client(), srv.URL, releaseSmokeGameID)
			if tc.wantError == "" {
				if err != nil || result.Format != api.PackageFormatVersion ||
					result.IRVersion != gameir.Version || result.AssetCount != 0 {
					t.Fatalf("canonical real Go IR rejected: %+v %v", result, err)
				}
				return
			}
			if err == nil || err.Error() != tc.wantError || result != (packageReport{}) {
				t.Fatalf("unsafe or unspecific package admission: result=%+v err=%v; want %s", result, err, tc.wantError)
			}
		})
	}
}

func TestLiveReleaseSmokeRejectsOversizeAndMalformedJSONWithoutDumpingResponse(t *testing.T) {
	cases := []struct {
		name string
		body []byte
		want string
	}{
		{name: "oversize", body: bytes.Repeat([]byte("x"), maxEnvelopeBytes+1), want: "package_envelope_too_large"},
		{name: "malformed", body: []byte("{invalid"), want: "invalid_package_json"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(tc.body)
			}))
			defer srv.Close()
			report, err := verifyLivePackage(context.Background(), srv.Client(), srv.URL, releaseSmokeGameID)
			if err == nil || err.Error() != tc.want || report != (packageReport{}) {
				t.Fatalf("malformed or oversized release package was admitted: %+v %v", report, err)
			}
		})
	}
}

func TestLiveReleaseSmokeRejectsUnsafeOriginsAndGameIDsBeforeHTTP(t *testing.T) {
	unsafe := []string{
		"http://deployed.example", "https://user:secret@deployed.example",
		"https://deployed.example/path", "https://deployed.example?key=hidden",
		"https://deployed.example/#fragment", "https://",
	}
	for _, raw := range unsafe {
		if _, err := parseReleaseOrigin(raw); err == nil {
			t.Fatalf("unsafe origin was admitted")
		}
	}
	for _, game := range []string{"", "../admin", "bad%2fid", "name with spaces", strings.Repeat("g", 129)} {
		_, err := verifyLivePackage(context.Background(), http.DefaultClient, "https://deployed.example", game)
		if err == nil || err.Error() != "invalid_game_id" {
			t.Fatalf("unsafe game identity should fail before HTTP: %v", err)
		}
	}
}
