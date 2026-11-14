package api

import (
	"encoding/binary"
	"fmt"
)

// GIF's logical canvas is only its first image dimension. Animated frames
// may each require separate decoded pixel buffers even when compressed bytes
// and the first canvas fit in the existing per-image raster budget.
//
// Inspect *container metadata*, never DecodeAll: the latter can allocate all
// frame pixel buffers before resource admission. The caller must already
// enforce the compressed asset byte ceiling and DecodeConfig MIME/axis caps.
const maxGIFFrames = 128
const maxGIFDecodedFramePixels uint64 = 64_000_000

func gifFrameMetadataBudget(data []byte) error {
	_, err := gifFrameMetadataPixels(data)
	return err
}

// gifFrameMetadataPixels returns the already bounded sum of *all* GIF frame
// rectangles for the containing game package's aggregate decoded-pixel budget.
func gifFrameMetadataPixels(data []byte) (uint64, error) {
	if len(data) < 13 ||
		(string(data[:6]) != "GIF87a" && string(data[:6]) != "GIF89a") {
		return 0, fmt.Errorf("GIF header missing or truncated")
	}
	canvasWidth := int(binary.LittleEndian.Uint16(data[6:8]))
	canvasHeight := int(binary.LittleEndian.Uint16(data[8:10]))
	if canvasWidth == 0 || canvasHeight == 0 {
		return 0, fmt.Errorf("GIF logical screen has empty dimensions")
	}
	cursor := 13
	if data[10]&0x80 != 0 {
		tableBytes := 3 * (1 << (int(data[10]&0x07) + 1))
		if len(data)-cursor < tableBytes {
			return 0, fmt.Errorf("truncated GIF global color table")
		}
		cursor += tableBytes
	}

	var decodedPixels uint64
	frames := 0
	for {
		if cursor >= len(data) {
			return 0, fmt.Errorf("truncated GIF before trailer")
		}
		block := data[cursor]
		cursor++
		switch block {
		case 0x3b: // End of GIF container.
			if frames == 0 {
				return 0, fmt.Errorf("GIF contains no image frames")
			}
			if cursor != len(data) {
				return 0, fmt.Errorf("unexpected data after GIF trailer")
			}
			return decodedPixels, nil
		case 0x21: // Extension block, including graphic control.
			if cursor >= len(data) {
				return 0, fmt.Errorf("truncated GIF extension label")
			}
			cursor++ // Extension label.
			next, err := skipGIFDataSubblocks(data, cursor)
			if err != nil {
				return 0, err
			}
			cursor = next
		case 0x2c: // Image descriptor.
			if len(data)-cursor < 9 {
				return 0, fmt.Errorf("truncated GIF frame descriptor")
			}
			left := int(binary.LittleEndian.Uint16(data[cursor : cursor+2]))
			top := int(binary.LittleEndian.Uint16(data[cursor+2 : cursor+4]))
			width := int(binary.LittleEndian.Uint16(data[cursor+4 : cursor+6]))
			height := int(binary.LittleEndian.Uint16(data[cursor+6 : cursor+8]))
			packed := data[cursor+8]
			cursor += 9
			if width == 0 || height == 0 ||
				width > maxRasterDimension || height > maxRasterDimension ||
				left+width > canvasWidth || top+height > canvasHeight {
				return 0, fmt.Errorf("GIF frame dimensions exceed logical canvas or admission bounds")
			}
			framePixels := uint64(width) * uint64(height)
			if framePixels > maxRasterPixels {
				return 0, fmt.Errorf("GIF frame exceeds decoded pixel budget")
			}
			if frames >= maxGIFFrames ||
				decodedPixels > maxGIFDecodedFramePixels-framePixels {
				return 0, fmt.Errorf("GIF animation exceeds frame or aggregate decoded pixel budget")
			}
			frames++
			decodedPixels += framePixels
			if packed&0x80 != 0 {
				tableBytes := 3 * (1 << (int(packed&0x07) + 1))
				if len(data)-cursor < tableBytes {
					return 0, fmt.Errorf("truncated GIF local color table")
				}
				cursor += tableBytes
			}
			if cursor >= len(data) {
				return 0, fmt.Errorf("missing GIF LZW minimum code size")
			}
			lzwMinSize := data[cursor]
			cursor++
			if lzwMinSize < 2 || lzwMinSize > 8 {
				return 0, fmt.Errorf("invalid GIF minimum LZW code size")
			}
			next, err := skipGIFDataSubblocks(data, cursor)
			if err != nil {
				return 0, err
			}
			cursor = next
		default:
			return 0, fmt.Errorf("invalid GIF container block marker")
		}
	}
}

// GIF image data and extension payloads are a sequence of length-prefixed
// subblocks, terminated by one zero byte. Never trust a length that exceeds
// the remaining compressed payload; no allocation, recursion or LZW decode.
func skipGIFDataSubblocks(data []byte, cursor int) (int, error) {
	for {
		if cursor >= len(data) {
			return 0, fmt.Errorf("truncated GIF data subblock")
		}
		size := int(data[cursor])
		cursor++
		if len(data)-cursor < size {
			return 0, fmt.Errorf("truncated GIF subblock payload")
		}
		cursor += size
		if size == 0 {
			return cursor, nil
		}
	}
}
