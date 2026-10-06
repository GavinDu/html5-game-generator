class_name Synth
extends RefCounted
## Procedural audio: music-box loops and short SFX, generated as AudioStreamWAV.
## No binary audio assets needed.

const RATE := 16000


static func _finish(data: PackedByteArray, loop: bool) -> AudioStreamWAV:
	var wav := AudioStreamWAV.new()
	wav.format = AudioStreamWAV.FORMAT_16_BITS
	wav.mix_rate = RATE
	wav.stereo = false
	wav.data = data
	if loop:
		wav.loop_mode = AudioStreamWAV.LOOP_FORWARD
		wav.loop_begin = 0
		wav.loop_end = data.size() / 2
	return wav


## Short enveloped sine blip.
static func tone(freq: float, dur: float, volume := 0.5, harmonics := 0.25) -> AudioStreamWAV:
	var n := int(dur * RATE)
	var data := PackedByteArray()
	data.resize(n * 2)
	for i in range(n):
		var t := float(i) / RATE
		var env := 1.0 - float(i) / float(n)
		env *= env
		var s := (sin(TAU * freq * t) + harmonics * sin(TAU * freq * 2.0 * t)) * env * volume
		data.encode_s16(i * 2, int(clampf(s, -1.0, 1.0) * 32767.0))
	return _finish(data, false)


## Music-box arpeggio loop. notes: Array[float] frequencies, one per beat.
static func music_box(notes: Array, beat_sec: float, volume := 0.32) -> AudioStreamWAV:
	var total := int(notes.size() * beat_sec * RATE)
	var data := PackedByteArray()
	data.resize(total * 2)
	for i in range(total):
		var t := float(i) / RATE
		var beat := int(t / beat_sec) % notes.size()
		var freq: float = notes[beat]
		var bt := fmod(t, beat_sec) / beat_sec
		var env := exp(-3.5 * bt)
		var s := (sin(TAU * freq * t) + 0.3 * sin(TAU * freq * 2.0 * t)
				+ 0.12 * sin(TAU * freq * 3.0 * t)) * env * volume
		data.encode_s16(i * 2, int(clampf(s, -1.0, 1.0) * 32767.0))
	return _finish(data, true)


## Calm loop matching the web original: F A C F C A.
static func calm_loop() -> AudioStreamWAV:
	return music_box([174.61, 220.0, 261.63, 349.23, 261.63, 220.0], 0.6)


## Playful pentatonic loop (sky game): C5 D5 E5 G5 A5 C6.
static func playful_loop() -> AudioStreamWAV:
	return music_box([523.25, 587.33, 659.25, 783.99, 880.0, 1046.5, 880.0, 783.99], 0.34, 0.28)


## Adventurous loop (escape game): C E G C G E, driving eighth feel.
static func adventurous_loop() -> AudioStreamWAV:
	return music_box([261.63, 329.63, 392.0, 523.25, 392.0, 329.63,
		261.63, 329.63, 392.0, 523.25, 659.25, 523.25], 0.32, 0.3)
