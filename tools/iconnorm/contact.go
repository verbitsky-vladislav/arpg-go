package main

// Контактный лист: спрайты листа выкладываются в сетку, под каждым — его
// номер в манифесте. Нужен, чтобы выбирать иконки под предметы глазами:
// в атласе они лежат как попало, а тут видно «этот — 042».

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

const (
	contactCols  = 16
	contactScale = 2
	contactPad   = 3
	contactLabel = 7 // высота подписи в пикселях листа (до масштаба)
)

// digits — цифры 3x5, по строке на пиксельный ряд.
var digits = [10][5]string{
	{"###", "# #", "# #", "# #", "###"},
	{" # ", "## ", " # ", " # ", "###"},
	{"###", "  #", "###", "#  ", "###"},
	{"###", "  #", "###", "  #", "###"},
	{"# #", "# #", "###", "  #", "  #"},
	{"###", "#  ", "###", "  #", "###"},
	{"###", "#  ", "###", "# #", "###"},
	{"###", "  #", "  #", "  #", "  #"},
	{"###", "# #", "###", "# #", "###"},
	{"###", "# #", "###", "  #", "###"},
}

func contactPack(dir, outDir string) error {
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return err
	}
	var man Manifest
	if err := json.Unmarshal(raw, &man); err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	for _, key := range sortedSheets(man.Sheets) {
		sh := man.Sheets[key]
		if len(sh.Sprites) == 0 {
			continue
		}
		src, err := decode(filepath.Join(dir, sh.File))
		if err != nil {
			return err
		}
		cw, ch := 0, 0
		for _, s := range sh.Sprites {
			cw, ch = max(cw, s.W), max(ch, s.H)
		}
		cw += contactPad * 2
		ch += contactPad*2 + contactLabel
		rows := (len(sh.Sprites) + contactCols - 1) / contactCols
		dst := image.NewRGBA(image.Rect(0, 0, contactCols*cw*contactScale, rows*ch*contactScale))
		for y := dst.Bounds().Min.Y; y < dst.Bounds().Max.Y; y++ {
			for x := dst.Bounds().Min.X; x < dst.Bounds().Max.X; x++ {
				dst.SetRGBA(x, y, color.RGBA{28, 30, 38, 255})
			}
		}
		for i, s := range sh.Sprites {
			cx := (i % contactCols) * cw
			cy := (i / contactCols) * ch
			// ячейка чуть светлее фона, чтобы границы были видны
			fillCell(dst, cx*contactScale, cy*contactScale, (cw-1)*contactScale, (ch-1)*contactScale,
				color.RGBA{44, 47, 58, 255})
			ox := cx + contactPad + (cw-contactPad*2-s.W)/2
			oy := cy + contactPad
			blitScaled(dst, src, s, ox, oy)
			drawNum(dst, i+1, cx+contactPad, cy+ch-contactLabel)
		}
		name := fmt.Sprintf("%s_%s_contact.png", man.ID, key)
		f, err := os.Create(filepath.Join(outDir, name))
		if err != nil {
			return err
		}
		err = png.Encode(f, dst)
		f.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func fillCell(dst *image.RGBA, x, y, w, h int, c color.RGBA) {
	for j := y; j < y+h; j++ {
		for i := x; i < x+w; i++ {
			dst.SetRGBA(i, j, c)
		}
	}
}

func blitScaled(dst *image.RGBA, src image.Image, s Sprite, ox, oy int) {
	b := src.Bounds()
	for y := 0; y < s.H; y++ {
		for x := 0; x < s.W; x++ {
			c := src.At(b.Min.X+s.X+x, b.Min.Y+s.Y+y)
			if _, _, _, a := c.RGBA(); a == 0 {
				continue
			}
			for dy := 0; dy < contactScale; dy++ {
				for dx := 0; dx < contactScale; dx++ {
					dst.Set((ox+x)*contactScale+dx, (oy+y)*contactScale+dy, c)
				}
			}
		}
	}
}

func drawNum(dst *image.RGBA, n, x, y int) {
	s := fmt.Sprint(n)
	col := color.RGBA{150, 210, 255, 255}
	for k, ch := range s {
		d := int(ch - '0')
		if d < 0 || d > 9 {
			continue
		}
		for row := 0; row < 5; row++ {
			for colx := 0; colx < 3; colx++ {
				if digits[d][row][colx] != '#' {
					continue
				}
				px, py := x+k*4+colx, y+row
				for dy := 0; dy < contactScale; dy++ {
					for dx := 0; dx < contactScale; dx++ {
						dst.SetRGBA(px*contactScale+dx, py*contactScale+dy, col)
					}
				}
			}
		}
	}
}
