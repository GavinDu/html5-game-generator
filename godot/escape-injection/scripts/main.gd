extends Node2D
## Escape from the Injection — Godot rework.
## Tap LEFT/RIGHT to switch lanes. Dodge syringes, grab courage stars, and keep
## ahead of the chaser — the visible gap IS your health. Every 40 seconds you
## escape somewhere new, 1.12x faster. Caught? A shot in the butt... with tears.
## Same rules as the GameShim original, rebuilt with parallax scrolling,
## tweened lane changes, a chaser that visibly looms closer, speed lines,
## camera shake and procedural audio.

const LANES: Array[float] = [90.0, 195.0, 300.0]
const LEVEL_SECS := 40.0
const LEVEL_NAMES: Array[String] = ["Clinic", "Hallway", "Playground", "Park", "Beach", "Mall"]
const GAP_START := 220.0
const GAP_MAX := 250.0
const GAP_CAUGHT := 55.0
const GAME_ID := "escape-from-injection"

var blob: EscapeBlob
var chaser: Sprite2D
var chaser_base_scale := Vector2.ONE
var chaser_bob := 0.0
var syringes: Array[FallingSyringe] = []
var stars: Array[CourageStar] = []

var bg_sprite: Sprite2D
var dots_layer: Node2D
var dash_layer: Node2D

var gap := GAP_START
var score := 0.0
var elapsed := 0.0
var level_t := 0.0
var spawn_t := 0.0
var star_t := 3.0
var hint_t := 0.0
var meta_level := 1
var meta_mult := 1.0
var over := false
var ended := false
var catch_t := 0.0
var cried := false

var camera: Camera2D
var trauma := 0.0
var hud := {}
var flash: ColorRect
var speed_lines: Array[CPUParticles2D] = []
var banner_tween: Tween

var music_player: AudioStreamPlayer
var sfx_pool: Array[AudioStreamPlayer] = []
var sfx_idx := 0
var sfx_streams := {}

var self_test := false
var test_elapsed := 0.0
var test_phase := 0  # 0 = dodge to LV 2, 1 = idle until caught
var api_base := ""


func _ready() -> void:
	Engine.time_scale = 1.0
	var args := OS.get_cmdline_args() + OS.get_cmdline_user_args()
	self_test = "--self-test" in args
	for a in args:
		if a.begins_with("--api="):
			api_base = a.trim_prefix("--api=")
	if self_test:
		Engine.time_scale = 8.0
	randomize()
	_build_world()
	_build_hud()
	if not self_test:
		_build_audio()
	if self_test:
		print("[self-test] escape start")


func _bg_tex(i: int) -> Texture2D:
	return load("res://assets/bg%d.png" % i)


