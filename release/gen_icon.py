#!/usr/bin/env python3
"""Generate Lexicon icon.ico — multi-resolution ICO for Windows installer.
Also generates icon-192.png, icon-512.png from the SVG source.

Usage: python3 gen_icon.py
"""
import os
import shutil
import struct
import io

def svg_to_png(svg_path, size):
    """Rasterize an SVG to a PIL Image at the given size using cairosvg."""
    import cairosvg
    from PIL import Image
    png_bytes = cairosvg.svg2png(url=svg_path, output_width=size, output_height=size)
    return Image.open(io.BytesIO(png_bytes)).convert("RGBA")


def save_ico(images, path):
    """Save a proper multi-resolution ICO file."""
    count = len(images)
    header = struct.pack('<HHH', 0, 1, count)  # reserved, type=1 (icon), count
    data_offset = 6 + count * 16
    entries = b''
    image_data = b''
    for img in images:
        buf = io.BytesIO()
        img.save(buf, format='PNG')
        png_bytes = buf.getvalue()
        w = img.width if img.width < 256 else 0
        h = img.height if img.height < 256 else 0
        entries += struct.pack('<BBBBHHII', w, h, 0, 0, 0, 32, len(png_bytes), data_offset)
        image_data += png_bytes
        data_offset += len(png_bytes)
    with open(path, 'wb') as f:
        f.write(header + entries + image_data)


def main():
    out_dir = "/mnt/c/Users/kevin/CascadeProjects/lexicon/release"
    os.makedirs(out_dir, exist_ok=True)

    svg_path = "/mnt/c/Users/kevin/CascadeProjects/lexicon/frontend/public/icon.svg"
    if not os.path.exists(svg_path):
        print(f"ERROR: SVG not found at {svg_path}")
        return

    # Generate ICO with multiple resolutions from the SVG
    ico_sizes = [16, 32, 48, 64, 128, 256]
    ico_images = []
    for sz in ico_sizes:
        img = svg_to_png(svg_path, sz)
        ico_images.append(img)
        print(f"  Generated {sz}x{sz} icon")

    ico_path = os.path.join(out_dir, "lexicon.ico")
    save_ico(ico_images, ico_path)
    print(f"  Saved {ico_path} ({os.path.getsize(ico_path)} bytes)")

    # PNG versions for PWA/manifest
    for sz, name in [(192, "icon-192.png"), (512, "icon-512.png")]:
        png_img = svg_to_png(svg_path, sz)
        png_path = os.path.join(out_dir, name)
        png_img.save(png_path, "PNG")
        print(f"  Saved {png_path}")

    # Copy SVG to release dir
    svg_dst = os.path.join(out_dir, "icon.svg")
    shutil.copy2(svg_path, svg_dst)
    print(f"  Copied SVG to {svg_dst}")

    print("Done!")


if __name__ == "__main__":
    main()
