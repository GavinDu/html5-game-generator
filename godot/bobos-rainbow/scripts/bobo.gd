class_name Bobo
extends Area2D
## The bear prince. Glides toward the tap x with a lean; squash & stretch on catch.

var target_x := 195.0
var sprite: Sprite2D


func _ready() -> void:
	collision_layer = 1
	collision_mask = 0
	monitoring = false
	monitorable = true
	sprite = Sprite2D.new()
	sprite.texture = load("res://assets/bear.png")
	var ts: Vector2 = sprite.texture.get_size()
	sprite.scale = Vector2(104.0 / ts.x, 192.0 / ts.y)
	add_child(sprite)
	var shape := CollisionShape2D.new()
	var circle := CircleShape2D.new()
	circle.radius = 55.0
	shape.shape = circle
	shape.position = Vector2(0, -40)
	add_child(shape)


func _physics_process(delta: float) -> void:
	var nx := lerpf(position.x, target_x, minf(1.0, delta * 10.0))
	rotation = clampf((target_x - nx) * 0.0015, -0.25, 0.25)
	position.x = nx


func squash() -> void:
	var tw := create_tween()
	tw.tween_property(self, "scale", Vector2(1.28, 0.78), 0.09)
	tw.tween_property(self, "scale", Vector2.ONE, 0.35).set_trans(Tween.TRANS_ELASTIC).set_ease(Tween.EASE_OUT)
