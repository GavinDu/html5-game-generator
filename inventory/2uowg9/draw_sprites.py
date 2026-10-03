#!/usr/bin/env python3
"""Draw the Escape from the Injection sprites in a cute rounded-cartoon style
(thick black outlines, flat bright fills, blush cheeks) a la dress-up-duck.

Outputs PNGs into assets/ next to this script.
"""
import math
import os
from PIL import Image, ImageDraw

HERE = os.path.dirname(os.path.abspath(__file__))
ASSETS = os.path.join(HERE, "assets")
BLACK = (35, 30, 40, 255)

def outlined_ellipse(d, bbox, fill, outline_w=7):
    x0, y0, x1, y1 = bbox
    d.ellipse([x0 - outline_w, y0 - outline_w, x1 + outline_w, y1 + outline_w], fill=BLACK)
    d.ellipse(bbox, fill=fill)

def outlined_rounded_rect(d, bbox, r, fill, outline_w=7):
    x0, y0, x1, y1 = bbox
    d.rounded_rectangle([x0 - outline_w, y0 - outline_w, x1 + outline_w, y1 + outline_w],
                        radius=r + outline_w, fill=BLACK)
    d.rounded_rectangle(bbox, radius=r, fill=fill)

def outlined_polygon(d, pts, fill, outline_w=7):
    # fake thick outline by stamping black circles along the edges
    for (x0, y0), (x1, y1) in zip(pts, pts[1:] + pts[:1]):
        steps = max(2, int(math.hypot(x1 - x0, y1 - y0) / 4))
        for i in range(steps + 1):
            t = i / steps
            d.ellipse([x0 + (x1 - x0) * t - outline_w, y0 + (y1 - y0) * t - outline_w,
                       x0 + (x1 - x0) * t + outline_w, y0 + (y1 - y0) * t + outline_w], fill=BLACK)
    d.polygon(pts, fill=fill)

def eyes(d, cx, cy, r=11, gap=26, worried=False):
    for sx in (-1, 1):
        ex = cx + sx * gap
        d.ellipse([ex - r - 3, cy - r - 5, ex + r + 3, cy + r + 3], fill=BLACK)
        d.ellipse([ex - r, cy - r - 2, ex + r, cy + r], fill=(255, 255, 255, 255))
        px, py = ex - 3, cy - 6
        d.ellipse([px - 5, py - 5, px + 5, py + 5], fill=BLACK)
        d.ellipse([px - 2, py - 8, px + 2, py - 4], fill=(255, 255, 255, 255))
    if worried:  # raised scared brows
        d.line([cx - gap - 14, cy - r - 16, cx - gap + 10, cy - r - 24], fill=BLACK, width=5)
        d.line([cx + gap - 10, cy - r - 24, cx + gap + 14, cy - r - 16], fill=BLACK, width=5)

def blush(d, cx, cy, gap=34):
    for sx in (-1, 1):
        d.ellipse([cx + sx * gap - 12, cy - 8, cx + sx * gap + 12, cy + 8],
                  fill=(255, 150, 170, 255))

def smile(d, cx, cy, w=26, h=14):
    d.arc([cx - w, cy - h, cx + w, cy + h], start=15, end=165, fill=BLACK, width=6)

# ---------------------------------------------------------------- blob
def draw_blob():
    img = Image.new("RGBA", (140, 160), (0, 0, 0, 0))
    d = ImageDraw.Draw(img)
    # feet
    for fx in (48, 92):
        outlined_rounded_rect(d, [fx - 14, 128, fx + 14, 150], 10, (255, 253, 245, 255), 6)
    # body
    outlined_ellipse(d, [14, 12, 126, 142], (255, 253, 245, 255), 7)
    eyes(d, 70, 66, r=12, gap=26, worried=True)
    blush(d, 70, 96, gap=36)
    smile(d, 70, 92, w=22, h=12)
    img.save(os.path.join(ASSETS, "blob.png"))

