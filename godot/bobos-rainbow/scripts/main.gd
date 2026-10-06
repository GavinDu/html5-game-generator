extends Node2D
## Bobo's Rainbow — Godot rework.
## Catch falling hearts to repaint the rainbow. 12 hearts clear the round and
## start the next LV, 1.25x faster. 3 misses end the run; your LV hits the
## leaderboard. Same rules as the GameShim original, rebuilt with Godot's
## physics areas, particles, tweens, camera shake and procedural audio.

const HEARTS_TO_WIN := 12
const MAX_MISSES := 3
const MAX_COMBO := 5
const BAND_NAMES: Array[String] = ["Red", "Orange", "Yellow", "Green", "Blue", "Purple"]
const HEART_COLORS := {
	"heart": Color("#EC70A8"),
	"heart_blue": Color("#5DADE2"),
	"heart_gold": Color("#FFD94D"),
}
const GAME_ID := "sample-rainbow-bear"

var bobo: Bobo
var rainbow: Rainbow
var kitten: Sprite2D
var hearts: Array[Heart] = []
var heart_tex := {}

var caught := 0
var misses := 0
var score := 0
var combo := 1
var level := 1
var meta_level := 1
var meta_mult := 1.0
var elapsed := 0.0
var spawn_t := 0.0
var over := false

var camera: Camera2D
var trauma := 0.0
var hud := {}
var clouds: Array[Node2D] = []

var music_player: AudioStreamPlayer
var sfx_pool: Array[AudioStreamPlayer] = []
var sfx_idx := 0
var sfx_streams := {}

var banner_tween: Tween

var self_test := false
var test_elapsed := 0.0
var api_base := ""


func _ready() -> void:
	Engine.time_scale = 1.0
	var args := OS.get_cmdline_args() + OS.get_cmdline_user_args()
	self_test = "--self-test" in args
	for a in args:
		if a.begins_with("--api="):
			api_base = a.trim_prefix("--api=")
	if self_test:
		Engine.time_scale = 30.0
	randomize()
	heart_tex["heart"] = HeartTex.make(HEART_COLORS["heart"])
	heart_tex["heart_blue"] = HeartTex.make(HEART_COLORS["heart_blue"])
	heart_tex["heart_gold"] = HeartTex.make(HEART_COLORS["heart_gold"], 56)
	_build_world()
	_build_hud()
	if not self_test:
		_build_audio()
	if self_test:
		print("[self-test] bobo start")


func _build_world() -> void:
	RenderingServer.set_default_clear_color(Color("#8ecae6"))
	# Sky gradient.
	var grad := Gradient.new()
	grad.set_color(0, Color("#7cc4e8"))
	grad.set_color(1, Color("#cdeef7"))
	var gtex := GradientTexture2D.new()
	gtex.gradient = grad
	gtex.fill_from = Vector2(0.5, 0.0)
	gtex.fill_to = Vector2(0.5, 1.0)
	gtex.width = 390
	gtex.height = 844
	var sky := TextureRect.new()
	sky.texture = gtex
	sky.set_anchors_preset(Control.PRESET_FULL_RECT)
	add_child(sky)
	# Drifting clouds (soft white ellipses).
	for i in range(4):
		var cloud := Polygon2D.new()
		var pts := PackedVector2Array()
		for k in range(20):
			var a := TAU * float(k) / 20.0
			pts.append(Vector2(cos(a) * (46.0 + 14.0 * sin(a * 2.0)), sin(a) * 18.0))
		cloud.polygon = pts
		cloud.color = Color(1, 1, 1, 0.85)
		cloud.position = Vector2(randf_range(0, 390), randf_range(60, 320))
		clouds.append(cloud)
		add_child(cloud)
	# Meadow.
	var meadow := Polygon2D.new()
	meadow.polygon = PackedVector2Array([Vector2(0, 790), Vector2(390, 790), Vector2(390, 844), Vector2(0, 844)])
	meadow.color = Color("#7ccf8d")
	add_child(meadow)
	# Rainbow arch + kitten underneath.
	rainbow = Rainbow.new()
	rainbow.position = Vector2(195, 640)
	add_child(rainbow)
	kitten = Sprite2D.new()
	kitten.texture = load("res://assets/kitten.png")
	var kts: Vector2 = kitten.texture.get_size()
	kitten.scale = Vector2(76.0 / kts.x, 88.0 / kts.y)
	kitten.position = Vector2(195, 640)
	add_child(kitten)
	var ktw := create_tween().set_loops()
	ktw.tween_property(kitten, "position:y", 632.0, 1.2).set_trans(Tween.TRANS_SINE)
	ktw.tween_property(kitten, "position:y", 648.0, 1.2).set_trans(Tween.TRANS_SINE)
	# Bobo.
	bobo = Bobo.new()
	bobo.position = Vector2(195, 740)
	add_child(bobo)
	# Camera for shake.
	camera = Camera2D.new()
	camera.position = Vector2(195, 422)
	add_child(camera)
	camera.make_current()


