class_name Heart
extends Area2D
## A falling heart. Caught via area_entered against Bobo; missed below the screen.

signal missed(heart: Heart)

var speed := 200.0
var gold := false
var done := false
var sprite: Sprite2D


func setup(tex: Texture2D, is_gold: bool, fall_speed: float, start_pos: Vector2) -> void:
	gold = is_gold
	speed = fall_speed
	position = start_pos
	collision_layer = 2
	collision_mask = 1
	monitoring = true
	monitorable = true
	sprite = Sprite2D.new()
	sprite.texture = tex
	var ts: Vector2 = tex.get_size()
	var want := 58.0 if gold else 46.0
	sprite.scale = Vector2(want / ts.x, want / ts.y)
	add_child(sprite)
	var shape := CollisionShape2D.new()
	var circle := CircleShape2D.new()
	circle.radius = 24.0
	shape.shape = circle
	add_child(shape)
	if gold:
		var tw := create_tween().set_loops()
		var s0: Vector2 = sprite.scale
		tw.tween_property(sprite, "scale", s0 * 1.18, 0.4).set_trans(Tween.TRANS_SINE)
		tw.tween_property(sprite, "scale", s0, 0.4).set_trans(Tween.TRANS_SINE)


func _physics_process(delta: float) -> void:
	if done:
		return
	position.y += speed * delta
	if position.y > 880.0:
		done = true
		missed.emit(self)
