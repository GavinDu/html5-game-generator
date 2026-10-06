class_name WindDemon
extends Area2D
## A wind demon bumbling down with a sway. Bonked by tapping it.

signal bonked(demon: WindDemon)
signal touched_hero(demon: WindDemon)

var fall_speed := 55.0
var sway := 0.0
var done := false
var sprite: Sprite2D


func setup(start_x: float, speed: float) -> void:
	fall_speed = speed
	position = Vector2(start_x, -50.0)
	sway = randf() * 6.0
	collision_layer = 4
	collision_mask = 1
	monitoring = true
	monitorable = true
	sprite = Sprite2D.new()
	sprite.texture = load("res://assets/demon.png")
	var ts: Vector2 = sprite.texture.get_size()
	sprite.scale = Vector2(64.0 / ts.x, 78.0 / ts.y)
	add_child(sprite)
	var shape := CollisionShape2D.new()
	var circle := CircleShape2D.new()
	circle.radius = 34.0
	shape.shape = circle
	add_child(shape)
	area_entered.connect(_on_area)


func _on_area(area: Area2D) -> void:
	if done:
		return
	if area is SkyHero:
		done = true
		touched_hero.emit(self)


func bonk() -> void:
	if done:
		return
	done = true
	bonked.emit(self)


func _physics_process(delta: float) -> void:
	if done:
		return
	position.y += fall_speed * delta
	sway += delta * 3.0
	position.x += sin(sway) * 30.0 * delta
	rotation = sin(sway) * 0.18
	if position.y > 900.0:
		done = true
		queue_free()
