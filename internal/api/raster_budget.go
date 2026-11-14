package api

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"strings"
)

// These are conservative hard-fail admission ceilings, not empirical mobile
// GPU targets. A small compressed asset can still request a huge allocation.
const maxRasterDimension = 8192
const maxRasterPixels uint64 = 16_777_216
const maxPackageRasterPixels uint64 = 64_000_000

// rasterHeaderPixels validates the declared PNG/JPEG/GIF header without
// allocating a decoded image. Explicit unsupported image MIME fails closed.
// Legacy IR with no MIME is accepted only if the *actual bytes* have a
// supported PNG/JPEG/GIF header and pass the same dimension/pixel limits;
// neither a URL suffix nor an empty label bypasses those checks.
func rasterHeaderPixels(mime string, data []byte) (uint64, error) {
	var expected string
	switch strings.ToLower(strings.TrimSpace(mime)) {
	case "":
		// Legacy Game IR v1 omitted mimeType. DecodeConfig still validates
		// the actual format and dimensions before any asset is advertised.
		expected = ""
	case "image/png":
		expected = "png"
	case "image/jpeg":
		expected = "jpeg"
	case "image/gif":
		expected = "gif"
	default:
		// Unknown image MIME is not a safe opt-out from decoded pixel limits.
		// New formats require a separately versioned, tested decoder gate.
		return 0, fmt.Errorf("unsupported declared image MIME type")
	}
	config, actual, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, fmt.Errorf("invalid declared raster image header: %w", err)
	}
	if expected != "" && actual != expected {
		return 0, fmt.Errorf("raster MIME and encoded format disagree: declared %s, actual %s", expected, actual)
	}
	if actual != "png" && actual != "jpeg" && actual != "gif" {
		return 0, fmt.Errorf("unsupported legacy raster encoding")
	}
	if config.Width <= 0 || config.Height <= 0 ||
		config.Width > maxRasterDimension || config.Height > maxRasterDimension {
		return 0, fmt.Errorf("raster image dimensions exceed admission ceiling")
	}
	// Convert only positive, individually bounded dimensions to uint64;
	// avoid platform-dependent int overflow when calculating total pixels.
	pixels := uint64(config.Width) * uint64(config.Height)
	if pixels > maxRasterPixels {
		return 0, fmt.Errorf("raster image decoded pixels exceed admission ceiling")
	}
	if actual == "gif" {
		if err := gifFrameMetadataBudget(data); err != nil {
			return 0, err
		}
	}
	return pixels, nil
}

func packageRasterAggregateAllows(current, incoming uint64) bool {
	return current <= maxPackageRasterPixels &&
		incoming <= maxRasterPixels &&
		incoming <= maxPackageRasterPixels-current
}
