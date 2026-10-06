extends Node2D
## 飘带补天 Ribbon Mends the Sky — Godot rework.
## Fly into dark sky-holes to stitch them shut (6 = next 关, 1.25x faster).
## Bonk wind demons by tapping them; 3 hits end the run.
## Same rules as the GameShim original, rebuilt with physics areas,
## particles, tweens, camera shake and procedural audio.

const HOLES_TO_WIN := 6
const MAX_LIVES := 3
const MAX_COMBO := 5
const GAME_ID := "sample-ribbon-sky"

var hero: SkyHero
var holes: Array[SkyHole] = []
var demons: Array[WindDemon] = []
var shards: Array[SkyShard] = []

var score := 0
var stitched := 0
var combo := 1
var lives := MAX_LIVES
var shards_got := 0
var elapsed := 0.0
var holes_spawned := 0
var spawn_hole_t := 0.0
var spawn_demon_t := 3.0
var spawn_shard_t := 1.5
var over := false
var win_t := -1.0
var win_tone_i := 0
var hit_t := 0.0
var level := 1
var meta_level := 1
var meta_mult := 1.0

var camera: Camera2D
var trauma := 0.0
var hud := {}
var banner_tween: Tween
var clouds: Array[Sprite2D] = []

var music_player: AudioStreamPlayer
var sfx_pool: Array[AudioStreamPlayer] = []
var sfx_idx := 0
var sfx_streams := {}

var self_test := false
var test_elapsed := 0.0
var test_bonk_t := 0.0
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
	_build_world()
	_build_hud()
	if not self_test:
		_build_audio()
	if self_test:
		print("[self-test] sky start")


func _build_world() -> void:
	RenderingServer.set_default_clear_color(Color("#3a4a7a"))
	var bg := TextureRect.new()
	bg.texture = load("res://assets/bg.jpg")
	bg.set_anchors_preset(Control.PRESET_FULL_RECT)
	bg.expand_mode = TextureRect.EXPAND_IGNORE_SIZE
	bg.stretch_mode = TextureRect.STRETCH_SCALE
	add_child(bg)
	# Twinkling stars.
	var stars := CPUParticles2D.new()
	stars.amount = 48
	stars.lifetime = 3.0
	stars.preprocess = 3.0
	stars.emission_shape = CPUParticles2D.EMISSION_SHAPE_RECTANGLE
	stars.emission_rect_extents = Vector2(195, 300)
	stars.direction = Vector2(0, 0)
	stars.spread = 0.0
	stars.gravity = Vector2.ZERO
	stars.initial_velocity_min = 0.0
	stars.initial_velocity_max = 0.0
	stars.scale_amount_min = 1.0
	stars.scale_amount_max = 2.6
	stars.color = Color(1, 1, 1, 0.75)
	stars.position = Vector2(195, 260)
	add_child(stars)
	stars.emitting = true
	# Drifting clouds (hero sprite tinted white, low alpha).
	for i in range(3):
		var c := Sprite2D.new()
		c.texture = load("res://assets/hero.png")
		c.modulate = Color(1, 1, 1, 0.35)
		c.scale = Vector2(0.9, 0.5)
		c.position = Vector2(randf_range(0, 390), randf_range(120, 500))
		clouds.append(c)
		add_child(c)
	hero = SkyHero.new()
	hero.position = Vector2(195, 660)
	add_child(hero)
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
	l.add_theme_color_override("font_shadow_color", Color(0, 0, 0, 0.6))
	l.add_theme_constant_override("shadow_offset_x", 1)
	l.add_theme_constant_override("shadow_offset_y", 1)
	layer.add_child(l)
	hud[key] = l
	return l


func _build_hud() -> void:
	var layer := CanvasLayer.new()
	layer.layer = 5
	add_child(layer)
	_hud_label("title", "飘带补天", 16, 12, 30, Color.WHITE, layer)
	_hud_label("meta", "第1关", 150, 12, 24, Color("#ffe27a"), layer)
	_hud_label("score", "🌈 0/6  ⭐ 0", 16, 48, 24, Color.WHITE, layer)
	_hud_label("combo", "", 16, 80, 20, Color("#ffe27a"), layer)
	_hud_label("lives", "❤❤❤", 282, 12, 24, Color("#ff6b81"), layer)
	var banner := _hud_label("banner", "", 0, 108, 22, Color("#ffe27a"), layer)
	banner.size = Vector2(390, 40)
	banner.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	_hud_label("hint1", "👆 点哪里，云朵就飞到哪里", 16, 796, 17, Color.WHITE, layer)
	_hud_label("hint2", "🎀 钻过黑洞缝补天空  🔨 点风妖敲它", 16, 818, 17, Color.WHITE, layer)


