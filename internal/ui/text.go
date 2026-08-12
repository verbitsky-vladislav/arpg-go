package ui

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"golang.org/x/image/font/basicfont"
)

// face — единый растровый шрифт 7×13 для цветного текста (числа урона и пр.).
// DebugPrint остаётся для служебных надписей; этот путь нужен там, где важен цвет.
var face = text.NewGoXFace(basicfont.Face7x13)

// DrawText печатает s в точке (x,y) — левый верхний угол — цветом col.
func DrawText(dst *ebiten.Image, s string, x, y float64, col color.Color) {
	op := &text.DrawOptions{}
	op.GeoM.Translate(x, y)
	op.ColorScale.ScaleWithColor(col)
	text.Draw(dst, s, face, op)
}

// DrawTextCentered печатает s так, чтобы его центр был в x, верх — y.
func DrawTextCentered(dst *ebiten.Image, s string, x, y float64, col color.Color) {
	w, _ := text.Measure(s, face, 0)
	DrawText(dst, s, x-w/2, y, col)
}
