class_name EscapeBlob
extends Area2D
## The blob patient. Lane changes tween with squash & stretch and a lean.

const LANES: Array[float] = [90.0, 195.0, 300.0]

var lane := 1
var sprite: Sprite2D
var move_tween: Tween
var base_scale := Vector2.ONE


func _ready() -> void:
	collision_layer = 1
	collision_mask = 0
	monitoring = false
	monitorable = true
	sprite = Sprite2D.new()
	sprite.texture = load("res://assets/blob.png")
	var ts: Vector2 = sprite.texture.get_size()
	base_scale = Vector2(112.0 / ts.x, 128.0 / ts.y)
	sprite.scale = base_scale
	add_child(sprite)
	var shape := CollisionShape2D.new()
	var circle := CircleShape2D.new()
	circle.radius = 45.0
	shape.shape = circle
	add_child(shape)


func move_lane(dir: int) -> void:
	var nl := clampi(lane + dir, 0, 2)
	if nl == lane:
		return
	lane = nl
	if move_tween and move_tween.is_valid():
		move_tween.kill()
	move_tween = create_tween().set_parallel(true)
	move_tween.tween_property(self, "position:x", LANES[lane], 0.18).set_trans(Tween.TRANS_CUBIC).set_ease(Tween.EASE_OUT)
	move_tween.tween_property(sprite, "rotation", 0.16 * dir, 0.12)
	move_tween.tween_property(sprite, "scale", Vector2(1.18, 0.86), 0.1)
	var st := create_tween()
	st.tween_interval(0.12)
	st.tween_property(sprite, "rotation", 0.0, 0.18)
	st.tween_property(sprite, "scale", base_scale, 0.22).set_trans(Tween.TRANS_ELASTIC).set_ease(Tween.EASE_OUT)
