"""Build assets/tray_windows.ico from the master artwork.

The Windows shell never scales an icon it can find an exact frame for, so the
frame list covers every size the display scalings ask for: 32 and 16 at 100%,
40 and 20 at 125%, 48 and 24 at 150%, and so on up to 300%, plus the sizes
Explorer's icon views use. A missing frame is what makes an icon look soft --
Windows takes the nearest one and stretches it.

Downscaling happens in linear light with premultiplied alpha, because averaging
sRGB values directly drags the bright metal edges of this mark toward the dark
body and leaves the small sizes muddy. Each size then gets an unsharp pass
sized to it, since detail that survives at 96px is a smear at 16px.

Usage:
    python scripts/icons/build_windows_icon.py
    go generate .     # rebuilds the committed rsrc_windows_*.syso

Requires Pillow and NumPy.
"""

from __future__ import annotations

import struct
from io import BytesIO
from pathlib import Path

import numpy as np
from PIL import Image, ImageFilter

ROOT = Path(__file__).resolve().parents[2]
SOURCE = ROOT / "assets" / "source_tray_icon.png"
ICO_OUT = ROOT / "assets" / "tray_windows.ico"
PNG_OUT = ROOT / "assets" / "tray.png"

# 16/32 are the 100% window and shell icons; the rest are those two and the
# 48/96 shell sizes at 125%, 150%, 175%, 200%, 225%, 250% and 300%.
SIZES = (16, 20, 24, 28, 32, 36, 40, 48, 56, 60, 64, 72, 80, 96, 120, 128, 256)

# Below this, frames are stored as DIBs. PNG frames need Vista-era shell code to
# decode, and the window icon path (LookupIconIdFromDirectoryEx plus
# CreateIconFromResourceEx, in desktop_chrome_windows.go) reads exactly these
# small frames. Above it, PNG saves enough bytes in the executable to be worth
# the newer decoder.
PNG_FROM = 96


def srgb_to_linear(c: np.ndarray) -> np.ndarray:
    return np.where(c <= 0.04045, c / 12.92, ((c + 0.055) / 1.055) ** 2.4)


def linear_to_srgb(c: np.ndarray) -> np.ndarray:
    c = np.clip(c, 0.0, 1.0)
    return np.where(c <= 0.0031308, c * 12.92, 1.055 * np.power(c, 1 / 2.4) - 0.055)


def square(art: Image.Image, margin: float) -> Image.Image:
    """Center the trimmed artwork on a square canvas with the given margin."""
    side = int(round(max(art.size) / (1 - 2 * margin)))
    canvas = Image.new("RGBA", (side, side), (0, 0, 0, 0))
    canvas.paste(art, ((side - art.width) // 2, (side - art.height) // 2))
    return canvas


def resample(img: Image.Image, size: int) -> Image.Image:
    source = np.asarray(img).astype(np.float64) / 255.0
    alpha = source[..., 3:4]
    planes = np.concatenate([srgb_to_linear(source[..., :3]) * alpha, alpha], axis=2)
    scaled = np.stack(
        [
            np.asarray(
                Image.fromarray(planes[..., i].astype(np.float32), mode="F")
                .resize((size, size), Image.LANCZOS)
            ).astype(np.float64)
            for i in range(4)
        ],
        axis=2,
    )
    out_alpha = np.clip(scaled[..., 3:4], 0.0, 1.0)
    rgb = linear_to_srgb(scaled[..., :3] / np.where(out_alpha > 1e-5, out_alpha, 1.0))
    pixels = np.concatenate([rgb, out_alpha], axis=2)
    return Image.fromarray((np.clip(pixels, 0, 1) * 255 + 0.5).astype(np.uint8), "RGBA")


def margin_for(size: int) -> float:
    # A 16px frame cannot spare a pixel to padding; a 256px one looks cramped
    # without it.
    if size <= 24:
        return 0.0
    if size <= 64:
        return 0.02
    return 0.035


def sharpen_for(size: int) -> int:
    # Percentages, tuned by eye against the old frames: the more the artwork
    # shrank, the more its edges need putting back.
    if size <= 20:
        return 150
    if size <= 32:
        return 125
    if size <= 64:
        return 80
    if size < 128:
        return 40
    return 0


def frame(art: Image.Image, size: int) -> Image.Image:
    img = resample(square(art, margin_for(size)), size)
    percent = sharpen_for(size)
    if percent:
        img = img.filter(ImageFilter.UnsharpMask(radius=1.0, percent=percent, threshold=0))
    return img


def dib(img: Image.Image) -> bytes:
    """A 32bpp bottom-up DIB with the all-zero AND mask an ICONDIR expects."""
    width, height = img.size
    header = struct.pack(
        "<IiiHHIIiiII", 40, width, height * 2, 1, 32, 0, width * height * 4, 0, 0, 0, 0
    )
    bgra = np.asarray(img)[..., [2, 1, 0, 3]]
    mask_stride = ((width + 31) // 32) * 4
    return header + np.flipud(bgra).tobytes() + b"\0" * (mask_stride * height)


def encode(img: Image.Image) -> bytes:
    if img.width >= PNG_FROM:
        buffer = BytesIO()
        img.save(buffer, "png", optimize=True)
        return buffer.getvalue()
    return dib(img)


def write_ico(frames: list[Image.Image], path: Path) -> None:
    payloads = [encode(img) for img in frames]
    offset = 6 + 16 * len(frames)
    directory = bytearray(struct.pack("<HHH", 0, 1, len(frames)))
    for img, payload in zip(frames, payloads):
        side = 0 if img.width >= 256 else img.width
        directory += struct.pack(
            "<BBBBHHII", side, side, 0, 0, 1, 32, len(payload), offset
        )
        offset += len(payload)
    path.write_bytes(bytes(directory) + b"".join(payloads))


def main() -> None:
    master = Image.open(SOURCE).convert("RGBA")
    art = master.crop(master.split()[3].getbbox())
    frames = [frame(art, size) for size in SIZES]
    write_ico(frames, ICO_OUT)
    # The cross-platform tray takes the same mark at its own single size.
    frame(art, 32).save(PNG_OUT, "png", optimize=True)
    print(f"{ICO_OUT.relative_to(ROOT)}: {len(SIZES)} frames, {ICO_OUT.stat().st_size} bytes")


if __name__ == "__main__":
    main()