func _build_world() -> void:
	RenderingServer.set_default_clear_color(Color("#d8f3e3"))
	# Base backdrop (swapped per level with a crossfade).
	bg_sprite = Sprite2D.new()
	bg_sprite.texture = _bg_tex(0)
	bg_sprite.position = Vector2(195, 422)
	add_child(bg_sprite)
	# Parallax layer 1: soft dots drifting slowly.
	dots_layer = Node2D.new()
	add_child(dots_layer)
	for i in range(26):
		var d := Polygon2D.new()
		var pts := PackedVector2Array()
		var r := randf_range(5.0, 13.0)
		for k in range(12):
			var a := TAU * float(k) / 12.0
			pts.append(Vector2(cos(a) * r, sin(a) * r))
		d.polygon = pts
		d.color = Color(1, 1, 1, 0.5)
		d.position = Vector2(randf_range(0, 390), randf_range(0, 1688))
		dots_layer.add_child(d)
	# Parallax layer 2: lane dashes rushing faster.
	dash_layer = Node2D.new()
	add_child(dash_layer)
	for lx in [142.0, 247.0]:
		var y := 0.0
		while y < 1688.0:
			var dash := ColorRect.new()
			dash.color = Color(1, 1, 1, 0.55)
			dash.size = Vector2(5, 26)
			dash.position = Vector2(lx - 2.5, y)
			dash_layer.add_child(dash)
			y += 52.0
	# Blob + chaser.
	blob = EscapeBlob.new()
	blob.position = Vector2(195, 680)
	add_child(blob)
	chaser = Sprite2D.new()
	chaser.texture = load("res://assets/chaser.png")
	var cts: Vector2 = chaser.texture.get_size()
	chaser_base_scale = Vector2(105.0 / cts.x, 122.0 / cts.y)
	chaser.scale = chaser_base_scale
	chaser.position = Vector2(195, 680 + gap)
	add_child(chaser)
	# Speed lines at the screen edges.
	for sx in [24.0, 366.0]:
		var sl := CPUParticles2D.new()
		sl.amount = 12
		sl.lifetime = 0.5
		sl.emission_shape = CPUParticles2D.EMISSION_SHAPE_RECTANGLE
		sl.emission_rect_extents = Vector2(8, 400)
		sl.direction = Vector2(0, 1)
		sl.spread = 10.0
		sl.gravity = Vector2.ZERO
		sl.initial_velocity_min = 500.0
		sl.initial_velocity_max = 900.0
		sl.scale_amount_min = 1.5
		sl.scale_amount_max = 3.0
		sl.color = Color(1, 1, 1, 0.45)
		sl.position = Vector2(sx, 422)
		add_child(sl)
		sl.emitting = true
		speed_lines.append(sl)
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
	_hud_label("title", "Escape from the Injection", 16, 14, 22, Color("#123046"), layer)
	_hud_label("meta", "LV 1 - Clinic", 16, 44, 20, Color("#1e7b3a"), layer)
	_hud_label("score", "Score: 0", 280, 14, 22, Color("#123046"), layer)
	_hud_label("hint", "Tap < left / right > to dodge!\nTip: stay in the middle lane.", 16, 76, 17, Color("#123046"), layer)
	var banner := _hud_label("banner", "", 0, 140, 26, Color("#b3402e"), layer)
	banner.size = Vector2(390, 80)
	banner.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	banner.autowrap_mode = TextServer.AUTOWRAP_WORD_SMART
	# Red hit-flash overlay.
	var flash_layer := CanvasLayer.new()
	flash_layer.layer = 8
	add_child(flash_layer)
	flash = ColorRect.new()
	flash.color = Color(0.85, 0.15, 0.2, 0.0)
	flash.set_anchors_preset(Control.PRESET_FULL_RECT)
	flash.mouse_filter = Control.MOUSE_FILTER_IGNORE
	flash_layer.add_child(flash)


func _build_audio() -> void:
	music_player = AudioStreamPlayer.new()
	music_player.stream = Synth.adventurous_loop()
	music_player.volume_db = -11.0
	add_child(music_player)
	music_player.play()
	sfx_streams["swish"] = Synth.tone(560.0, 0.06, 0.3)
	sfx_streams["star"] = Synth.tone(990.0, 0.12)
	sfx_streams["hit"] = Synth.tone(150.0, 0.2)
	sfx_streams["levelup"] = Synth.tone(880.0, 0.25)
	sfx_streams["caught"] = Synth.tone(180.0, 0.4)
	sfx_streams["cry"] = Synth.tone(220.0, 0.5)
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


func _banner(msg: String, hold := false) -> void:
	var l: Label = hud["banner"]
	l.text = msg
	if banner_tween and banner_tween.is_valid():
		banner_tween.kill()
	l.scale = Vector2.ONE
	l.modulate.a = 1.0
	l.pivot_offset = Vector2(195, 30)
	banner_tween = create_tween()
	banner_tween.tween_property(l, "scale", Vector2(1.18, 1.18), 0.16)
	banner_tween.tween_property(l, "scale", Vector2.ONE, 0.22)
	if not hold:
		banner_tween.tween_interval(2.2)
		banner_tween.tween_property(l, "modulate:a", 0.0, 0.4)


func _burst(pos: Vector2, color: Color, amount := 12) -> void:
	var p := CPUParticles2D.new()
	p.amount = amount
	p.lifetime = 0.6
	p.one_shot = true
	p.explosiveness = 0.9
	p.direction = Vector2(0, -1)
	p.spread = 180.0
	p.gravity = Vector2(0, 420)
	p.initial_velocity_min = 110.0
	p.initial_velocity_max = 250.0
	p.scale_amount_min = 2.0
	p.scale_amount_max = 4.0
	p.color = color
	p.position = pos
	add_child(p)
	p.emitting = true
	get_tree().create_timer(1.2).timeout.connect(p.queue_free)


func _level_name(n: int) -> String:
	if n <= LEVEL_NAMES.size():
		return LEVEL_NAMES[n - 1]
	return "Nowhere Left!"


func _set_bg(idx: int) -> void:
	var next := Sprite2D.new()
	next.texture = _bg_tex(idx)
	next.position = Vector2(195, 422)
	next.modulate.a = 0.0
	add_child(next)
	move_child(next, bg_sprite.get_index())
	var tw := create_tween()
	tw.tween_property(next, "modulate:a", 1.0, 0.8)
	tw.tween_callback(bg_sprite.queue_free)
	bg_sprite = next