func _build_audio() -> void:
	music_player = AudioStreamPlayer.new()
	music_player.stream = Synth.playful_loop()
	music_player.volume_db = -12.0
	add_child(music_player)
	music_player.play()
	sfx_streams["stitch"] = Synth.tone(520.0, 0.15)
	sfx_streams["bonk"] = Synth.tone(200.0, 0.15)
	sfx_streams["hit"] = Synth.tone(150.0, 0.3)
	sfx_streams["shard"] = Synth.tone(740.0, 0.12)
	sfx_streams["wave"] = Synth.tone(820.0, 0.25)
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


func _banner(msg: String, color := Color("#ffe27a")) -> void:
	var l: Label = hud["banner"]
	l.text = msg
	l.add_theme_color_override("font_color", color)
	if banner_tween and banner_tween.is_valid():
		banner_tween.kill()
	l.scale = Vector2.ONE
	l.modulate.a = 1.0
	l.pivot_offset = Vector2(195, 20)
	banner_tween = create_tween()
	banner_tween.tween_property(l, "scale", Vector2(1.2, 1.2), 0.15)
	banner_tween.tween_property(l, "scale", Vector2.ONE, 0.2)
	banner_tween.tween_interval(1.4)
	banner_tween.tween_property(l, "modulate:a", 0.0, 0.4)


func _burst(pos: Vector2, color: Color, amount := 14) -> void:
	var p := CPUParticles2D.new()
	p.amount = amount
	p.lifetime = 0.6
	p.one_shot = true
	p.explosiveness = 0.9
	p.direction = Vector2(0, -1)
	p.spread = 180.0
	p.gravity = Vector2(0, 260)
	p.initial_velocity_min = 90.0
	p.initial_velocity_max = 240.0
	p.scale_amount_min = 2.0
	p.scale_amount_max = 4.0
	p.color = color
	p.position = pos
	add_child(p)
	p.emitting = true
	get_tree().create_timer(1.2).timeout.connect(p.queue_free)


func _thread_flash(a: Vector2, b: Vector2) -> void:
	var line := Line2D.new()
	line.points = PackedVector2Array([a, b])
	line.width = 7.0
	line.default_color = Color("#ffb3d9")
	line.begin_cap_mode = Line2D.LINE_CAP_ROUND
	line.end_cap_mode = Line2D.LINE_CAP_ROUND
	add_child(line)
	var tw := create_tween()
	tw.tween_property(line, "modulate:a", 0.0, 0.35)
	tw.tween_callback(line.queue_free)


func _handle_tap(pos: Vector2) -> void:
	if over:
		return
	# Bonk the nearest demon under the tap.
	var best: WindDemon = null
	var best_d := 65.0 * 65.0
	for d in demons:
		if d.done:
			continue
		var dd := pos.distance_squared_to(d.position)
		if dd < best_d:
			best_d = dd
			best = d
	if best:
		_bonk_demon(best)
		return
	hero.tx = clampf(pos.x, 40.0, 350.0)
	hero.ty = clampf(pos.y, 120.0, 780.0)


func _unhandled_input(event: InputEvent) -> void:
	if over or self_test:
		return
	if event is InputEventScreenTouch and event.pressed:
		_handle_tap(event.position)
	elif event is InputEventMouseButton and event.pressed and event.button_index == MOUSE_BUTTON_LEFT:
		_handle_tap(event.position)


func _spawn_hole() -> void:
	holes_spawned += 1
	var hole := SkyHole.new()
	add_child(hole)
	hole.setup(60.0 + randf() * 270.0, 170.0 + randf() * 380.0)
	hole.stitched.connect(_on_hole_stitched)
	holes.append(hole)


