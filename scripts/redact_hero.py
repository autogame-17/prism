"""Redact the embedded one-line URL in docs/screenshots/hero.raw.png and write hero.png.

We blur the public URL substring that follows "OPENAI-COMPATIBLE BASE URL" so that
the screenshot can be published without leaking the user's currently-active
trycloudflare.com hostname (even though it's ephemeral, posting it on a public
README invites random scrapers to probe it).

The coordinates are derived from the 1024x724 source by hand. If the design or
font changes, eyeball the URL strip in Preview and update X1/X2/Y1/Y2 below.
"""

from PIL import Image, ImageFilter, ImageDraw

SRC = "/Users/seikiko/Downloads/prism-desktop/docs/screenshots/hero.raw.png"
DST = "/Users/seikiko/Downloads/prism-desktop/docs/screenshots/hero.png"

X1, Y1, X2, Y2 = 304, 211, 588, 234

img = Image.open(SRC).convert("RGB")

region = img.crop((X1, Y1, X2, Y2))
region = region.filter(ImageFilter.GaussianBlur(radius=8))
img.paste(region, (X1, Y1))

draw = ImageDraw.Draw(img)
mid = (Y1 + Y2) // 2
draw.text((X1 + 8, mid - 7), "https://prism-xxxx.example.tld", fill=(170, 170, 170))

img.save(DST, optimize=True)
print(f"wrote {DST} ({img.size})")