func _handle_tap(x: float) -> void:
	if over:
		return
	if x < 195.0:
		blob.move_lane(-1)
	else:
		blob.move_lane(1)
	_sfx("swish")


func _unhandled_input(event: InputEvent) -> void:
	if over or self_test:
		return
	if event is InputEventScreenTouch and event.pressed:
		_handle_tap(event.position.x)
	elif event is InputEventMouseButton and event.pressed and event.button_index == MOUSE_BUTTON_LEFT:
		_handle_tap(event.position.x)


func _spawn_syringe() -> void:
	# Fairness: never an unavoidable 3-lane wall — stack into lanes whose
	# syringes are arriving together.
	var young := {}
	for s in syringes:
		if is_instance_valid(s) and not s.done and elapsed - s.born < 0.6:
			young[s.lane] = true
	var keys := young.keys()
	var l := int(keys[randi() % keys.size()]) if keys.size() >= 2 else randi() % 3
	var speed := 210.0 * minf(meta_mult, 3.2)
	var sy := FallingSyringe.new()
	add_child(sy)
	sy.setup(speed, elapsed, LANES[l])
	sy.lane = l
	sy.hit.connect(_on_syringe_hit)
	syringes.append(sy)


func _on_syringe_hit(sy: FallingSyringe) -> void:
	if over or ended:
		return
	syringes.erase(sy)
	var pos := sy.position
	sy.queue_free()
	gap -= 45.0
	trauma = 0.6
	_sfx("hit")
	_burst(pos, Color("#7db9e8"), 10)
	FloatLabel.spawn(self, pos, "-45", Color("#b3402e"))
	var tw := create_tween()
	flash.color.a = 0.32
	tw.tween_property(flash, "color:a", 0.0, 0.35)
	_update_hud()


func _spawn_star() -> void:
	var speed := 210.0 * minf(meta_mult, 3.2)
	var st := CourageStar.new()
	add_child(st)
	st.setup(speed, LANES[randi() % 3])
	st.collected.connect(_on_star_collected)
	stars.append(st)


func _on_star_collected(st: CourageStar) -> void:
	if over or ended:
		return
	stars.erase(st)
	var pos := st.position
	st.queue_free()
	score += 25.0
	gap = minf(GAP_MAX, gap + 12.0)
	_sfx("star")
	_burst(pos, Color("#FFD94D"), 12)
	FloatLabel.spawn(self, pos, "+25", Color("#b8860b"))
	_update_hud()


func _sweep(arr: Array) -> void:
	for i in range(arr.size() - 1, -1, -1):
		var e = arr[i]
		if not is_instance_valid(e) or e.done:
			arr.remove_at(i)


func _update_hud() -> void:
	(hud["score"] as Label).text = "Score: " + str(int(score))
	(hud["meta"] as Label).text = "LV " + str(meta_level) + " - " + _level_name(meta_level)


func _end_game() -> void:
	ended = true
	_sfx("cry")
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
	t.text = "(;_;) Injected in the butt...\nReached Level " + str(meta_level) + "!\nScore: " + str(int(score))
	t.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	t.add_theme_font_size_override("font_size", 24)
	vb.add_child(t)
	var btn := Button.new()
	btn.text = "Play Again"
	btn.custom_minimum_size = Vector2(200, 56)
	btn.pressed.connect(func() -> void: get_tree().reload_current_scene())
	vb.add_child(btn)
	panel.scale = Vector2(0.7, 0.7)
	var tw := create_tween()
	tw.tween_property(panel, "scale", Vector2.ONE, 0.3).set_trans(Tween.TRANS_BACK).set_ease(Tween.EASE_OUT)
	_post_score()


func _post_score() -> void:
	if api_base == "":
		return
	var http := HTTPRequest.new()
	add_child(http)
	var body := JSON.stringify({"name": "GodotPlayer", "score": int(score), "level": meta_level})
	http.request(api_base + "/v1/games/" + GAME_ID + "/scores",
		["Content-Type: application/json"], HTTPClient.METHOD_POST, body)


