"""Render a macOS menu bar template icon for Prism.

Output: build/trayicon-template.png, 36x36 (2x Retina @ 18pt). Must be
black-on-transparent so macOS can recolour it per menu bar appearance.

Design: a stout triangular prism with a bold outline and a single refracted
ray crossing through it. The glyph is visually aligned with the other stock
menu bar icons (WiFi fan, battery, magnifying glass, etc.) — single shape,
thick stroke, no thin details that disappear at 1x.

Rendering note: PIL's ImageDraw.line does NOT antialias — it writes
binary 0/255 alpha which shows up as visible stair-stepping ("毛边") on
the rendered template, especially for the diagonal triangle edges and
the refracted ray. We sidestep that by rendering at SS_FACTOR x the
target size and then downsampling with LANCZOS, which produces clean
fractional-alpha edges that macOS templates render smoothly.
"""
from PIL import Image, ImageDraw

SIZE = 36
SS_FACTOR = 4  # supersample factor; 4x is the sweet spot — visibly
               # smoother than 2x, and 8x produces no discernible
               # improvement over 4x once LANCZOS-downsampled.
OUT = "build/trayicon-template.png"


def main():
    canvas = SIZE * SS_FACTOR
    img = Image.new("RGBA", (canvas, canvas), (0, 0, 0, 0))
    draw = ImageDraw.Draw(img)

    # Geometry expressed in target-pt units, scaled into the supersample
    # canvas at draw time. Padding values mirror the original layout so
    # the glyph footprint inside the menu bar stays unchanged — this
    # change is purely about edge quality.
    pad_top = 5
    pad_bot = 6
    pad_side = 3
    top = (SIZE / 2 * SS_FACTOR, pad_top * SS_FACTOR)
    bl = (pad_side * SS_FACTOR, (SIZE - pad_bot) * SS_FACTOR)
    br = ((SIZE - pad_side) * SS_FACTOR, (SIZE - pad_bot) * SS_FACTOR)

    # Stroke widths are in target-pt as well (~3pt outline at SIZE,
    # ~2pt ray) so the supersample factor doesn't accidentally make
    # the icon heavier or lighter than before.
    black = (0, 0, 0, 255)
    triangle_stroke = 3 * SS_FACTOR
    ray_stroke = 2 * SS_FACTOR

    draw.line([top, bl], fill=black, width=triangle_stroke)
    draw.line([bl, br], fill=black, width=triangle_stroke)
    draw.line([br, top], fill=black, width=triangle_stroke)

    # One refracted ray: enters near the left-bottom vertex, exits
    # through the right face toward lower-right. Single diagonal so
    # the glyph reads even at 1x / in the notch squeeze zone.
    ray_in_start = (1 * SS_FACTOR, (SIZE - pad_bot + 1) * SS_FACTOR)
    ray_in_end = (SIZE * 0.42 * SS_FACTOR, SIZE * 0.55 * SS_FACTOR)
    ray_out_end = ((SIZE - 1) * SS_FACTOR, SIZE * 0.78 * SS_FACTOR)
    draw.line([ray_in_start, ray_in_end], fill=black, width=ray_stroke)
    draw.line([ray_in_end, ray_out_end], fill=black, width=ray_stroke)

    img = img.resize((SIZE, SIZE), Image.LANCZOS)
    img.save(OUT, format="PNG")
    print("wrote", OUT, img.size)


if __name__ == "__main__":
    main()