# --------------------------------------------------------------- chaser
def draw_chaser():
    # kid with spiky hair holding a syringe, dress-up-duck style
    img = Image.new("RGBA", (150, 175), (0, 0, 0, 0))
    d = ImageDraw.Draw(img)
    # body (teal shirt)
    outlined_rounded_rect(d, [45, 92, 105, 158], 18, (90, 200, 230, 255), 7)
    # arm + syringe (right side, pointing up-forward)
    d.line([100, 110, 128, 78], fill=BLACK, width=13)
    d.line([100, 110, 128, 78], fill=(255, 217, 179, 255), width=7)
    # syringe in hand
    sx, sy = 122, 66
    d.polygon([(sx - 4, sy + 26), (sx + 4, sy + 26), (sx + 2, sy + 40), (sx - 2, sy + 40)],
              fill=(160, 160, 170, 255))  # needle
    outlined_rounded_rect(d, [sx - 11, sy - 12, sx + 11, sy + 26], 4, (190, 225, 255, 255), 5)
    d.rectangle([sx - 6, sy + 2, sx + 6, sy + 22], fill=(80, 168, 255, 255))  # liquid
    d.rectangle([sx - 14, sy - 20, sx + 14, sy - 12], fill=(255, 150, 170, 255))  # plunger pad
    d.rectangle([sx - 3, sy - 12, sx + 3, sy - 2], fill=(160, 160, 170, 255))
    # head
    outlined_ellipse(d, [38, 22, 112, 96], (255, 217, 179, 255), 7)
    # spiky hair
    spikes = [(38, 52), (30, 22), (48, 30), (50, 8), (66, 24), (76, 4),
              (88, 24), (104, 12), (104, 34), (120, 30), (112, 52)]
    outlined_polygon(d, spikes, (45, 40, 50, 255), 5)
    # angry-determined face
    for sxx, ex in ((-1, 58), (1, 92)):
        d.ellipse([ex - 11, 52, ex + 11, 74], fill=BLACK)
        d.ellipse([ex - 8, 55, ex + 8, 71], fill=(255, 255, 255, 255))
        d.ellipse([ex - 1, 57, ex + 7, 65], fill=BLACK)
    d.line([44, 46, 66, 52], fill=BLACK, width=5)   # brows ><
    d.line([106, 46, 84, 52], fill=BLACK, width=5)
    blush(d, 75, 82, gap=30)
    d.arc([63, 78, 87, 94], start=200, end=340, fill=BLACK, width=5)  # gritting smile
    img.save(os.path.join(ASSETS, "chaser.png"))

# -------------------------------------------------------------- syringe
def draw_syringe():
    # big obstacle syringe
    img = Image.new("RGBA", (80, 130), (0, 0, 0, 0))
    d = ImageDraw.Draw(img)
    cx = 40
    # needle
    d.polygon([(cx - 3, 108), (cx + 3, 108), (cx, 126)], fill=(170, 170, 180, 255))
    # barrel
    outlined_rounded_rect(d, [cx - 20, 18, cx + 20, 108], 8, (190, 225, 255, 255), 6)
    # liquid
    d.rounded_rectangle([cx - 14, 62, cx + 14, 102], radius=6, fill=(80, 168, 255, 255))
    # measurement ticks
    for ty in (34, 48, 62, 76, 90):
        d.line([cx + 8, ty, cx + 16, ty], fill=BLACK, width=3)
    # plunger
    d.rectangle([cx - 4, 2, cx + 4, 18], fill=(170, 170, 180, 255))
    outlined_rounded_rect(d, [cx - 16, -6, cx + 16, 6], 5, (255, 150, 170, 255), 5)
    img.save(os.path.join(ASSETS, "syringe.png"))

# ------------------------------------------------------------------ star
def draw_star():
    img = Image.new("RGBA", (64, 64), (0, 0, 0, 0))
    d = ImageDraw.Draw(img)
    cx, cy, r1, r2 = 32, 32, 26, 11
    pts = []
    for i in range(10):
        a = -math.pi / 2 + i * math.pi / 5
        r = r1 if i % 2 == 0 else r2
        pts.append((cx + r * math.cos(a), cy + r * math.sin(a)))
    outlined_polygon(d, pts, (255, 217, 77, 255), 5)
    # cute face
    d.ellipse([22, 24, 28, 32], fill=BLACK)
    d.ellipse([36, 24, 42, 32], fill=BLACK)
    d.arc([26, 30, 38, 42], start=20, end=160, fill=BLACK, width=3)
    d.ellipse([44, 16, 50, 22], fill=(255, 255, 255, 220))  # sparkle
    img.save(os.path.join(ASSETS, "star.png"))

# ---------------------------------------------------------------- backgrounds
def draw_bg(name, top, bottom):
    W, H = 390, 844
    img = Image.new("RGBA", (W, H), (0, 0, 0, 255))
    d = ImageDraw.Draw(img)
    for y in range(H):
        t = y / H
        d.line([(0, y), (W, y)],
               fill=tuple(int(top[i] + (bottom[i] - top[i]) * t) for i in range(3)) + (255,))
    # soft white dots
    import random
    random.seed(7)
    for _ in range(40):
        x, y, r = random.randint(0, W), random.randint(0, H), random.randint(6, 16)
        d.ellipse([x - r, y - r, x + r, y + r], fill=(255, 255, 255, 70))
    # faint lane guides
    for lx in (142, 247):
        y = 0
        while y < H:
            d.line([(lx, y), (lx, y + 26)], fill=(255, 255, 255, 90), width=5)
            y += 52
    img.save(os.path.join(ASSETS, name))

def main():
    os.makedirs(ASSETS, exist_ok=True)
    draw_blob()
    draw_chaser()
    draw_syringe()
    draw_star()
    draw_bg("bg0.png", (207, 245, 225), (234, 251, 243))   # mint clinic
    draw_bg("bg1.png", (255, 232, 209), (255, 246, 234))   # peach hallway
    draw_bg("bg2.png", (227, 220, 255), (244, 241, 255))   # lavender playground
    print("sprites drawn ->", ASSETS)

if __name__ == "__main__":
    main()