func _process(delta: float) -> void:
	# Self-test tick runs first so phase transitions are observed even
	# during the caught sequence / end panel.
	if self_test:
		_test_tick(delta)
	if ended:
		return
	# Caught sequence.
	if over:
		catch_t += delta
		gap = maxf(0.0, gap - 420.0 * delta)
		chaser.position.x = lerpf(chaser.position.x, blob.position.x, minf(1.0, delta * 10.0))
		chaser.position.y = 680.0 + gap
		if catch_t > 0.55 and not cried:
			cried = true
			_banner("(;_;) Injected in the butt...", true)
			_sfx("cry")
		if catch_t > 1.7:
			_end_game()
		return
	elapsed += delta
	score += delta * 10.0
	if int(score) % 10 == 0:
		_update_hud()
	if hint_t < 4.0:
		hint_t += delta
		if hint_t >= 4.0:
			(hud["hint"] as Label).text = ""
	# Level up: escape somewhere new, quicker.
	level_t += delta
	if level_t >= LEVEL_SECS:
		level_t = 0.0
		meta_level += 1
		meta_mult *= 1.12
		_set_bg((meta_level - 1) % 3)
		_update_hud()
		_banner("LEVEL " + str(meta_level) + "! Faster! " + _level_name(meta_level))
		_sfx("levelup")
	# Chaser trails the blob, looming larger as the gap shrinks.
	chaser_bob += delta * 6.0
	chaser.position.x = lerpf(chaser.position.x, blob.position.x, minf(1.0, delta * 5.0))
	chaser.position.y = 680.0 + gap + sin(chaser_bob) * 6.0
	var loom := 1.0 + (GAP_MAX - gap) / GAP_MAX * 0.55
	chaser.scale = chaser_base_scale * loom
	# Parallax scroll + speed lines scale with pace.
	var capped := minf(meta_mult, 3.2)
	var speed := 210.0 * capped
	_scroll_layer(dots_layer, speed * 0.22 * delta)
	_scroll_layer(dash_layer, speed * 0.65 * delta)
	for sl in speed_lines:
		sl.amount = int(6.0 + capped * 8.0)
	# Spawns.
	spawn_t += delta
	if spawn_t >= maxf(0.55, 1.3 / capped):
		spawn_t = 0.0
		_spawn_syringe()
	star_t += delta
	if star_t >= 2.5:
		star_t = 0.0
		_spawn_star()
	_sweep(syringes)
	_sweep(stars)
	# The gap slowly recovers while running clean.
	gap = minf(GAP_MAX, gap + 6.0 * delta)
	# Caught? The plot says he is finally caught.
	if gap <= GAP_CAUGHT and not over:
		over = true
		catch_t = 0.0
		_banner("CAUGHT!", true)
		_sfx("caught")
	# Camera shake decay.
	if trauma > 0.0:
		trauma = maxf(0.0, trauma - delta * 1.7)
		camera.offset = Vector2(randf_range(-1.0, 1.0), randf_range(-1.0, 1.0)) * trauma * 20.0
	elif camera.offset != Vector2.ZERO:
		camera.offset = Vector2.ZERO


func _scroll_layer(layer: Node2D, dy: float) -> void:
	for c in layer.get_children():
		c.position.y += dy
		if c.position.y >= 1688.0:
			c.position.y -= 1688.0


func _physics_process(_delta: float) -> void:
	pass


func _lane_of(x: float) -> int:
	var best := 0
	var bd := 1e9
	for i in range(3):
		var d := absf(LANES[i] - x)
		if d < bd:
			bd = d
			best = i
	return best


func _test_tick(delta: float) -> void:
	test_elapsed += delta
	if test_phase == 0:
		# Dodge bot: avoid lanes with syringes about 1.4s out.
		var danger := [false, false, false]
		for s in syringes:
			if is_instance_valid(s) and not s.done and s.position.y > 380.0 and s.position.y < 700.0:
				danger[_lane_of(s.position.x)] = true
		var target := blob.lane
		if danger[blob.lane]:
			var opts: Array[int] = []
			for l in [1, 0, 2]:
				if not danger[l]:
					opts.append(l)
			if opts.size() > 0:
				target = opts[0]
		if target < blob.lane:
			_handle_tap(50.0)
		elif target > blob.lane:
			_handle_tap(340.0)
		if meta_level >= 2:
			print("[self-test] phase 0 PASS reached LV 2, score=", int(score))
			test_phase = 1
			test_elapsed = 0.0
		elif test_elapsed > 150.0:
			print("[self-test] FAIL dodge timeout at LV ", meta_level)
			get_tree().quit()
	elif test_phase == 1:
		# Idle: the chaser must eventually catch the blob.
		if over and ended:
			print("[self-test] PASS caught at LV ", meta_level, " score=", int(score))
			get_tree().quit()
		elif test_elapsed > 200.0:
			print("[self-test] FAIL idle never caught")
			get_tree().quit()
