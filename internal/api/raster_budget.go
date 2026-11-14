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
// allocating a decoded image. Unsupported and missing image MIME labels
// fail closed instead of bypassing the decoded budget; new formats require
// explicit versioned capability and header-decode support. Callers must first
// enforce byte limits.
func rasterHeaderPixels(mime string, data []byte) (uint64, error) {
	var expected string
	switch strings.ToLower(strings.TrimSpace(mime)) {
	case "image/png":
		expected = "png"
	case "image/jpeg":
		expected = "jpeg"
	case "image/gif":
		expected = "gif"
	default:
		// Unknown image MIME is not a safe opt-out from decoded pixel limits.
		// New formats require a separately versioned, tested decoder gate.
		return 0, fmt.Errorf("unsupported or missing image MIME type")
	}
	config, actual, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, fmt.Errorf("invalid declared raster image header: %w", err)
	}
	if actual != expected {
		return 0, fmt.Errorf("raster MIME and encoded format disagree: declared %s, actual %s", expected, actual)
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
	return pixels, nil
}

func packageRasterAggregateAllows(current, incoming uint64) bool {
	return current <= maxPackageRasterPixels &&
		incoming <= maxRasterPixels &&
		incoming <= maxPackageRasterPixels-current
}
