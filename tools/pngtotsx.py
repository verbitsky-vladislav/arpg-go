#!/usr/bin/env python3
"""PNG → TSX: генерирует пустой Tiled-тайлсет (.tsx) из PNG.

Из картинки берётся только сетка тайлов (columns/tilecount по размеру тайла).
Разметку рельефов (wangsets) картинка не несёт — она ставится потом в Tiled
или выучивается с карты-примера. Тайл по умолчанию 16px.

    python3 tools/pngtotsx.py <png> [--tile 16] [--out path.tsx]
    python3 tools/pngtotsx.py <dir>  --batch [--recursive] [--tile 16]

Батч устойчив: файл не кратный тайлу — пропускается с пометкой, а не роняет прогон.

По умолчанию .tsx кладётся рядом с PNG с тем же именем. Существующий .tsx
НЕ перезаписывается без --force (в нём может лежать ручная разметка).
"""
import argparse, os, struct, sys, glob

def png_size(path):
    """Размер PNG без внешних зависимостей: читаем chunk IHDR."""
    with open(path, 'rb') as f:
        sig = f.read(8)
        if sig[:8] != b'\x89PNG\r\n\x1a\n':
            raise ValueError(f'{path}: не PNG')
        f.read(4)                       # длина IHDR
        if f.read(4) != b'IHDR':
            raise ValueError(f'{path}: нет IHDR')
        w, h = struct.unpack('>II', f.read(8))
        return w, h

def make_tsx(png, tile, name=None):
    w, h = png_size(png)
    if w % tile or h % tile:
        raise ValueError(f'{png}: {w}x{h} не кратно {tile}')
    cols, rows = w // tile, h // tile
    count = cols * rows
    name = name or os.path.splitext(os.path.basename(png))[0]
    img = os.path.basename(png)
    return (f'<?xml version="1.0" encoding="UTF-8"?>\n'
            f'<tileset version="1.10" tiledversion="1.12.2" name="{name}" '
            f'tilewidth="{tile}" tileheight="{tile}" tilecount="{count}" columns="{cols}">\n'
            f' <image source="{img}" width="{w}" height="{h}"/>\n'
            f'</tileset>\n'), count

def write_one(png, tile, out, force):
    xml, count = make_tsx(png, tile)
    if out is None:
        out = os.path.splitext(png)[0] + '.tsx'
    if os.path.exists(out) and not force:
        return out, count, 'ПРОПУЩЕН (уже есть, --force для перезаписи)'
    with open(out, 'w', encoding='utf-8') as f:
        f.write(xml)
    return out, count, 'записан'

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('target', help='PNG-файл или каталог (с --batch)')
    ap.add_argument('--tile', type=int, default=16)
    ap.add_argument('--out', default=None)
    ap.add_argument('--batch', action='store_true')
    ap.add_argument('--recursive', action='store_true', help='с --batch: обойти подпапки')
    ap.add_argument('--force', action='store_true')
    ap.add_argument('--print', action='store_true', help='печатать xml в stdout, не писать файл')
    a = ap.parse_args()

    if a.print:
        xml, count = make_tsx(a.target, a.tile)
        sys.stdout.write(xml)
        return
    if a.batch:
        pat = '**/*.png' if a.recursive else '*.png'
        pngs = sorted(glob.glob(os.path.join(a.target, pat), recursive=a.recursive))
        made = skipped = errors = 0
        for p in pngs:
            try:
                out, count, st = write_one(p, a.tile, None, a.force)
                if st == 'записан':
                    made += 1
                else:
                    skipped += 1
                print(f'{os.path.relpath(p, a.target):<48} {count:>5}  {st}')
            except Exception as e:
                errors += 1
                print(f'{os.path.relpath(p, a.target):<48}    —  ПРОПУСК: {e}')
        print(f'\nитог: записано {made}, пропущено(существуют) {skipped}, ошибок/некратных {errors}')
    else:
        out, count, st = write_one(a.target, a.tile, a.out, a.force)
        print(f'{out}  ({count} тайлов)  {st}')

if __name__ == '__main__':
    main()
