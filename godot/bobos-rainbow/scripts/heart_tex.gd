class_name HeartTex
extends RefCounted
## Procedural heart sprite textures (the web originals were procedural too).
## Draws a dark outline ring with a colored heart inside.


static func _inside(px: float, py: float, s: float) -> bool:
	var r := s * 0.27
	var c1 := Vector2(s * 0.32, s * 0.36)
	var c2 := Vector2(s * 0.68, s * 0.36)
	var d1 := (px - c1.x) * (px - c1.x) + (py - c1.y) * (py - c1.y)
	var d2 := (px - c2.x) * (px - c2.x) + (py - c2.y) * (py - c2.y)
	if d1 < r * r or d2 < r * r:
		return true
	# Triangle: (0.07s,0.44s) (0.93s,0.44s) (0.5s,0.96s)
	var ax := s * 0.07
	var ay := s * 0.44
	var bx := s * 0.93
	var by := s * 0.44
	var cx := s * 0.5
	var cy := s * 0.96
	var d := (by - cy) * (ax - cx) + (cx - bx) * (ay - cy)
	if absf(d) < 0.0001:
		return false
	var l1 := ((by - cy) * (px - cx) + (cx - bx) * (py - cy)) / d
	var l2 := ((cy - ay) * (px - cx) + (ax - cx) * (py - cy)) / d
	var l3 := 1.0 - l1 - l2
	return l1 >= 0.0 and l2 >= 0.0 and l3 >= 0.0


static func make(color: Color, size := 46) -> ImageTexture:
	var img := Image.create(size, size, false, Image.FORMAT_RGBA8)
	img.fill(Color(0, 0, 0, 0))
	var dark := Color(0.16, 0.13, 0.16, 1.0)
	var shrink := 0.82
	var ox := size * 0.5 * (1.0 - shrink)
	var oy := size * 0.5 * (1.0 - shrink)
	for y in range(size):
		for x in range(size):
			if _inside(float(x), float(y), float(size)):
				img.set_pixel(x, y, dark)
			var sx := ox + float(x) * shrink
			var sy := oy + float(y) * shrink
			if _inside(sx, sy, float(size)):
				img.set_pixel(x, y, color)
	# glossy highlight dot
	var hr := size * 0.09
	for y in range(size):
		for x in range(size):
			var dx := float(x) - size * 0.36
			var dy := float(y) - size * 0.30
			if dx * dx + dy * dy < hr * hr:
				img.set_pixel(x, y, Color(1, 1, 1, 0.55))
	return ImageTexture.create_from_image(img)
