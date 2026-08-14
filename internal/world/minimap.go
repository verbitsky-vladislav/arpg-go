package world

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/vladislav/game/internal/engine"
	"github.com/vladislav/game/internal/physics"
)

// Цвета мини-карты: вода, мель, трава, тропа, плато.
var (
	miniDeep    = color.RGBA{0x1e, 0x38, 0x55, 0xff}
	miniShallow = color.RGBA{0x35, 0x62, 0x87, 0xff}
	miniGrass   = color.RGBA{0x4c, 0x7a, 0x3a, 0xff}
	miniTrail   = color.RGBA{0x86, 0x64, 0x3e, 0xff}
	miniPlateau = color.RGBA{0x69, 0x9b, 0x4a, 0xff}
)

// Minimap — обзор всей карты картинкой со стороной side пикселей. Считается
// один раз на каждый размер (в HUD она маленькая, по TAB — во весь экран):
// карта за забег не меняется. Клетки берутся выборкой, а не
// усреднением, — пиксель мини-карты остаётся пикселем, без мыла.
func (m *Map) Minimap(side int) *ebiten.Image {
	if side <= 0 {
		return nil
	}
	if img, ok := m.mini[side]; ok {
		return img
	}
	img := image.NewRGBA(image.Rect(0, 0, side, side))
	for py := range side {
		ty := py * m.h / side
		for px := range side {
			img.Set(px, py, m.cellColor(px*m.w/side, ty))
		}
	}
	if m.mini == nil {
		m.mini = map[int]*ebiten.Image{}
	}
	out := ebiten.NewImageFromImage(img)
	m.mini[side] = out
	return out
}

// cellColor — чем клетка (tx,ty) выглядит на обзоре. Земля определяется по
// слоям арта, а не по физике: обрывы тоже непроходимы, но они не синие.
func (m *Map) cellColor(tx, ty int) color.RGBA {
	i := ty*m.w + tx
	at := func(l []uint16) uint16 {
		if i >= 0 && i < len(l) {
			return l[i]
		}
		return 0
	}
	center := engine.Vec2{
		X: (float64(tx) + 0.5) * float64(m.ts),
		Y: (float64(ty) + 0.5) * float64(m.ts),
	}
	switch {
	case at(m.plateau) != 0:
		return miniPlateau
	case at(m.ground) != 0:
		return miniGrass
	case at(m.mud) != 0:
		return miniTrail
	case m.field.CellAt(center) == physics.Shallow:
		return miniShallow
	}
	return miniDeep
}

// MiniPos — точка мира в координатах мини-карты со стороной side.
func (m *Map) MiniPos(p engine.Vec2, side int) (x, y float32) {
	w, h := m.Size()
	return float32(p.X / w * float64(side)), float32(p.Y / h * float64(side))
}
