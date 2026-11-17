// Command verify-live-package performs an explicit, read-only pre-release
// check against a REAL published game package. It never publishes, deploys,
// authenticates players, or treats a browser score as authoritative.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"doodlequest/internal/api"
	"doodlequest/internal/gameir"
)

const maxEnvelopeBytes = 8 << 20
const requestTimeout = 10 * time.Second

var safeGameID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,127}$`)

// packageReport deliberately contains no creator identity, Game IR, game ID,
// access credentials or raw response content.
type packageReport struct {
	Format string
	IRVersion string
	AssetCount int
}

func parseReleaseOrigin(raw string) (*url.URL, error) {
	origin, err := url.Parse(raw)
	if err != nil || origin == nil || origin.Scheme != "https" ||
		origin.Hostname() == "" || origin.User != nil ||
		(origin.Path != "" && origin.Path != "/") ||
		origin.RawQuery != "" || origin.Fragment != "" ||
		strings.HasSuffix(origin.Host, ":") {
		return nil, errors.New("invalid_https_release_origin")
	}
	origin.Path = ""
	origin.RawPath = ""
	return origin, nil
}

// verifyLivePackage never follows a redirect or logs the body. The injected
// client is used only for test TLS trust; CheckRedirect is overridden here.
func verifyLivePackage(ctx context.Context, client *http.Client, baseURL string, gameID string) (packageReport, error) {
	origin, err := parseReleaseOrigin(baseURL)
	if err != nil {
		return packageReport{}, err
	}
	if !safeGameID.MatchString(gameID) {
		return packageReport{}, errors.New("invalid_game_id")
	}
	if client == nil {
		return packageReport{}, errors.New("missing_http_client")
	}
	safeClient := *client
	// A release smoke is anonymous/public even if the injected test client
	// happens to be configured with a cookie jar.
	safeClient.Jar = nil
	safeClient.Timeout = requestTimeout
	safeClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}

	packageURL := *origin
	packageURL.Path = "/v1/games/" + gameID + "/package"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, packageURL.String(), nil)
	if err != nil {
		return packageReport{}, errors.New("invalid_package_request")
	}
	req.Header.Set("Accept", "application/json")
	response, err := safeClient.Do(req)
	if err != nil {
		return packageReport{}, errors.New("package_transport_failure")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		// No response body is emitted, including redirects or auth errors.
		return packageReport{}, fmt.Errorf("package_http_status_%d", response.StatusCode)
	}
	contentType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		return packageReport{}, errors.New("non_json_package")
	}
	limited, err := io.ReadAll(io.LimitReader(response.Body, maxEnvelopeBytes+1))
	if err != nil {
		return packageReport{}, errors.New("package_body_read_failure")
	}
	if len(limited) > maxEnvelopeBytes {
		return packageReport{}, errors.New("package_envelope_too_large")
	}
	var pkg api.GamePackage
	if err := json.Unmarshal(limited, &pkg); err != nil {
		return packageReport{}, errors.New("invalid_package_json")
	}
	if pkg.PackageFormatVersion == "" {
		return packageReport{}, errors.New("missing_package_format_version")
	}
	if pkg.PackageFormatVersion != api.PackageFormatVersion {
		return packageReport{}, errors.New("unexpected_package_format_version")
	}
	if pkg.MinRuntimeVersion != api.MinRuntimeVersion {
		return packageReport{}, errors.New("missing_or_wrong_min_runtime_version")
	}
	if pkg.GameID != gameID {
		return packageReport{}, errors.New("package_game_id_mismatch")
	}
	if pkg.IR == nil {
		return packageReport{}, errors.New("missing_game_ir")
	}
	if pkg.IRVersion != gameir.Version || pkg.IR.IRVersion != gameir.Version {
		return packageReport{}, errors.New("wrong_game_ir_version")
	}
	if pkg.Assets == nil {
		return packageReport{}, errors.New("missing_assets_array")
	}
	if err := gameir.Validate(pkg.IR); err != nil {
		return packageReport{}, errors.New("invalid_published_game_ir")
	}
	// The API publisher hashes Go json.Marshal(*gameir.Document) before
	// serializing the envelope. Using the exact typed Go document keeps
	// release-smoke canonicalization aligned with the actual source.
	canonicalIR, err := json.Marshal(pkg.IR)
	if err != nil {
		return packageReport{}, errors.New("cannot_canonicalize_game_ir")
	}
	digest := sha256.Sum256(canonicalIR)
	sha := hex.EncodeToString(digest[:])
	if pkg.IRSHA256 != sha {
		return packageReport{}, errors.New("package_full_ir_sha256_mismatch")
	}
	if pkg.IRHash != sha[:12] {
		return packageReport{}, errors.New("package_short_ir_hash_mismatch")
	}
	for _, asset := range pkg.Assets {
		if len(asset.SHA256) != 64 {
			return packageReport{}, errors.New("missing_asset_sha256")
		}
		if bytes, err := hex.DecodeString(asset.SHA256); err != nil || len(bytes) != sha256.Size {
			return packageReport{}, errors.New("invalid_asset_sha256")
		}
	}
	return packageReport{
		Format: pkg.PackageFormatVersion,
		IRVersion: pkg.IRVersion,
		AssetCount: len(pkg.Assets),
	}, nil
}

func main() {
	base := flag.String("base-url", "", "HTTPS origin of deployed public Game Package service (no credentials)")
	game := flag.String("game-id", "", "Known real public published game ID (never printed)")
	flag.Parse()
	if flag.NArg() != 0 || *base == "" || *game == "" {
		fmt.Fprintln(os.Stderr, "usage: verify-live-package -base-url https://service.example -game-id PUBLIC_GAME_ID")
		os.Exit(2)
	}
	result, err := verifyLivePackage(context.Background(), http.DefaultClient, *base, *game)
	if err != nil {
		fmt.Fprintf(os.Stderr, "LIVE_PACKAGE_CONTRACT_FAIL reason=%s\n", err.Error())
		os.Exit(1)
	}
	fmt.Printf("LIVE_PACKAGE_CONTRACT_OK format=%s irVersion=%s assets=%d\n",
		result.Format, result.IRVersion, result.AssetCount)
}
