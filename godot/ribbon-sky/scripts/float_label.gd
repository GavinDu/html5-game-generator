class_name FloatLabel
extends Node2D
## Floating "+20" score popup, drawn with the fallback font.

var text := ""
var text_color := Color(0.07, 0.19, 0.27)
var life := 0.9
var age := 0.0


static func spawn(parent: Node, pos: Vector2, label_text: String, color := Color(0.07, 0.19, 0.27)) -> void:
	var fl := FloatLabel.new()
	fl.text = label_text
	fl.text_color = color
	fl.position = pos
	parent.add_child(fl)


func _process(delta: float) -> void:
	age += delta
	position.y -= 62.0 * delta
	var a := clampf(1.0 - age / life, 0.0, 1.0)
	modulate.a = a
	queue_redraw()
	if age >= life:
		queue_free()


func _draw() -> void:
	var font := ThemeDB.fallback_font
	var c := text_color
	c.a = modulate.a
	draw_string(font, Vector2(-60, 0), text, HORIZONTAL_ALIGNMENT_CENTER, 120, 24, c)
