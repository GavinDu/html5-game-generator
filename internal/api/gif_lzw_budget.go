package api

import (
	"compress/lzw"
	"encoding/binary"
	"fmt"
	"io"
)

// gifSubblockReader exposes only compressed image bytes, skipping GIF's
// length-prefix bytes. It never duplicates the compressed image or allocates
// decoded frames. The enclosing metadata gate checks every subblock boundary.
type gifSubblockReader struct {
	data      []byte
	offset    int
	remaining int
	done      bool
}

func (r *gifSubblockReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.done {
		return 0, io.EOF
	}
	if r.remaining == 0 {
		if r.offset >= len(r.data) {
			return 0, io.ErrUnexpectedEOF
		}
		n := int(r.data[r.offset])
		r.offset++
		if n == 0 {
			r.done = true
			return 0, io.EOF
		}
		if n > len(r.data)-r.offset {
			return 0, io.ErrUnexpectedEOF
		}
		r.remaining = n
	}
	n := copy(p, r.data[r.offset:r.offset+r.remaining])
	r.offset += n
	r.remaining -= n
	return n, nil
}

// validateGIFLZWImage decodes palette *indexes* into an 8 KiB scratch buffer.
// It does not allocate a frame, RGBA bitmap, or all animated frames. An early
// LZW end code, corrupt stream, or output exceeding its declared frame area
// fails closed. This does not validate animation disposal/GPU compositing.
func validateGIFLZWImage(subblocks []byte, litWidth int, pixels uint64, paletteEntries int) error {
	if paletteEntries < 2 || paletteEntries > 256 {
		return fmt.Errorf("GIF frame has no supported active color table")
	}
	if pixels == 0 || pixels > maxRasterPixels {
		return fmt.Errorf("GIF LZW frame pixel budget exceeded")
	}
	reader := lzw.NewReader(&gifSubblockReader{data: subblocks}, lzw.LSB, litWidth)
	defer reader.Close()
	var scratch [8192]byte
	var decoded uint64
	for {
		n, err := reader.Read(scratch[:])
		if uint64(n) > pixels-decoded {
			return fmt.Errorf("GIF LZW stream emits more pixels than frame")
		}
		// Indices are bytes, but a GIF frame can declare only 2/4/.../256
		// palette entries. An invalid index must never reach WebKit's
		// browser decoder, even when compressed syntax and pixel count fit.
		if paletteEntries < 256 {
			for _, index := range scratch[:n] {
				if int(index) >= paletteEntries {
					return fmt.Errorf("GIF frame index exceeds active color table")
				}
			}
		}
		decoded += uint64(n)
		if err == io.EOF {
			if decoded != pixels {
				return fmt.Errorf("GIF LZW output shorter than frame")
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("invalid GIF LZW frame stream: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("GIF LZW decoder made no progress")
		}
	}
}

// gifLZWFrameBudget runs only AFTER metadata, compressed-byte and per-frame
// pixel admission. It checks decompression completeness without DecodeAll's
// unbounded array of frame allocations. This second streaming pass is bounded
// by the previously accepted 128 frames and 64M total decoded indexes.
func gifLZWFrameBudget(data []byte) error {
	if err := gifFrameMetadataBudget(data); err != nil {
		return err
	}
	cursor := 13
	globalPaletteEntries := 0
	if data[10]&0x80 != 0 {
		globalPaletteEntries = 1 << (int(data[10]&0x07) + 1)
		cursor += 3 * globalPaletteEntries
	}
	var pendingTransparencyIndex byte
	var pendingTransparencyEnabled bool
	var pendingGraphicControl bool
	for {
		if cursor >= len(data) {
			return fmt.Errorf("GIF LZW scanner missing trailer")
		}
		block := data[cursor]
		cursor++
		switch block {
		case 0x3b:
			if pendingGraphicControl {
				return fmt.Errorf("GIF graphic control has no following graphic block")
			}
			return nil
		case 0x21:
			// Metadata admission has already bounded subblock boundaries.
			// Graphic Control (0xf9) is exactly one 4-byte subblock:
			// packed flags, delay low/high, transparent color index.
			label := data[cursor]
			cursor++
			next, err := skipGIFDataSubblocks(data, cursor)
			if err != nil {
				return err
			}
			switch label {
			case 0xf9:
				if pendingGraphicControl {
					return fmt.Errorf("duplicate GIF graphic control before image")
				}
				if next-cursor != 6 || data[cursor] != 4 || data[cursor+5] != 0 {
					return fmt.Errorf("invalid GIF graphic control block")
				}
				pendingGraphicControl = true
				pendingTransparencyEnabled = data[cursor+1]&0x01 != 0
				pendingTransparencyIndex = data[cursor+4]
			case 0x01:
				// GIF89a Plain Text Extension starts with one fixed 12-byte
				// text-grid descriptor, followed by zero or more text subblocks.
				// The first metadata pass proves the length is in bounds, but
				// does not enforce this label-specific shape. Do not let a
				// malformed descriptor masquerade as a valid browser asset.
				if data[cursor] != 12 {
					return fmt.Errorf("GIF plain text extension requires a 12-byte header")
				}
				// A standalone uploaded GIF has no saved color table from a
				// previous stream. Plain Text always indexes the GLOBAL palette,
				// even if the following image supplies its own local palette.
				if globalPaletteEntries == 0 {
					return fmt.Errorf("GIF plain text extension requires a global color table")
				}
				if int(data[cursor+11]) >= globalPaletteEntries ||
					int(data[cursor+12]) >= globalPaletteEntries {
					return fmt.Errorf("GIF plain text colors exceed global color table")
				}
				// Plain text is a GIF graphic rendering block. It consumes
				// the pending GCE, unlike comment/application extensions.
				pendingGraphicControl = false
				pendingTransparencyEnabled = false
			}
			cursor = next
		case 0x2c:
			// The first metadata pass already proved the descriptor and all
			// frame bounds. Do not trust them without that pass.
			width := uint64(binary.LittleEndian.Uint16(data[cursor+4 : cursor+6]))
			height := uint64(binary.LittleEndian.Uint16(data[cursor+6 : cursor+8]))
			packed := data[cursor+8]
			cursor += 9
			activePaletteEntries := globalPaletteEntries
			if packed&0x80 != 0 {
				// Local palette overrides the global table for THIS frame.
				activePaletteEntries = 1 << (int(packed&0x07) + 1)
				cursor += 3 * activePaletteEntries
			}
			if activePaletteEntries == 0 {
				return fmt.Errorf("GIF frame has no local or global color table")
			}
			if pendingTransparencyEnabled &&
				int(pendingTransparencyIndex) >= activePaletteEntries {
				return fmt.Errorf("GIF transparent index exceeds active color table")
			}
			pendingGraphicControl = false
			pendingTransparencyEnabled = false
			minCodeSize := int(data[cursor])
			cursor++
			next, err := skipGIFDataSubblocks(data, cursor)
			if err != nil {
				return err
			}
			if err := validateGIFLZWImage(data[cursor:next], minCodeSize, width*height, activePaletteEntries); err != nil {
				return err
			}
			cursor = next
		default:
			return fmt.Errorf("unexpected GIF LZW container marker")
		}
	}
}
