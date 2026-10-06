class_name FallingSyringe
extends Area2D
## A syringe falling down the escape route. Hits the blob on contact.

signal hit(syringe: FallingSyringe)

var speed := 210.0
var born := 0.0
var lane := 1
var done := false
var wobble := 0.0


func setup(fall_speed: float, born_t: float, start_x: float) -> void:
	speed = fall_speed
	born = born_t
	position = Vector2(start_x, -60.0)
	wobble = randf() * TAU
	collision_layer = 2
	collision_mask = 1
	monitoring = true
	monitorable = true
	var sprite := Sprite2D.new()
	sprite.texture = load("res://assets/syringe.png")
	var ts: Vector2 = sprite.texture.get_size()
	sprite.scale = Vector2(56.0 / ts.x, 84.0 / ts.y)
	add_child(sprite)
	var shape := CollisionShape2D.new()
	var circle := CircleShape2D.new()
	circle.radius = 28.0
	shape.shape = circle
	add_child(shape)
	area_entered.connect(_on_area)


func _on_area(area: Area2D) -> void:
	if done:
		return
	if area is EscapeBlob:
		done = true
		hit.emit(self)


func _physics_process(delta: float) -> void:
	if done:
		return
	position.y += speed * delta
	wobble += delta * 4.0
	rotation = sin(wobble) * 0.12
	if position.y > 920.0:
		done = true
		queue_free()
