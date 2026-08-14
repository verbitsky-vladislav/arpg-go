// trace снимает слои готового направления в текстовые сетки — с них
// начинается рисование диагонали.
//
// Пишутся покадрово все слои кадра, кроме головы: тело, оружие, росчерк
// замаха, вспышка попадания. Голова рисуется вручную один раз на
// направление, поэтому вместо неё пишутся её покадровые сдвиги (poses.txt).
//
// Сдвиги считаются не от нулевого кадра клипа, а от положения головы в
// нулевом кадре ходьбы: именно туда посажена нарисованная голова, а у
// каждого клипа своя нулевая точка — в walk_attack, например, голова
// начинает на клетку левее и на две выше, чем в walk.
//
// Уже существующие файлы не перезаписываются: в сетки тела вносится
// правка под диагональ, и повторный прогон не должен её стирать.
// Перерисовать заново — удалить файл и запустить снова.
//
// usage: go run ./tools/dir8/trace <packDir> <clip> <right|up> <outDir>
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/vladislav/game/tools/dir8/internal/grid"
)

const frame = grid.FrameSize

// rowOf — строка направления в листах пака: down, left, right, up.
var rowOf = map[string]int{"right": 2, "up": 3}

func main() {
	if len(os.Args) != 5 {
		fmt.Fprintln(os.Stderr, "usage: trace <packDir> <clip> <right|up> <outDir>")
		os.Exit(2)
	}
	pack, clip, side, out := os.Args[1], os.Args[2], os.Args[3], os.Args[4]
	check(grid.UsePalette(filepath.Join(pack, "dir8", "palette.txt")))
	row, ok := rowOf[side]
	if !ok {
		check(fmt.Errorf("сторона %q: бывают right и up", side))
	}
	check(os.MkdirAll(out, 0o755))

	// Оружие лежит в двух слоях: спереди оно перед корпусом, со спины —
	// за ним, и непустой из двух как раз тот, что нужен этой стороне.
	weapon := "_front.png"
	if side == "up" {
		weapon = "_back.png"
	}
	layers := map[string]string{
		"body":  "_body.png",
		"sword": weapon,
		"swing": "_swing.png", // только у атак
		"red":   "_red.png",   // вспышка попадания, только у hurt и death
		// Голова обычно нарисована вручную одна на направление, но в
		// смерти она лежит лицом вниз, а в уроне зажмурена — там нужен
		// покадровый оригинал. Лишние head<N>.txt удаляются вручную.
		"head": "_head.png",
	}

	body, err := findLayer(pack, clip, layers["body"])
	check(err)
	frames := body.Bounds().Dx() / frame
	drawn := drawnFrames(body, row)
	if drawn < frames {
		fmt.Printf("  в ряду нарисовано %d кадров из %d — недостающие берутся по кругу\n", drawn, frames)
	}

	for _, name := range []string{"body", "sword", "swing", "red", "head"} {
		sheet, err := findLayer(pack, clip, layers[name])
		if err != nil {
			continue // не все слои есть у всех клипов
		}
		for i := range frames {
			f := cut(sheet, src(i, drawn), row)
			b, ok := bbox(f)
			if !ok {
				continue // слоя в этом кадре нет — файла не будет
			}
			write(filepath.Join(out, fmt.Sprintf("%s%d.txt", name, i)), dump(f, b))
		}
	}

	check(writePoses(pack, clip, row, frames, drawn, out))
	for c, n := range far {
		fmt.Printf("  ВНИМАНИЕ: цвета #%02X%02X%02X a=%d в палитре нет, взят ближайший (%d px)\n",
			c.R, c.G, c.B, c.A, n)
	}
}

// writePoses пишет покадровые сдвиги головы и оружия относительно их
// положения в нулевом кадре ходьбы.
func writePoses(pack, clip string, row, frames, drawn int, out string) error {
	base := func(part string) (image.Point, error) {
		l, err := findLayer(pack, "walk", part)
		if err != nil {
			return image.Point{}, err
		}
		b, ok := bbox(cut(l, 0, row))
		if !ok {
			return image.Point{}, fmt.Errorf("в нулевом кадре ходьбы пуст слой %s", part)
		}
		return b.Min, nil
	}
	headBase, err := base("_head.png")
	if err != nil {
		return err
	}
	weapon := "_front.png"
	if row == rowOf["up"] {
		weapon = "_back.png"
	}
	swordBase, swordErr := base(weapon)

	head, err := findLayer(pack, clip, "_head.png")
	if err != nil {
		return err
	}
	sword, _ := findLayer(pack, clip, weapon)

	var sb strings.Builder
	sb.WriteString("# сдвиги от нулевого кадра ходьбы: головы и оружия.\n")
	sb.WriteString("# по строке на кадр: headDX headDY swordDX swordDY\n")
	for i := range frames {
		var dx, dy, sx, sy int
		if b, ok := bbox(cut(head, src(i, drawn), row)); ok {
			dx, dy = b.Min.X-headBase.X, b.Min.Y-headBase.Y
		}
		if sword != nil && swordErr == nil {
			if b, ok := bbox(cut(sword, src(i, drawn), row)); ok {
				sx, sy = b.Min.X-swordBase.X, b.Min.Y-swordBase.Y
			}
		}
		fmt.Fprintf(&sb, "%d %d %d %d\n", dx, dy, sx, sy)
	}
	write(filepath.Join(out, "poses.txt"), sb.String())
	return nil
}

