class_name Rainbow
extends Node2D
## The six-band rainbow arch, drawn natively with draw_arc.
## Bands reveal one by one with a pop; revealed bands shimmer gently.

const RADII: Array[float] = [150.0, 132.0, 114.0, 96.0, 78.0, 60.0]
const COLORS: Array[Color] = [
	Color("#FF5A5A"), Color("#FFA94D"), Color("#FFD94D"),
	Color("#6BCB77"), Color("#4D96FF"), Color("#B983FF"),
]

var bands_revealed := 0
var time := 0.0


func reveal_band(idx: int) -> void:
	bands_revealed = maxi(bands_revealed, idx + 1)
	queue_redraw()
	var tw := create_tween()
	tw.tween_property(self, "scale", Vector2(1.08, 1.08), 0.12)
	tw.tween_property(self, "scale", Vector2.ONE, 0.3).set_trans(Tween.TRANS_BACK).set_ease(Tween.EASE_OUT)


func reset() -> void:
	bands_revealed = 0
	scale = Vector2.ONE
	queue_redraw()


func _process(delta: float) -> void:
	time += delta
	if bands_revealed > 0:
		queue_redraw()


func _draw() -> void:
	for i in range(bands_revealed):
		var shimmer := 0.9 + 0.1 * sin(time * 3.0 + float(i) * 0.7)
		var c := COLORS[i]
		c.a = shimmer
		draw_arc(Vector2.ZERO, RADII[i], PI, TAU, 64, c, 17.0, true)
