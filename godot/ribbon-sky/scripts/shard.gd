class_name SkyShard
extends Area2D
## A drifting rainbow shard. Collected when the hero touches it.

signal collected(shard: SkyShard)

var done := false


func setup(start_x: float) -> void:
	position = Vector2(start_x, -40.0)
	collision_layer = 8
	collision_mask = 1
	monitoring = true
	monitorable = true
	var sprite := Sprite2D.new()
	sprite.texture = load("res://assets/shard.png")
	var ts: Vector2 = sprite.texture.get_size()
	sprite.scale = Vector2(44.0 / ts.x, 90.0 / ts.y)
	add_child(sprite)
	var shape := CollisionShape2D.new()
	var circle := CircleShape2D.new()
	circle.radius = 40.0
	shape.shape = circle
	add_child(shape)
	area_entered.connect(_on_area)
	var tw := create_tween().set_loops()
	tw.tween_property(sprite, "rotation", 0.35, 1.1).set_trans(Tween.TRANS_SINE)
	tw.tween_property(sprite, "rotation", -0.35, 1.1).set_trans(Tween.TRANS_SINE)


func _on_area(area: Area2D) -> void:
	if done:
		return
	if area is SkyHero:
		done = true
		collected.emit(self)


func _physics_process(delta: float) -> void:
	if done:
		return
	position.y += 75.0 * delta
	if position.y > 900.0:
		done = true
		queue_free()