func _hud_label(key: String, text: String, x: float, y: float, size: int, color: Color, layer: CanvasLayer) -> Label:
	var l := Label.new()
	l.text = text
	l.position = Vector2(x, y)
	l.add_theme_font_size_override("font_size", size)
	l.add_theme_color_override("font_color", color)
	l.add_theme_color_override("font_shadow_color", Color(1, 1, 1, 0.7))
	l.add_theme_constant_override("shadow_offset_x", 1)
	l.add_theme_constant_override("shadow_offset_y", 1)
	layer.add_child(l)
	hud[key] = l
	return l


func _build_hud() -> void:
	var layer := CanvasLayer.new()
	layer.layer = 5
	add_child(layer)
	_hud_label("title", "Bobo's Rainbow", 16, 14, 24, Color("#123046"), layer)
	_hud_label("meta", "LV 1", 216, 14, 24, Color("#1e7b3a"), layer)
	_hud_label("score", "Score: 0", 16, 46, 26, Color("#123046"), layer)
	_hud_label("combo", "", 16, 78, 20, Color("#b3402e"), layer)
	_hud_label("lives", "❤❤❤", 296, 14, 24, Color("#d33a4e"), layer)
	var banner := _hud_label("banner", "", 0, 108, 22, Color("#b3402e"), layer)
	banner.size = Vector2(390, 40)
	banner.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	_hud_label("hint", "Tap to move Bobo - catch the hearts!", 16, 150, 18, Color("#123046"), layer)


func _build_audio() -> void:
	music_player = AudioStreamPlayer.new()
	music_player.stream = Synth.calm_loop()
	music_player.volume_db = -10.0
	add_child(music_player)
	music_player.play()
	sfx_streams["catch"] = Synth.tone(520.0, 0.12)
	sfx_streams["golden"] = Synth.tone(1200.0, 0.2)
	sfx_streams["miss"] = Synth.tone(180.0, 0.2)
	sfx_streams["levelup"] = Synth.tone(770.0, 0.25)
	sfx_streams["win"] = Synth.tone(1047.0, 0.3)
	sfx_streams["lose"] = Synth.tone(220.0, 0.5)
	for i in range(8):
		var p := AudioStreamPlayer.new()
		add_child(p)
		sfx_pool.append(p)


func _sfx(name: String, pitch := 1.0) -> void:
	if self_test or not sfx_streams.has(name):
		return
	var p := sfx_pool[sfx_idx]
	sfx_idx = (sfx_idx + 1) % sfx_pool.size()
	p.stream = sfx_streams[name]
	p.pitch_scale = pitch
	p.play()


func _banner(msg: String, color := Color("#b3402e")) -> void:
	var l: Label = hud["banner"]
	l.text = msg
	l.add_theme_color_override("font_color", color)
	if banner_tween and banner_tween.is_valid():
		banner_tween.kill()
	l.scale = Vector2.ONE
	l.modulate.a = 1.0
	l.pivot_offset = Vector2(195, 20)
	banner_tween = create_tween()
	banner_tween.tween_property(l, "scale", Vector2(1.25, 1.25), 0.15)
	banner_tween.tween_property(l, "scale", Vector2.ONE, 0.2)
	banner_tween.tween_interval(1.3)
	banner_tween.tween_property(l, "modulate:a", 0.0, 0.4)