func _on_hole_stitched(hole: SkyHole) -> void:
	if over:
		return
	hole.show_stitched()
	stitched += 1
	var gain := 50 * combo
	score += gain
	combo = mini(MAX_COMBO, combo + 1)
	_burst(hole.position, Color("#ffd6ec"), 18)
	_thread_flash(hero.position, hole.position)
	FloatLabel.spawn(self, hole.position, "+" + str(gain), Color("#e2488a"))
	_sfx("stitch", 1.0 + stitched * 0.06)
	_update_hud()
	if stitched >= HOLES_TO_WIN:
		win_t = 0.0
		win_tone_i = 0
	else:
		var nl := mini(3, stitched / 2 + 1)
		if nl > level:
			level = nl
			hero.fly_speed = 5.0 + 0.5 * (level - 1)
			_banner("第 " + str(level) + " 波！更快了！")
			_sfx("wave")
		else:
			_banner("补好啦！ " + str(stitched) + "/" + str(HOLES_TO_WIN) + " 🎀")


func _spawn_demon() -> void:
	var demon := WindDemon.new()
	add_child(demon)
	demon.setup(50.0 + randf() * 290.0, 55.0 * pow(1.25, level - 1) * meta_mult)
	demon.bonked.connect(_on_demon_bonked)
	demon.touched_hero.connect(_on_demon_touched_hero)
	demons.append(demon)


func _on_demon_bonked(demon: WindDemon) -> void:
	_bonk_demon(demon)


func _bonk_demon(demon: WindDemon) -> void:
	if demon.done:
		return
	demon.done = true
	demons.erase(demon)
	var pos := demon.position
	demon.queue_free()
	score += 100
	_burst(pos, Color("#ffe27a"), 16)
	FloatLabel.spawn(self, pos, "+100", Color("#b8860b"))
	_sfx("bonk")
	_update_hud()


func _on_demon_touched_hero(demon: WindDemon) -> void:
	if over or hit_t > 0.0:
		return
	demons.erase(demon)
	demon.queue_free()
	lives -= 1
	combo = 1
	hit_t = 1.5
	hero.blink(true)
	trauma = 0.7
	_burst(hero.position, Color("#ff9aa5"), 20)
	_banner("哎呀！被风妖撞到了！", Color("#ff9aa5"))
	_sfx("hit")
	_update_hud()
	if lives <= 0:
		_end_game()


func _spawn_shard() -> void:
	var shard := SkyShard.new()
	add_child(shard)
	shard.setup(50.0 + randf() * 290.0)
	shard.collected.connect(_on_shard_collected)
	shards.append(shard)


func _on_shard_collected(shard: SkyShard) -> void:
	if over:
		return
	shards.erase(shard)
	var pos := shard.position
	shard.queue_free()
	shards_got += 1
	var gain := 25
	if shards_got % 3 == 0:
		gain += 100
		_banner("幸运星星 +100! ⭐")
	score += gain
	_burst(pos, Color("#aef1ff"), 10)
	FloatLabel.spawn(self, pos, "+" + str(gain), Color("#2a9db8"))
	_sfx("shard", 1.0 + shards_got * 0.03)
	_update_hud()


func _next_round() -> void:
	meta_level += 1
	meta_mult *= 1.25
	stitched = 0
	combo = 1
	level = 1
	lives = MAX_LIVES
	hero.fly_speed = 5.0
	holes_spawned = 0
	spawn_hole_t = 0.0
	spawn_demon_t = 2.0
	spawn_shard_t = 1.0
	win_t = -1.0
	for h in holes:
		if is_instance_valid(h):
			h.queue_free()
	for d in demons:
		if is_instance_valid(d):
			d.queue_free()
	for s in shards:
		if is_instance_valid(s):
			s.queue_free()
	holes.clear()
	demons.clear()
	shards.clear()
	hero.blink(false)
	_update_hud()
	_banner("第 " + str(meta_level) + " 关！更快了！")
	_sfx("win")
	_burst(Vector2(195, 400), Color("#ffe27a"), 30)


