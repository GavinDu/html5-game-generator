package media

import (
	"bytes"
	"encoding/binary"
	"math"
)

// MusicWAV synthesizes a short loopable tune as 16-bit mono WAV.
// The mood (from the plot) picks the scale and tempo. This stands in for
// the licensed-library matcher; same interface (mood in -> audio out).
func MusicWAV(mood string) []byte {
	const rate = 22050
	const secs = 8

	// Frequencies in Hz for the arpeggio, per mood.
	var notes []float64
	var beatSec float64
	switch mood {
	case "spooky":
		notes = []float64{220.00, 261.63, 311.13, 233.08, 220.00, 174.61} // A C Eb Bb A F
		beatSec = 0.55
	case "calm":
		notes = []float64{174.61, 220.00, 261.63, 349.23, 261.63, 220.00} // F A C F C A
		beatSec = 0.6
	case "playful":
		// 宫商角徵羽 pentatonic, music-box register: C5 D5 E5 G5 A5 C6.
		notes = []float64{659.25, 783.99, 880.00, 783.99, 659.25, 587.33, 523.25, 587.33}
		beatSec = 0.34
	default: // adventurous
		notes = []float64{261.63, 329.63, 392.00, 523.25, 392.00, 329.63} // C E G C G E
		beatSec = 0.42
	}

	total := rate * secs
	samples := make([]int16, total)
	noteSamples := int(beatSec * rate)

	for i := range samples {
		ni := (i / noteSamples) % len(notes)
		posInNote := i % noteSamples
		f := notes[ni]
		// Simple ADSR-ish envelope to avoid clicks.
		attack := float64(posInNote) / float64(noteSamples) * 4
		if attack > 1 {
			attack = 1
		}
		release := float64(noteSamples-posInNote) / float64(noteSamples) * 4
		if release > 1 {
			release = 1
		}
		env := attack
		if release < env {
			env = release
		}
		// Fundamental + soft octave for warmth.
		v := math.Sin(2*math.Pi*f*float64(i)/rate)*0.7 +
			math.Sin(2*math.Pi*2*f*float64(i)/rate)*0.2
		samples[i] = int16(v * env * 0.5 * 32767)
	}

	var buf bytes.Buffer
	dataSize := len(samples) * 2
	// RIFF header.
	buf.WriteString("RIFF")
	binary.Write(&buf, binary.LittleEndian, uint32(36+dataSize))
	buf.WriteString("WAVE")
	// fmt chunk.
	buf.WriteString("fmt ")
	binary.Write(&buf, binary.LittleEndian, uint32(16))
	binary.Write(&buf, binary.LittleEndian, uint16(1)) // PCM
	binary.Write(&buf, binary.LittleEndian, uint16(1)) // mono
	binary.Write(&buf, binary.LittleEndian, uint32(rate))
	binary.Write(&buf, binary.LittleEndian, uint32(rate*2)) // byte rate
	binary.Write(&buf, binary.LittleEndian, uint16(2))      // block align
	binary.Write(&buf, binary.LittleEndian, uint16(16))     // bits
	// data chunk.
	buf.WriteString("data")
	binary.Write(&buf, binary.LittleEndian, uint32(dataSize))
	binary.Write(&buf, binary.LittleEndian, samples)
	return buf.Bytes()
}
