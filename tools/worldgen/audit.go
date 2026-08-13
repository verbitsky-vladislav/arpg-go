package main

// audit.go — служебные режимы:
//   audit           — проверить все биомы на манифест и обязательные роли/файлы
//   index <dir> <s> — выгрузить атлас листа с номерами тайлов (помощник авторинга)

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
)

// runAudit проходит по assets/biomes/* и репортит пробелы (E11/E12 в мелком виде).
func runAudit(args []string) {
	root := "assets/biomes"
	if len(args) > 0 {
		root = args[0]
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		fatal(err)
	}
	var withManifest, total int
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		total++
		dir := filepath.Join(root, e.Name())
		m, err := LoadManifest(dir)
		if err != nil {
			fmt.Printf("  %-22s нет манифеста\n", e.Name())
			continue
		}
		withManifest++
		var probs []string
		probs = append(probs, m.validateRoles()...)
		// E12: файлы листов и пропсов существуют
		for name, sh := range m.Sheets {
			if !fileExists(filepath.Join(dir, sh.File)) {
				probs = append(probs, "нет файла листа "+name+": "+sh.File)
			}
		}
		for _, p := range m.Props {
			if !fileExists(filepath.Join(dir, p.File)) {
				probs = append(probs, "нет файла пропса "+p.ID+": "+p.File)
			}
		}
		if len(probs) == 0 {
			fmt.Printf("  %-22s OK (ролей: %d, листов: %d, пропсов: %d)\n",
				e.Name(), len(m.Terrains), len(m.Sheets), len(m.Props))
		} else {
			fmt.Printf("  %-22s %d проблем:\n", e.Name(), len(probs))
			for _, p := range probs {
				fmt.Println("      -", p)
			}
		}
	}
	fmt.Printf("\nитого биомов: %d, с манифестом: %d\n", total, withManifest)
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// runIndex выгружает атлас листа, увеличенный, с сеткой и номерами тайлов —
// чтобы вручную выбрать id для манифеста, глядя на арт (читает только PNG).
func runIndex(args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "использование: index <biome-dir> <sheet-name>")
		os.Exit(2)
	}
	biomeDir, sheetName := args[0], args[1]
	m, err := LoadManifest(biomeDir)
	if err != nil {
		fatal(err)
	}
	sh, ok := m.Sheets[sheetName]
	if !ok {
		fatal(fmt.Errorf("лист %q не найден в манифесте", sheetName))
	}
	src, err := loadImage(filepath.Join(biomeDir, sh.File))
	if err != nil {
		fatal(err)
	}
	ts := m.TileSize
	const scale = 3
	b := src.Bounds()
	cols := sh.Columns
	rows := (b.Dy() + ts - 1) / ts
	out := image.NewRGBA(image.Rect(0, 0, b.Dx()*scale, b.Dy()*scale))
	// сам арт
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			c := src.At(b.Min.X+x, b.Min.Y+y)
			for sy := 0; sy < scale; sy++ {
				for sx := 0; sx < scale; sx++ {
					out.Set(x*scale+sx, y*scale+sy, c)
				}
			}
		}
	}
	// сетка + номера
	gridCol := color.RGBA{255, 0, 128, 200}
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			id := r*cols + c
			if id >= sh.Count {
				continue
			}
			px, py := c*ts*scale, r*ts*scale
			drawRectOutline(out, px, py, ts*scale, ts*scale, gridCol)
			drawInt(out, px+1, py+1, id, color.RGBA{255, 255, 0, 255})
		}
	}
	name := fmt.Sprintf("index_%s.png", sheetName)
	if err := writePNG(name, out); err != nil {
		fatal(err)
	}
	fmt.Printf("%s: %dx%d тайлов (cols=%d) → %s\n", sheetName, cols, rows, cols, name)
}

func drawRectOutline(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	for i := 0; i < w; i++ {
		img.SetRGBA(x+i, y, c)
		img.SetRGBA(x+i, y+h-1, c)
	}
	for i := 0; i < h; i++ {
		img.SetRGBA(x, y+i, c)
		img.SetRGBA(x+w-1, y+i, c)
	}
}

// font3x5 — компактные цифры 0-9 для оверлея номеров тайлов.
var font3x5 = map[rune][5]uint8{
	'0': {0b111, 0b101, 0b101, 0b101, 0b111},
	'1': {0b010, 0b110, 0b010, 0b010, 0b111},
	'2': {0b111, 0b001, 0b111, 0b100, 0b111},
	'3': {0b111, 0b001, 0b111, 0b001, 0b111},
	'4': {0b101, 0b101, 0b111, 0b001, 0b001},
	'5': {0b111, 0b100, 0b111, 0b001, 0b111},
	'6': {0b111, 0b100, 0b111, 0b101, 0b111},
	'7': {0b111, 0b001, 0b010, 0b010, 0b010},
	'8': {0b111, 0b101, 0b111, 0b101, 0b111},
	'9': {0b111, 0b101, 0b111, 0b001, 0b111},
}

func drawInt(img *image.RGBA, x, y, n int, c color.RGBA) {
	s := itoa(n)
	cx := x
	for _, r := range s {
		g, ok := font3x5[r]
		if !ok {
			continue
		}
		for row := 0; row < 5; row++ {
			for col := 0; col < 3; col++ {
				if g[row]&(1<<uint(2-col)) != 0 {
					img.SetRGBA(cx+col, y+row, c)
				}
			}
		}
		cx += 4
	}
}
