class_name CourageStar
extends Area2D
## A courage star. Collect it for score and to push the chaser back.

signal collected(star: CourageStar)

var speed := 210.0
var done := false
var sprite: Sprite2D


func setup(fall_speed: float, start_x: float) -> void:
	speed = fall_speed
	position = Vector2(start_x, -40.0)
	collision_layer = 4
	collision_mask = 1
	monitoring = true
	monitorable = true
	sprite = Sprite2D.new()
	sprite.texture = load("res://assets/star.png")
	var ts: Vector2 = sprite.texture.get_size()
	sprite.scale = Vector2(52.0 / ts.x, 52.0 / ts.y)
	add_child(sprite)
	var shape := CollisionShape2D.new()
	var circle := CircleShape2D.new()
	circle.radius = 30.0
	shape.shape = circle
	add_child(shape)
	area_entered.connect(_on_area)
	var tw := create_tween().set_loops()
	tw.tween_property(sprite, "rotation", TAU, 2.4).from(0.0)


func _on_area(area: Area2D) -> void:
	if done:
		return
	if area is EscapeBlob:
		done = true
		collected.emit(self)


func _physics_process(delta: float) -> void:
	if done:
		return
	position.y += speed * delta
	if position.y > 920.0:
		done = true
		queue_free()
