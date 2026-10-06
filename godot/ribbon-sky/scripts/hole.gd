class_name SkyHole
extends Area2D
## A dark sky-hole drifting down. Stitched when the hero flies into it:
## the open sprite pops into the stitched one with a thread flash.

signal stitched(hole: SkyHole)

var target_y := 300.0
var done := false
var open_sprite: Sprite2D
var closed_sprite: Sprite2D


func setup(start_x: float, ty: float) -> void:
	target_y = ty
	position = Vector2(start_x, -70.0)
	collision_layer = 2
	collision_mask = 1
	monitoring = true
	monitorable = true
	open_sprite = Sprite2D.new()
	open_sprite.texture = load("res://assets/hole.png")
	var ts: Vector2 = open_sprite.texture.get_size()
	open_sprite.scale = Vector2(110.0 / ts.x, 110.0 / ts.y)
	add_child(open_sprite)
	closed_sprite = Sprite2D.new()
	closed_sprite.texture = load("res://assets/hole_closed.png")
	closed_sprite.scale = open_sprite.scale
	closed_sprite.visible = false
	add_child(closed_sprite)
	var shape := CollisionShape2D.new()
	var circle := CircleShape2D.new()
	circle.radius = 60.0
	shape.shape = circle
	add_child(shape)
	area_entered.connect(_on_area)


func _on_area(area: Area2D) -> void:
	if done:
		return
	if area is SkyHero:
		done = true
		stitched.emit(self)


func _physics_process(delta: float) -> void:
	if position.y < target_y:
		position.y = minf(target_y, position.y + 22.0 * delta)


func show_stitched() -> void:
	open_sprite.visible = false
	closed_sprite.visible = true
	closed_sprite.scale = open_sprite.scale * 0.4
	var tw := create_tween()
	tw.tween_property(closed_sprite, "scale", open_sprite.scale * 1.12, 0.18).set_trans(Tween.TRANS_BACK).set_ease(Tween.EASE_OUT)
	tw.tween_property(closed_sprite, "scale", open_sprite.scale, 0.15)
