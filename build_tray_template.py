"""Render a macOS menu bar template icon for Prism.

Output: build/trayicon-template.png, 36x36 (2x Retina @ 18pt). Must be
black-on-transparent so macOS can recolour it per menu bar appearance.

Design: a stout triangular prism with a bold outline and a single refracted
ray crossing through it. The glyph is visually aligned with the other stock
menu bar icons (WiFi fan, battery, magnifying glass, etc.) — single shape,
thick stroke, no thin details that disappear at 1x.
"""
from PIL import Image, ImageDraw

SIZE = 36
OUT = "build/trayicon-template.png"


def main():
    img = Image.new("RGBA", (SIZE, SIZE), (0, 0, 0, 0))
    draw = ImageDraw.Draw(img)

    # Equilateral triangle centred in the canvas.
    pad_top = 5
    pad_bot = 6
    pad_side = 3
    top = (SIZE / 2, pad_top)
    bl = (pad_side, SIZE - pad_bot)
    br = (SIZE - pad_side, SIZE - pad_bot)

    # Thick outline triangle (~3px stroke at 2x = 1.5pt, same weight class as
    # the system WiFi / battery glyphs).
    black = (0, 0, 0, 255)
    stroke = 3
    draw.line([top, bl], fill=black, width=stroke)
    draw.line([bl, br], fill=black, width=stroke)
    draw.line([br, top], fill=black, width=stroke)

    # One refracted ray: enters near the left-bottom vertex, exits through
    # the right face toward lower-right. Kept to a single diagonal so the
    # glyph reads even at 1x / in the notch squeeze zone.
    ray_in_start = (1, SIZE - pad_bot + 1)
    ray_in_end = (SIZE * 0.42, SIZE * 0.55)
    ray_out_end = (SIZE - 1, SIZE * 0.78)
    draw.line([ray_in_start, ray_in_end], fill=black, width=2)
    draw.line([ray_in_end, ray_out_end], fill=black, width=2)

    img.save(OUT, format="PNG")
    print("wrote", OUT, img.size)


if __name__ == "__main__":
    main()