func _end_game() -> void:
	over = true
	var n := mini(3, meta_level)
	_sfx("lose")
	var stars := ""
	for i in range(3):
		stars += "★" if i < n else "☆"
	_banner(stars + " 到达第" + str(meta_level) + "关！")
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
	t.text = stars + "\n到达第" + str(meta_level) + "关！\n⭐ " + str(score)
	t.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	t.add_theme_font_size_override("font_size", 26)
	vb.add_child(t)
	var btn := Button.new()
	btn.text = "再来一局"
	btn.custom_minimum_size = Vector2(200, 56)
	btn.pressed.connect(func() -> void: get_tree().reload_current_scene())
	vb.add_child(btn)
	panel.scale = Vector2(0.7, 0.7)
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
	(hud["score"] as Label).text = "🌈 " + str(stitched) + "/" + str(HOLES_TO_WIN) + "  ⭐ " + str(score)
	(hud["combo"] as Label).text = "连击 x" + str(combo) + "!" if combo > 1 else ""
	var s := ""
	for i in range(lives):
		s += "❤"
	(hud["lives"] as Label).text = s
	(hud["meta"] as Label).text = "第" + str(meta_level) + "关"


func _sweep(arr: Array) -> void:
	for i in range(arr.size() - 1, -1, -1):
		var e = arr[i]
		if not is_instance_valid(e) or e.done:
			arr.remove_at(i)


func _physics_process(delta: float) -> void:
	if over:
		return
	elapsed += delta
	if hit_t > 0.0:
		hit_t -= delta
		if hit_t <= 0.0:
			hero.blink(false)
	for c in clouds:
		c.position.x += 8.0 * delta
		if c.position.x > 440.0:
			c.position.x = -50.0
	# Holes drift in.
	if holes_spawned < HOLES_TO_WIN:
		spawn_hole_t += delta
		if spawn_hole_t > 2.2 / pow(1.25, level - 1) / meta_mult:
			spawn_hole_t = 0.0
			_spawn_hole()
	# Demons bumble down.
	spawn_demon_t += delta
	if spawn_demon_t > 4.0 / pow(1.3, level - 1) / meta_mult and demons.size() < 2 + level:
		spawn_demon_t = 0.0
		_spawn_demon()
	# Shards drift by.
	spawn_shard_t += delta
	if spawn_shard_t > 3.0 and shards.size() < 4:
		spawn_shard_t = 0.0
		_spawn_shard()
	_sweep(holes)
	_sweep(demons)
	_sweep(shards)
	# Camera shake.
	if trauma > 0.0:
		trauma = maxf(0.0, trauma - delta * 1.7)
		camera.offset = Vector2(randf_range(-1.0, 1.0), randf_range(-1.0, 1.0)) * trauma * 22.0
	elif camera.offset != Vector2.ZERO:
		camera.offset = Vector2.ZERO
	# Round-clear fanfare.
	if win_t >= 0.0:
		win_t += delta
		var tones: Array[float] = [523.0, 659.0, 784.0, 1047.0]
		while win_tone_i < tones.size() and win_tone_i * 0.22 <= win_t:
			_sfx("win", tones[win_tone_i] / 1047.0)
			win_tone_i += 1
		if win_t > 1.1:
			_next_round()
	# Self-test bot.
	if self_test:
		_test_bot(delta)
		test_elapsed += delta
		if meta_level >= 2:
			print("[self-test] PASS reached 关 2, score=", score)
			get_tree().quit()
		elif test_elapsed > 160.0:
			print("[self-test] FAIL timeout at 关 ", meta_level, " score=", score)
			get_tree().quit()


func _test_bot(delta: float) -> void:
	# Fly to the nearest open hole.
	var best: SkyHole = null
	var best_d := INF
	for h in holes:
		if h.done:
			continue
		var d := hero.position.distance_squared_to(h.position)
		if d < best_d:
			best_d = d
			best = h
	if best:
		hero.tx = best.position.x
		hero.ty = best.position.y
	# Bonk demons periodically through the real tap path.
	test_bonk_t += delta
	if test_bonk_t > 0.4:
		test_bonk_t = 0.0
		var nearest: WindDemon = null
		var nd := INF
		for d in demons:
			if d.done:
				continue
			var dd := hero.position.distance_squared_to(d.position)
			if dd < nd:
				nd = dd
				nearest = d
		if nearest:
			_handle_tap(nearest.position)
