#!/usr/bin/env python3
"""
Строит манифест тайлсета автоматически из примера карты, который идёт
в комплекте с ассетом. Разметка в Tiled не нужна.

    python3 build_manifest.py --assets ./forest --tmx Tiled_files/Forest.tmx \
                              --biome forest --out forest.manifest.json

Идея: художник в примере расставил тайлы правильно. Для каждой клетки
считаем 8-битную blob-маску по соседям того же логического слоя и
записываем, какой тайл художник туда положил. Получается таблица
маска -> список тайлов с весами, ровно то, что нужно генератору.

Пример покрывает 13 масок гладкой формы (заливка, 4 стороны, 4 внешних
угла, 4 внутренних). Остальные 34 — тонкие перешейки и одиночные клетки;
они не возникают, если генератор сглаживает рельеф, а на случай если
всё же возникнут, в манифест пишется цепочка запасных масок.
"""

import argparse, json, os, xml.etree.ElementTree as ET
from PIL import Image
from collections import Counter, defaultdict

FLIP = 0x1FFFFFFF  # снять биты отражений Tiled

# логические слои: имя -> (слои TMX в порядке приоритета, роль)
TERRAINS = {
    "water":     (["water"],                       None),
    "ground_a":  (["main_space", "ground",
                   "main_space2"],                 "green"),
    "ground_b":  (["main_space", "ground",
                   "main_space2"],                 "brown"),
    "plateau":   (["elevated_space"],              None),
}
GREEN_MIN = 15   # зелёность выше -> трава, ниже -> грунт
FILL_MASK = 255  # у заливки требуем полную непрозрачность
PALETTES = {
    "ground_spots":  ["spots1", "spots2", "spots3"],
    "water_detail":  ["water_details", "water_details2"],
    "surface_liquid":["water_lilies"],
    "shadow":        ["shadow"],
    "grass":         ["grass_elements", "grass_elements2", "grass_elements3"],
    "hangers":       ["lianas", "lianas2", "lianas3", "lianas4", "lianas5"],
    "reeds":         ["reeds"],
}
PREFABS = ["stairs"]

SIDE = [(0, -1), (1, 0), (0, 1), (-1, 0)]      # N E S W  -> биты 1 2 4 8
CORN = [(1, -1), (1, 1), (-1, 1), (-1, -1)]    # NE SE SW NW -> 16 32 64 128