func _burst(pos: Vector2, color: Color, amount := 14, spread := 180.0) -> void:
	var p := CPUParticles2D.new()
	p.amount = amount
	p.lifetime = 0.65
	p.one_shot = true
	p.explosiveness = 0.92
	p.direction = Vector2(0, -1)
	p.spread = spread
	p.gravity = Vector2(0, 520)
	p.initial_velocity_min = 120.0
	p.initial_velocity_max = 280.0
	p.scale_amount_min = 2.0
	p.scale_amount_max = 4.5
	p.color = color
	p.position = pos
	add_child(p)
	p.emitting = true
	get_tree().create_timer(1.4).timeout.connect(p.queue_free)


func _unhandled_input(event: InputEvent) -> void:
	if over or self_test:
		return
	var pos := Vector2.INF
	if event is InputEventScreenTouch and event.pressed:
		pos = event.position
	elif event is InputEventMouseButton and event.pressed and event.button_index == MOUSE_BUTTON_LEFT:
		pos = event.position
	if pos.x < INF:
		bobo.target_x = clampf(pos.x, 52.0, 338.0)


func _spawn_heart() -> void:
	var gold := randf() < 0.12
	var kind := "heart_gold" if gold else ("heart" if randf() < 0.5 else "heart_blue")
	var speed := 175.0 * pow(1.15, level - 1) * meta_mult
	var heart := Heart.new()
	add_child(heart)
	heart.setup(heart_tex[kind], gold, speed, Vector2(30.0 + randf() * 330.0, -24.0))
	heart.area_entered.connect(_on_heart_entered.bind(heart))
	heart.missed.connect(_on_heart_missed.bind(heart))
	hearts.append(heart)


func _on_heart_entered(area: Area2D, heart: Heart) -> void:
	if over or heart.done or area != bobo:
		return
	heart.done = true
	hearts.erase(heart)
	var pos := heart.position
	heart.queue_free()
	if heart.gold:
		score += 50
		_banner("GOLDEN HEART +50!", Color("#b8860b"))
		_sfx("golden")
		_burst(pos, Color("#FFD94D"), 22)
		FloatLabel.spawn(self, pos, "+50", Color("#b8860b"))
	else:
		var gain := 10 * combo
		score += gain
		caught += 1
		_sfx("catch", 1.0 + combo * 0.09)
		_burst(pos, Color("#EC70A8"))
		FloatLabel.spawn(self, pos, "+" + str(gain))
		bobo.squash()
		if caught % 2 == 0:
			_restore_band()
		combo = mini(MAX_COMBO, combo + 1)
	_update_hud()
	if caught >= HEARTS_TO_WIN:
		_next_round()


func _on_heart_missed(heart: Heart) -> void:
	hearts.erase(heart)
	heart.queue_free()
	if over or heart.gold:
		# Golden hearts may be ignored without penalty.
		_update_hud()
		return
	misses += 1
	combo = 1
	trauma = 0.55
	_sfx("miss")
	_burst(Vector2(heart.position.x, 810), Color(1, 1, 1, 0.8), 8, 60.0)
	_update_hud()
	if misses >= MAX_MISSES:
		_end_game()


func _restore_band() -> void:
	var idx := mini(5, caught / 2 - 1)
	rainbow.reveal_band(idx)
	var nl := mini(6, caught / 2 + 1)
	var msg := BAND_NAMES[idx] + " restored!"
	if nl > level:
		level = nl
		msg = "LEVEL " + str(level) + "! " + msg
		_sfx("levelup", 1.0 + level * 0.06)
	_banner(msg)


func _next_round() -> void:
	meta_level += 1
	meta_mult *= 1.25
	caught = 0
	misses = 0
	combo = 1
	level = 1
	spawn_t = 0.0
	for h in hearts:
		h.queue_free()
	hearts.clear()
	rainbow.reset()
	_update_hud()
	_banner("LV " + str(meta_level) + "! Faster!", Color("#1e7b3a"))
	_sfx("win")
	_burst(Vector2(195, 400), Color("#FFD94D"), 30)


