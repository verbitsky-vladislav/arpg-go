#!/usr/bin/env python3
"""Frame-accurate normalization for animal packs, driven by the CraftPix .tmx.

Two source layouts, unified to the same per-animation-sheet shape as the
creature packs (idle.png/walk.png/... + manifest.json, 4-directional grid):

  wild : already per-animation sheets. Frame is 32px, 4 dirs; frames = W/32.
  farm : one combined atlas per animal. The .tmx encodes frame size (stride*16)
         and the row layout (groups of 4 direction-rows = one animation). We
         slice the atlas into per-animation sheets accordingly.

Frame SIZE / COUNT / DIRECTIONS are derived exactly from the sheet dims + tmx.
Farm animation NAMES are a heuristic (by frame count; water animals get swim
variants) — flagged in the manifest as such; rename freely.

Usage: animals.py <animals_dir> <raw_dir_with_tmx>
  <animals_dir> : assets/mobs/animals (already merged & named; mutated in place)
  <raw_dir>     : dir holding original packs with Tiled/Animals.tmx (for farm geometry)
"""
import glob
import json
import os
import struct
import subprocess
import sys
import xml.etree.ElementTree as ET
from collections import defaultdict

ANIMALS = sys.argv[1]
RAW = sys.argv[2]
FPS = 7
WILD_FRAME = 32
WATER = {"duck", "drake", "duckling"}

LOOP = {"idle", "walk", "run", "flight", "swim", "swim_idle"}


def png_size(path):
    with open(path, "rb") as f:
        head = f.read(24)
    w, h = struct.unpack(">II", head[16:24])
    return int(w), int(h)


def load_farm_geometry():
    """animal -> {frame, groups:[nframes per animation]} from farm tmx files."""
    geo = {}
    for tmx in glob.glob(f"{RAW}/farm_animals_*/Tiled/*.tmx"):
        root = ET.parse(tmx).getroot()
        for ts in root.findall("tileset"):
            name = ts.get("name")
            if "shad" in name.lower():
                continue
            animal = name.lower().replace("_animation", "")
            cols = int(ts.get("columns"))
            img = ts.find("image")
            W, H = int(img.get("width")), int(img.get("height"))
            tiles = ts.findall("tile")
            if not tiles:
                continue
            f0 = [int(f.get("tileid")) for f in tiles[0].find("animation").findall("frame")]
            stride = (f0[1] - f0[0]) if len(f0) > 1 else 1
            frame = stride * 16
            step = frame // 16
            rowgroups = defaultdict(set)
            for t in tiles:
                tid = int(t.get("id"))
                nf = len(t.find("animation").findall("frame"))
                rowgroups[(tid // cols) // step].add(nf)
            frows = [sorted(rowgroups[r])[0] for r in sorted(rowgroups)]
            # collapse 4 direction-rows -> one animation each
            anims = [frows[i] for i in range(0, len(frows), 4)]
            geo[animal] = {"frame": frame, "anims": anims, "W": W, "H": H}
    return geo


def name_farm_anims(animal, counts):
    n = len(counts)
    if animal in WATER and n == 4:
        return ["walk", "idle", "swim", "swim_idle"]
    names, used = [], defaultdict(int)
    for c in counts:
        base = "idle" if c <= 4 else ("walk" if c <= 6 else "run")
        used[base] += 1
        names.append(base if used[base] == 1 else f"{base}_{used[base]}")
    return names


def write_manifest(folder, name, category, frame, anims, heuristic_names):
    man = {
        "name": name,
        "category": category,
        "frame": {"w": frame, "h": frame},
        "directions": ["down", "up", "left", "right"],
        "animations": anims,
    }
    if heuristic_names:
        man["anim_names"] = "heuristic (frame-count / water); verify in-engine, rename freely"
    with open(os.path.join(folder, "manifest.json"), "w") as f:
        json.dump(man, f, indent=2)
        f.write("\n")


def do_wild(folder, name):
    anims = {}
    for fn in sorted(os.listdir(folder)):
        if not fn.endswith(".png"):
            continue
        anim = fn[:-4]
        w, h = png_size(os.path.join(folder, fn))
        anims[anim] = {"file": fn, "frames": w // WILD_FRAME, "fps": FPS, "loop": anim in LOOP}
    write_manifest(folder, name, "animal", WILD_FRAME, anims, heuristic_names=False)
    print(f"wild {name}: frame={WILD_FRAME} dirs=4 anims={ {a: v['frames'] for a, v in anims.items()} }")


def do_farm(folder, name, geo):
    g = geo[name]
    frame, counts = g["frame"], g["anims"]
    atlas = os.path.join(folder, "atlas.png")
    names = name_farm_anims(name, counts)
    band = 4 * frame  # 4 direction-rows per animation
    anims = {}
    for i, (anim, nf) in enumerate(zip(names, counts)):
        out = f"{anim}.png"
        w = nf * frame
        # crop: full width of nf frames, the i-th 4-row band
        subprocess.run(
            ["magick", atlas, "-crop", f"{w}x{band}+0+{i*band}", "+repage", os.path.join(folder, out)],
            check=True,
        )
        anims[anim] = {"file": out, "frames": nf, "fps": FPS, "loop": anim in LOOP}
    os.remove(atlas)
    write_manifest(folder, name, "animal", frame, anims, heuristic_names=True)
    print(f"farm {name}: frame={frame} dirs=4 anims={ {a: v['frames'] for a, v in anims.items()} }")


def main():
    farm_geo = load_farm_geometry()
    for name in sorted(os.listdir(ANIMALS)):
        folder = os.path.join(ANIMALS, name)
        if not os.path.isdir(folder):
            continue
        if os.path.exists(os.path.join(folder, "atlas.png")):
            if name not in farm_geo:
                print(f"  WARN no tmx geometry for farm animal {name}; skipping")
                continue
            do_farm(folder, name, farm_geo)
        else:
            do_wild(folder, name)


if __name__ == "__main__":
    main()
