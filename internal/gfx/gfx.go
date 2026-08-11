// Package gfx — генерация пиксельной графики в коде. Никаких PNG:
// спрайт описывается текстовой картой, где каждый символ — цвет из палитры.
// Из карты строится *ebiten.Image один раз, дальше просто рисуется.
package gfx

import (
	"image"
	"image/color"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
)

// Palette сопоставляет символ карты цвету. Символы ' ' и '.' — прозрачные.
type Palette map[rune]color.Color

// Hex парсит цвет вида "#rrggbb" или "#rrggbbaa".
func Hex(s string) color.Color {
	s = strings.TrimPrefix(s, "#")
	r, _ := strconv.ParseUint(s[0:2], 16, 8)
	g, _ := strconv.ParseUint(s[2:4], 16, 8)
	b, _ := strconv.ParseUint(s[4:6], 16, 8)
	a := uint64(0xff)
	if len(s) >= 8 {
		a, _ = strconv.ParseUint(s[6:8], 16, 8)
	}
	return color.RGBA{uint8(r), uint8(g), uint8(b), uint8(a)}
}

// NewSprite строит спрайт из пиксельной карты.
//
//	rows  — строки одинаковой длины (ASCII); символ → цвет в palette.
//	scale — увеличение: 1 «пиксель» карты = scale×scale реальных пикселей.
//
// Пример: gfx.NewSprite([]string{"XSX","SSS"}, pal, 1).
func NewSprite(rows []string, palette Palette, scale int) *ebiten.Image {
	if scale < 1 {
		scale = 1
	}
	h := len(rows)
	w := 0
	for _, r := range rows {
		if len(r) > w {
			w = len(r)
		}
	}
	img := image.NewRGBA(image.Rect(0, 0, w*scale, h*scale))
	for y, row := range rows {
		for x, ch := range row { // ASCII: индекс байта == столбец
			c, ok := palette[ch]
			if !ok || ch == ' ' || ch == '.' {
				continue
			}
			fillBlock(img, x*scale, y*scale, scale, c)
		}
	}
	return ebiten.NewImageFromImage(img)
}

func fillBlock(img *image.RGBA, px, py, size int, c color.Color) {
	for dy := 0; dy < size; dy++ {
		for dx := 0; dx < size; dx++ {
			img.Set(px+dx, py+dy, c)
		}
	}
}