// drawnFrames — сколько кадров ряда художник нарисовал на самом деле.
// В верхнем ряду стойки, например, во всех паках нарисованы 4 кадра, а
// манифест обещает 12: хвост листа пустой. Сейчас из-за этого персонаж,
// стоящий спиной, пропадает на две трети цикла.
func drawnFrames(body *image.NRGBA, row int) int {
	n := body.Bounds().Dx() / frame
	for i := range n {
		if _, ok := bbox(cut(body, i, row)); !ok {
			return i
		}
	}
	return n
}

// src — из какого кадра исходника брать кадр i: недостающий хвост
// повторяет нарисованное по кругу.
func src(i, drawn int) int {
	if drawn <= 0 {
		return i
	}
	return i % drawn
}

// write не трогает существующий файл: ручные правки важнее свежей трассы.
func write(path, body string) {
	if _, err := os.Stat(path); err == nil {
		fmt.Println("  пропустил (уже есть):", path)
		return
	}
	check(os.WriteFile(path, []byte(body), 0o644))
}

// findLayer ищет слой клипа: имена в паке вида <оружие>_<clip><N>_<часть>.png,
// где N задаёт порядок отрисовки и у разных клипов разный.
func findLayer(pack, clip, suffix string) (*image.NRGBA, error) {
	paths, err := filepath.Glob(filepath.Join(pack, "parts", "*_"+clip+"[0-9]*.png"))
	if err != nil {
		return nil, err
	}
	for _, p := range paths {
		if strings.HasSuffix(p, suffix) {
			return loadPNG(p)
		}
	}
	return nil, fmt.Errorf("в %s нет слоя %s клипа %s", pack, suffix, clip)
}

// dump печатает рамку кадра сеткой палитры: строка "@ x y" и пиксели.
func dump(im *image.NRGBA, b image.Rectangle) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "@ %d %d\n", b.Min.X, b.Min.Y)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			sb.WriteByte(glyph(im.NRGBAAt(x, y)))
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

// far — цвета исходника, для которых в палитре не нашлось близкого.
// Молчаливое «взял ближайший» однажды увело красную вспышку в цвет
// тени, и слой поехал; теперь такое видно сразу.
var far = map[color.NRGBA]int{}

// glyph — ближайший цвет палитры пака.
func glyph(c color.NRGBA) byte {
	if c.A == 0 {
		return '.'
	}
	best, bestD := byte('?'), 1<<30
	for g, p := range grid.Palette {
		// Альфа входит в расстояние: вспышка попадания бывает двух
		// плотностей одного цвета, и без неё сильная молча становилась
		// слабой.
		dr, dg, db := int(p.R)-int(c.R), int(p.G)-int(c.G), int(p.B)-int(c.B)
		da := int(p.A) - int(c.A)
		if d := dr*dr + dg*dg + db*db + da*da; d < bestD {
			best, bestD = g, d
		}
	}
	// 24 единицы на канал: соседние тона растяжки ближе, чужой цвет дальше.
	if bestD > 3*24*24 {
		far[c]++
	}
	return best
}

func cut(sheet *image.NRGBA, col, row int) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, frame, frame))
	r := image.Rect(col*frame, row*frame, (col+1)*frame, (row+1)*frame)
	draw.Draw(out, out.Bounds(), sheet, r.Min, draw.Src)
	return out
}

func bbox(im *image.NRGBA) (image.Rectangle, bool) {
	b := im.Bounds()
	minX, minY, maxX, maxY := b.Max.X, b.Max.Y, b.Min.X, b.Min.Y
	found := false
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if im.NRGBAAt(x, y).A == 0 {
				continue
			}
			found = true
			minX, minY = min(minX, x), min(minY, y)
			maxX, maxY = max(maxX, x), max(maxY, y)
		}
	}
	return image.Rect(minX, minY, maxX+1, maxY+1), found
}

func loadPNG(p string) (*image.NRGBA, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	src, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	dst := image.NewNRGBA(src.Bounds())
	draw.Draw(dst, src.Bounds(), src, src.Bounds().Min, draw.Src)
	return dst, nil
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "trace:", err)
		os.Exit(1)
	}
}