def load_tmx(path):
    root = ET.parse(path).getroot()
    sheets = []
    for ts in root.findall("tileset"):
        fg = int(ts.attrib["firstgid"])
        if "source" in ts.attrib:
            src = os.path.join(os.path.dirname(path), ts.attrib["source"])
            sub = ET.parse(src).getroot()
            name = sub.attrib["name"]
            cnt = int(sub.attrib["tilecount"])
            cols = int(sub.attrib["columns"])
            img = sub.find("image").attrib["source"]
        else:
            name = ts.attrib["name"]
            cnt = int(ts.attrib["tilecount"])
            cols = int(ts.attrib["columns"])
            img = ts.find("image").attrib["source"]
        sheets.append({"name": name, "firstgid": fg, "tilecount": cnt,
                       "columns": cols, "image": img})
    sheets.sort(key=lambda s: s["firstgid"])

    layers = {}
    for L in root.findall("layer"):
        g = {}
        for ch in L.find("data").findall("chunk"):
            x0, y0 = int(ch.attrib["x"]), int(ch.attrib["y"])
            w = int(ch.attrib["width"])
            vals = [int(v) for v in ch.text.replace("\n", "").split(",") if v.strip()]
            for i, v in enumerate(vals):
                if v:
                    g[(x0 + i % w, y0 + i // w)] = v & FLIP
        layers[L.attrib["name"]] = g
    return sheets, layers


_stat_cache = {}

def tile_stats(assets, tmx_dir, sheets_meta, sheet_name, local):
    """(доля непрозрачных пикселей, зелёность по опаковым пикселям)."""
    key = (sheet_name, local)
    if key in _stat_cache:
        return _stat_cache[key]
    meta = next(s for s in sheets_meta if s["name"] == sheet_name)
    path = os.path.join(assets, tmx_dir, meta["image"])
    if path not in _stat_cache:
        _stat_cache[path] = Image.open(path).convert("RGBA")
    im = _stat_cache[path]
    r, c = divmod(local, meta["columns"])
    t = im.crop((c * 16, r * 16, c * 16 + 16, r * 16 + 16))
    rgb = list(t.convert("RGB").getdata())
    al = list(t.getchannel("A").getdata())
    op = [p for p, av in zip(rgb, al) if av > 16]
    cov = len(op) / 256
    if not op:
        res = (0.0, 0)
    else:
        R = sum(p[0] for p in op) / len(op)
        G = sum(p[1] for p in op) / len(op)
        B = sum(p[2] for p in op) / len(op)
        res = (cov, G - max(R, B))
    _stat_cache[key] = res
    return res


def resolve(sheets, gid):
    hit = None
    for s in sheets:
        if s["firstgid"] <= gid < s["firstgid"] + s["tilecount"]:
            hit = (s["name"], gid - s["firstgid"])
    return hit


def blob_mask(occ, x, y):
    s = 0
    for i, (dx, dy) in enumerate(SIDE):
        if (x + dx, y + dy) in occ:
            s |= 1 << i
    m = s
    for i, (dx, dy) in enumerate(CORN):
        if (s >> i & 1) and (s >> ((i + 1) % 4) & 1) and (x + dx, y + dy) in occ:
            m |= 1 << (4 + i)
    return m


def valid_masks():
    out = []
    for m in range(256):
        ok = True
        for i in range(4):
            if (m >> (4 + i) & 1) and not ((m >> i & 1) and (m >> ((i + 1) % 4) & 1)):
                ok = False
        if ok:
            out.append(m)
    return out


def fallback_for(mask, covered):
    """Ближайшая покрытая маска: сначала по совпадению сторон, потом по Хэммингу."""
    sides = mask & 0x0F
    same = [m for m in covered if (m & 0x0F) == sides]
    pool = same or list(covered)
    return min(pool, key=lambda m: bin(m ^ mask).count("1"))


def connected_components(cells):
    seen, comps = set(), []
    for p in cells:
        if p in seen:
            continue
        stack, comp = [p], []
        seen.add(p)
        while stack:
            q = stack.pop()
            comp.append(q)
            for dx, dy in SIDE + CORN:
                n = (q[0] + dx, q[1] + dy)
                if n in cells and n not in seen:
                    seen.add(n)
                    stack.append(n)
        comps.append(comp)
    return comps


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--assets", default=".")
    ap.add_argument("--tmx", required=True)
    ap.add_argument("--biome", required=True)
    ap.add_argument("--out", default="manifest.json")
    a = ap.parse_args()

    tmx = os.path.join(a.assets, a.tmx)
    sheets, layers = load_tmx(tmx)
    VALID = valid_masks()

    man = {"biome": a.biome, "tile_size": 16, "render_scale": 2,
           "sheets": [{"name": s["name"], "file": os.path.join(
                          os.path.dirname(a.tmx), s["image"]),
                       "columns": s["columns"], "tilecount": s["tilecount"]}
                      for s in sheets],
           "terrains": {}, "palettes": {}, "prefabs": {}}
    report = []

    # ---- terrains с blob-таблицей ----
    for tname, (lnames, _role) in TERRAINS.items():
        present = [n for n in lnames if n in layers]
        occ = set()
        for n in present:
            occ |= set(layers[n])
        if not occ:
            report.append(f"{tname}: слоёв нет, пропущен")
            continue
        m2t = defaultdict(Counter)
        for p in occ:
            gid = None
            for n in present:
                if p in layers[n]:
                    gid = layers[n][p]
                    break
            hit = resolve(sheets, gid)
            if not hit:
                continue
            mk = blob_mask(occ, *p)
            cov, green = tile_stats(a.assets, os.path.dirname(a.tmx),
                                    sheets, hit[0], hit[1])
            if cov == 0:
                continue
            if mk == FILL_MASK and cov < 0.999:
                continue                       # дырявая заливка
            if _role == "green" and green < GREEN_MIN:
                continue
            if _role == "brown" and green >= GREEN_MIN:
                continue
            m2t[mk][hit] += 1

        covered = sorted(k for k in m2t if k in VALID)
        table = {}
        for m in sorted(m2t):
            items = [{"sheet": s, "tile": t, "weight": w}
                     for (s, t), w in m2t[m].most_common()]
            table[str(m)] = items
        fb = {str(m): fallback_for(m, covered) for m in VALID if m not in covered}
        man["terrains"][tname] = {"masks": table, "fallback": fb,
                                  "sheets_used": sorted({s for c in m2t.values()
                                                         for (s, _t) in c})}
        report.append(f"{tname:<10} клеток {len(occ):>5}  масок покрыто "
                      f"{len(covered):>2}/47  вариантов всего "
                      f"{sum(len(v) for v in m2t.values()):>4}  "
                      f"листы: {', '.join(man['terrains'][tname]['sheets_used'])}")

    # ---- палитры: просто списки тайлов с весами ----
    for pname, lnames in PALETTES.items():
        c = Counter()
        for n in lnames:
            for gid in layers.get(n, {}).values():
                hit = resolve(sheets, gid)
                if hit:
                    c[hit] += 1
        if not c:
            continue
        man["palettes"][pname] = [{"sheet": s, "tile": t, "weight": w}
                                  for (s, t), w in c.most_common()]
        report.append(f"{pname:<10} палитра: {len(c)} тайлов, {sum(c.values())} клеток")

    # ---- префабы: связные куски, сохраняем форму целиком ----
    for pname in PREFABS:
        g = layers.get(pname, {})
        if not g:
            continue
        out = []
        for comp in connected_components(set(g)):
            x0 = min(p[0] for p in comp); y0 = min(p[1] for p in comp)
            cells = []
            for p in sorted(comp, key=lambda q: (q[1], q[0])):
                hit = resolve(sheets, g[p])
                if hit:
                    cells.append({"dx": p[0] - x0, "dy": p[1] - y0,
                                  "sheet": hit[0], "tile": hit[1]})
            out.append({"w": max(c["dx"] for c in cells) + 1,
                        "h": max(c["dy"] for c in cells) + 1,
                        "cells": cells})
        man["prefabs"][pname] = out
        report.append(f"{pname:<10} префабов: {len(out)} "
                      f"(размеры: {', '.join(f'{p['w']}x{p['h']}' for p in out)})")

    json.dump(man, open(a.out, "w", encoding="utf-8"),
              ensure_ascii=False, indent=1)

    print(f"\nМанифест -> {a.out}\n")
    for line in report:
        print("  " + line)
    print("""
Как читать покрытие: для гладкого острова нужны 13 масок — заливка 255,
четыре стороны 55/110/155/205, четыре внешних угла 19/38/76/137,
четыре внутренних 127/191/223/239. Если они есть, генератору хватит.
Остальные маски — тонкие перешейки; держи в генераторе сглаживание,
и они не появятся. На всякий случай в манифесте есть fallback.""")


if __name__ == "__main__":
    main()
