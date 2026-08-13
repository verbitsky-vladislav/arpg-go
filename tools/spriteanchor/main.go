// Команда spriteanchor считает по пикселям точку опоры спрайт-паков и
// вписывает её в их manifest.json.
//
// Кадр в листе — это коробка 32×32 (или 16/64), но животное занимает в ней не
// всю площадь: сверху обычно пустота, снизу — тень под лапами. Чтобы поставить
// зверя на тайл, нужно знать не размер кадра, а где у него земля. Это и есть
// anchor: смещение внутри кадра, которое совмещается с позицией существа в мире.
// По нему же идёт сортировка по глубине (кто за кем) и привязка хитбокса.
//
// Считается автоматически, потому что вручную это 30 паков × 3 размера кадра.
// Алгоритм: пробегаем непрозрачные пиксели, берём их общую рамку (bbox);
// anchor.X — середина рамки, anchor.Y — её низ. Низ берётся по анимации
// «стояния на земле» (idle → walk → sitting → первая по алфавиту): в death или
// flight зверь лежит/летит, и земля по ним считалась бы неверно.
//
// Данные производные от графики, поэтому им место в сгенерированном манифесте,
// а не в species.json. Запускать после assetnorm, который манифесты переписывает.
//
//	go run ./tools/spriteanchor                       # assets/mobs/animals
//	go run ./tools/spriteanchor -root assets/mobs/enemies
//	go run ./tools/spriteanchor -dry                  # только показать
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	_ "image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// manifest — структура файла с сохранением порядка полей при перезаписи.
type manifest struct {
	ID         string             `json:"id,omitempty"`
	Name       string             `json:"name"`
	Type       string             `json:"type,omitempty"`
	Category   string             `json:"category"`
	SourcePack string             `json:"source_pack,omitempty"`
	Frame      size               `json:"frame"`
	Directions []string           `json:"directions"`
	Animations map[string]animDef `json:"animations"`
	BBox       *rect              `json:"bbox,omitempty"`
	Anchor     *point             `json:"anchor,omitempty"`
	AnimNames  string             `json:"anim_names,omitempty"`
}

type size struct {
	W int `json:"w"`
	H int `json:"h"`
}

type point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// rect — рамка непрозрачных пикселей внутри кадра.
type rect struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

type animDef struct {
	File   string `json:"file"`
	Frames int    `json:"frames"`
	FPS    int    `json:"fps"`
	Loop   bool   `json:"loop"`
}

// groundAnims — анимации, в которых зверь стоит или идёт по земле: только по
// ним считается уровень опоры. Плавание исключено намеренно — там тело сидит по
// ватерлинию, и низ рамки означает не землю, а глубину посадки в воде.
var groundAnims = map[string]bool{"idle": true, "walk": true, "sitting": true}

func main() {
	root := flag.String("root", "assets/mobs/animals", "каталог с паками")
	dry := flag.Bool("dry", false, "не записывать, только показать")
	flag.Parse()

	paths, err := findManifests(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(paths) == 0 {
		fmt.Fprintf(os.Stderr, "паков в %s не найдено\n", *root)
		os.Exit(1)
	}

	fail := 0
	for _, p := range paths {
		if err := process(p, *dry); err != nil {
			fmt.Fprintln(os.Stderr, "ОШИБКА", err)
			fail++
		}
	}
	fmt.Printf("паков: %d, ошибок: %d\n", len(paths), fail)
	if fail > 0 {
		os.Exit(1)
	}
}

func findManifests(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == "manifest.json" {
			out = append(out, p)
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}

func process(mp string, dry bool) error {
	b, err := os.ReadFile(mp)
	if err != nil {
		return err
	}
	var m manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return fmt.Errorf("%s: %w", mp, err)
	}
	if m.Frame.W <= 0 || m.Frame.H <= 0 || len(m.Directions) == 0 {
		return fmt.Errorf("%s: нет размера кадра или направлений", mp)
	}
	dir := filepath.Dir(mp)

	var all rect      // рамка по всем анимациям — для отсечения и отрисовки
	var ground rect   // рамка по наземным анимациям — из неё берётся anchor
	var used []string // какие анимации дали землю (для отчёта)
	for _, name := range sortedAnims(m) {
		def := m.Animations[name]
		r, err := sheetBBox(filepath.Join(dir, def.File), m.Frame, def.Frames, len(m.Directions))
		if err != nil {
			return fmt.Errorf("%s/%s: %w", dir, name, err)
		}
		if r.W == 0 {
			continue // полностью прозрачный лист — в рамку не вносим
		}
		all = union(all, r)
		if groundAnims[name] {
			ground = union(ground, r)
			used = append(used, name)
		}
	}
	if all.W == 0 {
		return fmt.Errorf("%s: все листы прозрачные", dir)
	}
	if ground.W == 0 {
		// Наземных анимаций нет вовсе (например, только полёт) — считаем по всему.
		ground, used = all, []string{"все"}
	}

	m.BBox = &rect{all.X, all.Y, all.W, all.H}
	m.Anchor = &point{X: ground.X + ground.W/2, Y: ground.Y + ground.H}

	fmt.Printf("%-42s кадр %dx%d  bbox %d,%d %dx%d  anchor %d,%d  (по %s)\n",
		dir, m.Frame.W, m.Frame.H, all.X, all.Y, all.W, all.H,
		m.Anchor.X, m.Anchor.Y, strings.Join(used, "+"))
	if dry {
		return nil
	}
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(mp, append(out, '\n'), 0o644)
}

// sortedAnims — имена анимаций пака в стабильном порядке (обход map иначе
// случаен, а от порядка зависит вывод отчёта).
func sortedAnims(m manifest) []string {
	names := make([]string, 0, len(m.Animations))
	for n := range m.Animations {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// sheetBBox — рамка непрозрачных пикселей в координатах кадра, общая для всех
// кадров и всех направлений листа.
func sheetBBox(path string, frame size, frames, rows int) (rect, error) {
	f, err := os.Open(path)
	if err != nil {
		return rect{}, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return rect{}, err
	}
	b := img.Bounds()
	if b.Dx() < frames*frame.W || b.Dy() < rows*frame.H {
		return rect{}, fmt.Errorf("лист %dx%d мал для %d кадров %dx%d в %d строк",
			b.Dx(), b.Dy(), frames, frame.W, frame.H, rows)
	}

	minX, minY := frame.W, frame.H
	maxX, maxY := -1, -1
	for row := range rows {
		for col := range frames {
			ox, oy := b.Min.X+col*frame.W, b.Min.Y+row*frame.H
			for y := range frame.H {
				for x := range frame.W {
					if _, _, _, a := img.At(ox+x, oy+y).RGBA(); a == 0 {
						continue
					}
					minX = min(minX, x)
					minY = min(minY, y)
					maxX = max(maxX, x)
					maxY = max(maxY, y)
				}
			}
		}
	}
	if maxX < 0 {
		return rect{}, nil
	}
	return rect{X: minX, Y: minY, W: maxX - minX + 1, H: maxY - minY + 1}, nil
}

func union(a, b rect) rect {
	if a.W == 0 {
		return b
	}
	if b.W == 0 {
		return a
	}
	x1, y1 := min(a.X, b.X), min(a.Y, b.Y)
	x2, y2 := max(a.X+a.W, b.X+b.W), max(a.Y+a.H, b.Y+b.H)
	return rect{X: x1, Y: y1, W: x2 - x1, H: y2 - y1}
}