func _end_game() -> void:
	over = true
	var n := mini(3, meta_level)
	_sfx("lose")
	_banner("Reached LV " + str(meta_level) + "!", Color("#b3402e"))
	var stars := ""
	for i in range(3):
		stars += "★" if i < n else "☆"
	_show_end_panel(stars)
	_post_score()


func _show_end_panel(stars: String) -> void:
	var layer := CanvasLayer.new()
	layer.layer = 10
	add_child(layer)
	var dim := ColorRect.new()
	dim.color = Color(0, 0, 0, 0.45)
	dim.set_anchors_preset(Control.PRESET_FULL_RECT)
	layer.add_child(dim)
	var center := CenterContainer.new()
	center.set_anchors_preset(Control.PRESET_FULL_RECT)
	layer.add_child(center)
	var panel := PanelContainer.new()
	center.add_child(panel)
	var vb := VBoxContainer.new()
	vb.add_theme_constant_override("separation", 10)
	panel.add_child(vb)
	var t := Label.new()
	t.text = stars + "\nReached LV " + str(meta_level) + "!\nScore: " + str(score)
	t.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	t.add_theme_font_size_override("font_size", 26)
	vb.add_child(t)
	var btn := Button.new()
	btn.text = "Play Again"
	btn.custom_minimum_size = Vector2(200, 56)
	btn.pressed.connect(func() -> void: get_tree().reload_current_scene())
	vb.add_child(btn)
	panel.scale = Vector2(0.7, 0.7)
	panel.pivot_offset = panel.size / 2.0
	var tw := create_tween()
	tw.tween_property(panel, "scale", Vector2.ONE, 0.3).set_trans(Tween.TRANS_BACK).set_ease(Tween.EASE_OUT)


func _post_score() -> void:
	if api_base == "":
		return
	var http := HTTPRequest.new()
	add_child(http)
	var body := JSON.stringify({"name": "GodotPlayer", "score": score, "level": meta_level})
	http.request(api_base + "/v1/games/" + GAME_ID + "/scores",
		["Content-Type: application/json"], HTTPClient.METHOD_POST, body)


func _update_hud() -> void:
	(hud["score"] as Label).text = "Score: " + str(score)
	(hud["combo"] as Label).text = "COMBO x" + str(combo) + "!" if combo > 1 else ""
	var s := ""
	for i in range(MAX_MISSES - misses):
		s += "❤"
	(hud["lives"] as Label).text = s
	(hud["meta"] as Label).text = "LV " + str(meta_level)


func _physics_process(delta: float) -> void:
	if over:
		return
	elapsed += delta
	# Drifting clouds.
	for c in clouds:
		c.position.x += 12.0 * delta
		if c.position.x > 440.0:
			c.position.x = -50.0
	# Heart spawn pacing: round ramp x meta multiplier.
	var interval := maxf(0.35, 0.8 / pow(1.12, level - 1) / meta_mult)
	if caught < HEARTS_TO_WIN:
		spawn_t += delta
		if spawn_t >= interval:
			spawn_t = 0.0
			_spawn_heart()
	# Camera shake decay.
	if trauma > 0.0:
		trauma = maxf(0.0, trauma - delta * 1.7)
		camera.offset = Vector2(randf_range(-1.0, 1.0), randf_range(-1.0, 1.0)) * trauma * 22.0
	elif camera.offset != Vector2.ZERO:
		camera.offset = Vector2.ZERO
	# Self-test: perfect bot + assertions.
	if self_test:
		_test_bot()
		test_elapsed += delta
		if meta_level >= 2:
			print("[self-test] PASS survived to LV 2, score=", score, " caught_total_ok")
			get_tree().quit()
		elif test_elapsed > 150.0:
			print("[self-test] FAIL timeout at LV ", meta_level, " score=", score)
			get_tree().quit()


func _test_bot() -> void:
	var best: Heart = null
	var best_y := -1.0
	for h in hearts:
		if not h.done and h.position.y > best_y and h.position.y < 700.0:
			best_y = h.position.y
			best = h
	if best:
		bobo.target_x = clampf(best.position.x, 52.0, 338.0)
