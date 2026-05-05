"""
Enlarge the prism in the app icon.

Strategy:
  - Start from the user-approved source (img_v3_02118_*.png): 1024x1024,
    already square, four corners pure black, prism centered.
  - Crop the center ~72% (removes excessive blank margin around the prism).
  - Paste it back onto a fresh 1024x1024 pure-black canvas so the prism
    visually occupies ~82% of the icon — Dock renders it noticeably larger.
  - macOS applies its own rounded-corner mask on top (10% margin ensures
    the mask never clips the prism body).
"""
from PIL import Image

SRC = "/Users/seikiko/.cursor/projects/Users-seikiko-Downloads-one-api-0-6-10/assets/img_v3_02118_3e8c7ee2-dfda-4f0e-8fb4-77a416608d7g-5f0e646b-e251-48dd-b10c-944b48bab83d.png"
DST_APP = "build/appicon.png"
DST_TRAY = "build/trayicon.png"
DST_FRONTEND = "frontend/src/assets/logo.png"
SIZE = 1024


def main():
    src = Image.open(SRC).convert("RGB")
    w, h = src.size

    crop_ratio = 0.72
    side = int(min(w, h) * crop_ratio)
    lx = (w - side) // 2
    ly = (h - side) // 2
    center = src.crop((lx, ly, lx + side, ly + side))

    canvas = Image.new("RGB", (SIZE, SIZE), (6, 7, 11))
    target = int(SIZE * 0.88)
    scaled = center.resize((target, target), Image.Resampling.LANCZOS)
    ox = (SIZE - target) // 2
    oy = (SIZE - target) // 2
    canvas.paste(scaled, (ox, oy))

    canvas.save(DST_APP, format="PNG")
    canvas.resize((32, 32), Image.Resampling.LANCZOS).save(DST_TRAY, format="PNG")
    canvas.save(DST_FRONTEND, format="PNG")
    print("wrote", DST_APP, DST_FRONTEND, canvas.size)


if __name__ == "__main__":
    main()
