package ui

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// CursorKind — какой курсор рисовать. Системный курсор скрыт (см. main), рисуем
// свой: в интерфейсе — стрелка, в бою — прицел. Сцена выставляет вид в Draw,
// движок перед каждым кадром сбрасывает его на стрелку (см. core.Game.Draw).
type CursorKind int

const (
	CursorArrow CursorKind = iota // меню, книга, настройки
	CursorAim                     // игра: целимся мышью
)

// Cursor — вид курсора текущего кадра.
var Cursor CursorKind

// arrowArt — рисунок стрелки: '#' — обводка, 'X' — заливка.
var arrowArt = []string{
	"#.........",
	"##........",
	"#X#.......",
	"#XX#......",
	"#XXX#.....",
	"#XXXX#....",
	"#XXXXX#...",
	"#XXXXXX#..",
	"#XXX####..",
	"#X#XX#....",
	"##.#XX#...",
	"...#XX#...",
	"....###...",
}

var arrowImg *ebiten.Image

// arrow — картинка стрелки (строится один раз, как глифы шрифта).
func arrow() *ebiten.Image {
	if arrowImg != nil {
		return arrowImg
	}
	w, h := len(arrowArt[0]), len(arrowArt)
	px := make([]byte, w*h*4)
	for y, row := range arrowArt {
		for x := 0; x < w && x < len(row); x++ {
			var c color.RGBA
			switch row[x] {
			case '#':
				c = color.RGBA{0x08, 0x0a, 0x10, 0xff}
			case 'X':
				c = color.RGBA{0xf2, 0xf6, 0xff, 0xff}
			default:
				continue
			}
			i := (y*w + x) * 4
			px[i], px[i+1], px[i+2], px[i+3] = c.R, c.G, c.B, c.A
		}
	}
	arrowImg = ebiten.NewImage(w, h)
	arrowImg.WritePixels(px)
	return arrowImg
}

// DrawCursor рисует курсор в позиции мыши.
func DrawCursor(dst *ebiten.Image) {
	mx, my := ebiten.CursorPosition()
	if Cursor == CursorAim {
		drawAim(dst, float32(mx), float32(my))
		return
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(mx), float64(my)) // остриё стрелки — сама точка
	dst.DrawImage(arrow(), op)
}

// drawAim — прицел с двойной обводкой: читается на любом фоне.
func drawAim(dst *ebiten.Image, x, y float32) {
	line := color.RGBA{0xf2, 0xf6, 0xff, 0xff}
	shadow := color.RGBA{0x08, 0x0a, 0x10, 0xd0}

	vector.StrokeCircle(dst, x, y, 6, 2, shadow, true)
	vector.StrokeCircle(dst, x, y, 6, 1, line, true)
	for _, t := range [][4]float32{{-9, 0, -4, 0}, {9, 0, 4, 0}, {0, -9, 0, -4}, {0, 9, 0, 4}} {
		vector.StrokeLine(dst, x+t[0], y+t[1], x+t[2], y+t[3], 2, shadow, true)
		vector.StrokeLine(dst, x+t[0], y+t[1], x+t[2], y+t[3], 1, line, true)
	}
	vector.FillCircle(dst, x, y, 1, line, true)
}
