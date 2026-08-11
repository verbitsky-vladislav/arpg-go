package ui

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// charW — примерная ширина символа отладочного шрифта ebitenutil (px).
const charW = 6

// Button — простая прямоугольная кнопка. Состояние (ховер/клик) определяет
// вызывающая сцена — виджет лишь проверяет попадание и рисует себя.
type Button struct {
	X, Y, W, H float32
	Label      string
}

// Contains — попадает ли точка (mx,my) в кнопку.
func (b Button) Contains(mx, my int) bool {
	fx, fy := float32(mx), float32(my)
	return fx >= b.X && fx < b.X+b.W && fy >= b.Y && fy < b.Y+b.H
}

// Draw рисует кнопку; hovered подсвечивает её.
func (b Button) Draw(dst *ebiten.Image, hovered bool) {
	bg := color.RGBA{0x22, 0x26, 0x33, 0xff}
	border := color.RGBA{0x6a, 0x76, 0x9e, 0xff}
	if hovered {
		bg = color.RGBA{0x3a, 0x46, 0x66, 0xff}
		border = color.RGBA{0xc8, 0xd4, 0xff, 0xff}
	}
	vector.DrawFilledRect(dst, b.X, b.Y, b.W, b.H, bg, false)
	vector.StrokeRect(dst, b.X, b.Y, b.W, b.H, 1, border, false)
	tx := int(b.X + (b.W-float32(len(b.Label)*charW))/2)
	ty := int(b.Y + (b.H-16)/2)
	ebitenutil.DebugPrintAt(dst, b.Label, tx, ty)
}

// TextCentered печатает s так, чтобы его центр был в x (по горизонтали), y — верх.
func TextCentered(dst *ebiten.Image, s string, x, y int) {
	ebitenutil.DebugPrintAt(dst, s, x-len(s)*charW/2, y)
}

// DrawSpriteFit рисует img по центру (cx,cy) с масштабом так, чтобы высота
// стала targetH, сохраняя пропорции. Пиксель-арт остаётся чётким (nearest).
func DrawSpriteFit(dst, img *ebiten.Image, cx, cy, targetH float64) {
	DrawSpriteFitFlip(dst, img, cx, cy, targetH, false)
}

// DrawSpriteFitFlip — как DrawSpriteFit, но при flipX зеркалит по горизонтали
// (для видов влево/вправо, которые переиспользуют фронт).
func DrawSpriteFitFlip(dst, img *ebiten.Image, cx, cy, targetH float64, flipX bool) {
	if img == nil {
		return
	}
	iw := float64(img.Bounds().Dx())
	ih := float64(img.Bounds().Dy())
	if ih == 0 {
		return
	}
	sc := targetH / ih
	op := &ebiten.DrawImageOptions{}
	if flipX {
		op.GeoM.Scale(-sc, sc)
		op.GeoM.Translate(cx+iw*sc/2, cy-ih*sc/2)
	} else {
		op.GeoM.Scale(sc, sc)
		op.GeoM.Translate(cx-iw*sc/2, cy-ih*sc/2)
	}
	dst.DrawImage(img, op)
}
