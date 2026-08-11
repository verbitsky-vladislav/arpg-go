#!/usr/bin/env bash
# gifgen.sh — собирает анимированные GIF из PNG-кадров для визуальных витрин
# в docs/design/ (heroes.md, skills.md). Не часть игры; нужен ImageMagick 7 (magick).
#
# GIF кладём в docs/design/media/ (НЕ в assets/ — чтобы не раздувать игровой
# бинарник, куда assets вшиваются через //go:embed). Кадры генерит tools/spritegen;
# если поменял base.png — сперва перегони кадры, затем этот скрипт.
#
# Витрины ссылаются на GIF ЧИСТЫМ markdown (![](...)) — без HTML <img>, чтобы
# рендерилось в любом просмотрщике. Значит размер задаём тут (ресайзом), а не
# в разметке: героев ужимаем до 128px (таблица 6 колонок), скиллы — нативные 256px.
#
# Запуск из корня репозитория:
#   bash tools/gifgen.sh
set -euo pipefail

BG="#14141c"                 # тёмный игровой фон (как в бою)
SRC=assets/sprites
OUT=docs/design/media

# действие -> задержка кадра в сотых долях секунды (≈ frameTicks/60, как в игре).
# case, а не declare -A: системный bash 3.2 на macOS не знает ассоц. массивов.
hdelay() {
  case "$1" in
    idle) echo 7 ;;
    walk|loot) echo 5 ;;
    attack|punch|take_damage) echo 3 ;;
    *) echo 4 ;;
  esac
}

gif() { # <delay> <resize|-> <src_frames_dir> <out.gif>   (- = без ресайза)
  if [ "$2" = "-" ]; then
    magick -delay "$1" -loop 0 "$3"/frame_*.png \
      -background "$BG" -alpha remove -alpha off "$4"
  else
    magick -delay "$1" -loop 0 "$3"/frame_*.png \
      -filter point -resize "$2" -background "$BG" -alpha remove -alpha off "$4"
  fi
}

echo "герои… (128px, виды down/up/side)"
for h in assassin barbarian engineer knight mage ranger; do
  for dir in down up side; do
    mkdir -p "$OUT/heroes/$h/$dir"
    for a in idle walk attack punch take_damage loot; do
      gif "$(hdelay "$a")" "50%" "$SRC/heroes/$h/$dir/$a" "$OUT/heroes/$h/$dir/$a.gif"
    done
  done
done

echo "скиллы… (256px)"
for s in dash fireball pierce raskol turret vortex; do
  mkdir -p "$OUT/skills/$s"
  for anim in cast impact; do
    gif 3 "-" "$SRC/skills/$s/$anim" "$OUT/skills/$s/$anim.gif"
  done
done

echo "мобы… (128px, виды down/up/side)"
for m in kusaka puzyr moshka; do
  for dir in down up side; do
    mkdir -p "$OUT/mobs/$m/$dir"
    for a in idle walk attack; do
      gif "$(hdelay "$a")" "50%" "$SRC/mobs/$m/$dir/$a" "$OUT/mobs/$m/$dir/$a.gif"
    done
  done
done

echo "иконки статусов… (128px, статичные PNG на тёмном фоне)"
mkdir -p "$OUT/status"
for f in "$SRC"/status/*.png; do
  magick "$f" -filter point -resize 50% -background "$BG" -flatten "$OUT/status/$(basename "$f")"
done

echo "иконки гемов… (128px, статичные PNG)"
for sub in skill support; do
  mkdir -p "$OUT/gems/$sub"
  for f in "$SRC"/gems/$sub/*.png; do
    magick "$f" -filter point -resize 50% -background "$BG" -flatten "$OUT/gems/$sub/$(basename "$f")"
  done
done

echo "готово: $(find "$OUT" -name '*.gif' -o -name '*.png' | wc -l | tr -d ' ') файлов в $OUT"
