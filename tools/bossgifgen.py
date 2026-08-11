#!/usr/bin/env python3
# bossgifgen.py — собирает анимированные GIF из спрайтшитов боссов для визуальной
# витрины docs/design/bosses.md. Аналог tools/gifgen.sh (герои), но боссы хранят
# кадры не отдельными frame_*.png, а СЕТКОЙ-спрайтшитом (строки = направления,
# столбцы = кадры), поэтому лист надо резать. Параметры берём из <boss>.json.
#
# Не часть игры. Нужен ImageMagick 7 (magick) — им же режем и собираем. GIF кладём
# в docs/design/media/bosses/ (НЕ в assets/ — чтобы не раздувать игровой бинарник,
# куда assets вшиты через //go:embed).
#
# Скорость GIF = fps анимации (delay = 100/fps сотых сек, как в игре).
# Для body/spiderling подкладываем тень с пола (shadows/…) — существо «стоит».
# Показываем строку `down` (морда к камере — самый читаемый ракурс); плюс для
# ходьбы отдельный showcase всех 8 направлений (доказать честную направленность).
#
# Запуск из корня репозитория:
#   python3 tools/bossgifgen.py            # все боссы в assets/sprites/bosses
#   python3 tools/bossgifgen.py weaver     # только weaver
#
# Перед запуском при изменении арта: go run ./tools/weavergen

import json
import os
import subprocess
import sys
import tempfile

BG = "#14141c"                       # тёмный игровой фон (как в бою)
SRC_ROOT = "assets/sprites/bosses"
OUT_ROOT = "docs/design/media/bosses"

# Смещение тени вниз от центра кадра (под нижнюю массу тела), в пикселях финала.
BODY_SHADOW_DY = 20
SPIDER_SHADOW_DY = 6


def run(args):
    subprocess.run(args, check=True)


def crop_tile(sheet, fw, fh, col, row, out):
    """Вырезает один кадр (col,row) из спрайтшита."""
    run(["magick", sheet, "-crop", f"{fw}x{fh}+{col*fw}+{row*fh}", "+repage", out])


def compose_on_bg(tile, fw, fh, out, shadow=None, shadow_dy=0):
    """Кладёт кадр на тёмный фон, при наличии — тень под него."""
    args = ["magick", "-size", f"{fw}x{fh}", f"xc:{BG}"]
    if shadow and os.path.exists(shadow):
        args += [shadow, "-gravity", "center", "-geometry", f"+0+{shadow_dy}", "-composite"]
    args += [tile, "-gravity", "center", "-geometry", "+0+0", "-composite",
             "-alpha", "remove", "-alpha", "off", out]
    run(args)


def assemble_gif(frames, fps, out, scale=None):
    delay = max(2, round(100.0 / fps))
    args = ["magick", "-delay", str(delay), "-loop", "0"] + frames
    if scale:
        args += ["-filter", "point", "-resize", scale]
    args += ["-layers", "optimize", out]
    run(args)


# Тень под кадром по типу части. Кандидаты пробуются по очереди (первый
# существующий): weaver зовёт body_shadow, world_eater — shadow_head/shadow_segment.
def _shadow_for(part, shadow_path):
    cand = {
        "body": (("body_shadow", "shadow_segment"), BODY_SHADOW_DY),
        "head": (("shadow_head",), BODY_SHADOW_DY),
        "tail": (("shadow_segment", "body_shadow"), BODY_SHADOW_DY),
        "spiderling": (("spiderling_shadow",), SPIDER_SHADOW_DY),
    }.get(part)
    if not cand:
        return None, 0
    for base in cand[0]:
        if os.path.exists(shadow_path(base)):
            return shadow_path(base), cand[1]
    return None, 0


def anim_gif(src_dir, out_dir, a, shadow_path):
    """Один GIF анимации из строки `down` (row 0)."""
    fw, fh, frames = a["frame_w"], a["frame_h"], a["frames"]
    part = a["part"]
    sheet = os.path.join(src_dir, a["file"])
    out = os.path.join(out_dir, part, a["name"] + ".gif")
    os.makedirs(os.path.dirname(out), exist_ok=True)

    shadow, dy = _shadow_for(part, shadow_path)

    with tempfile.TemporaryDirectory() as tmp:
        composed = []
        for k in range(frames):
            tile = os.path.join(tmp, f"t{k:03d}.png")
            comp = os.path.join(tmp, f"c{k:03d}.png")
            crop_tile(sheet, fw, fh, k, 0, tile)
            compose_on_bg(tile, fw, fh, comp, shadow, dy)
            composed.append(comp)
        assemble_gif(composed, a["fps"], out)
    return out


def directions_gif(src_dir, out_dir, a, shadow_path):
    """Showcase всех 8 направлений: проигрывает анимацию для каждой строки
    подряд (down → down_left → … ), доказывая честную направленность без
    отражения. Только для многонаправленных анимаций."""
    if a["directions"] < 8:
        return None
    fw, fh, frames = a["frame_w"], a["frame_h"], a["frames"]
    sheet = os.path.join(src_dir, a["file"])
    out = os.path.join(out_dir, "showcase", a["name"] + "_8dir.gif")
    os.makedirs(os.path.dirname(out), exist_ok=True)
    shadow, _ = _shadow_for(a["part"], shadow_path)
    with tempfile.TemporaryDirectory() as tmp:
        composed = []
        for r in range(a["directions"]):
            for k in range(frames):
                tile = os.path.join(tmp, f"t{r}_{k:03d}.png")
                comp = os.path.join(tmp, f"c{r}_{k:03d}.png")
                crop_tile(sheet, fw, fh, k, r, tile)
                compose_on_bg(tile, fw, fh, comp, shadow, BODY_SHADOW_DY)
                composed.append(comp)
        assemble_gif(composed, a["fps"], out)
    return out


def do_boss(name):
    src_dir = os.path.join(SRC_ROOT, name)
    out_dir = os.path.join(OUT_ROOT, name)
    meta_path = os.path.join(src_dir, name + ".json")
    with open(meta_path) as f:
        doc = json.load(f)

    def shadow_path(base):
        return os.path.join(src_dir, "shadows", base + ".png")

    count = 0
    for a in doc["animations"]:
        # legs — статичные сегменты; тени — служебные спрайты, в витрину не идут.
        if a["part"] in ("legs", "shadows"):
            continue
        anim_gif(src_dir, out_dir, a, shadow_path)
        count += 1
    # Направленческий showcase — по ходьбе (weaver: walk_loop; world_eater: head_idle).
    for a in doc["animations"]:
        if a["name"] in ("walk_loop", "head_idle"):
            directions_gif(src_dir, out_dir, a, shadow_path)
            count += 1
    print(f"{name}: {count} GIF → {out_dir}")


def main():
    if subprocess.run(["command", "-v", "magick"], shell=False,
                      capture_output=True).returncode != 0 and not os.path.exists("/opt/homebrew/bin/magick"):
        pass  # magick проверится при первом вызове run()
    bosses = sys.argv[1:]
    if not bosses:
        bosses = [d for d in sorted(os.listdir(SRC_ROOT))
                  if os.path.isfile(os.path.join(SRC_ROOT, d, d + ".json"))]
    for b in bosses:
        do_boss(b)


if __name__ == "__main__":
    main()
