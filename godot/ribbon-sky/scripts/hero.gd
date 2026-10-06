class_name SkyHero
extends Area2D
## The cloud rider. Glides toward the tap point with a gentle bob.

var tx := 195.0
var ty := 660.0
var sprite: Sprite2D
var bob_t := 0.0
var fly_speed := 5.0


func _ready() -> void:
	collision_layer = 1
	collision_mask = 0
	monitoring = false
	monitorable = true
	sprite = Sprite2D.new()
	sprite.texture = load("res://assets/hero.png")
	var ts: Vector2 = sprite.texture.get_size()
	sprite.scale = Vector2(150.0 / ts.x, 96.0 / ts.y)
	add_child(sprite)
	var shape := CollisionShape2D.new()
	var circle := CircleShape2D.new()
	circle.radius = 55.0
	shape.shape = circle
	add_child(shape)


func _physics_process(delta: float) -> void:
	bob_t += delta
	var k := minf(1.0, delta * fly_speed)
	position.x = lerpf(position.x, tx, k)
	position.y = lerpf(position.y, ty + sin(bob_t * 4.0) * 6.0, k)


func blink(on: bool) -> void:
	var tw := create_tween().set_loops() if on else null
	if on:
		tw.tween_property(sprite, "modulate:a", 0.35, 0.15)
		tw.tween_property(sprite, "modulate:a", 1.0, 0.15)
		set_meta("blink_tween", tw)
	else:
		if has_meta("blink_tween"):
			var old: Tween = get_meta("blink_tween")
			if old and old.is_valid():
				old.kill()
		sprite.modulate.a = 1.0
