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
func validateGIFLZWImage(subblocks []byte, litWidth int, pixels uint64) error {
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
	if data[10]&0x80 != 0 {
		cursor += 3 * (1 << (int(data[10]&0x07) + 1))
	}
	for {
		if cursor >= len(data) {
			return fmt.Errorf("GIF LZW scanner missing trailer")
		}
		block := data[cursor]
		cursor++
		switch block {
		case 0x3b:
			return nil
		case 0x21:
			// All extension subblocks were already length-checked.
			cursor++
			next, err := skipGIFDataSubblocks(data, cursor)
			if err != nil {
				return err
			}
			cursor = next
		case 0x2c:
			// The first metadata pass already proved the descriptor and all
			// frame bounds. Do not trust them without that pass.
			width := uint64(binary.LittleEndian.Uint16(data[cursor+4 : cursor+6]))
			height := uint64(binary.LittleEndian.Uint16(data[cursor+6 : cursor+8]))
			packed := data[cursor+8]
			cursor += 9
			if packed&0x80 != 0 {
				cursor += 3 * (1 << (int(packed&0x07) + 1))
			}
			minCodeSize := int(data[cursor])
			cursor++
			next, err := skipGIFDataSubblocks(data, cursor)
			if err != nil {
				return err
			}
			if err := validateGIFLZWImage(data[cursor:next], minCodeSize, width*height); err != nil {
				return err
			}
			cursor = next
		default:
			return fmt.Errorf("unexpected GIF LZW container marker")
		}
	}
}
