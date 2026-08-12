package ui

import (
	"fmt"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/vladislav/game/internal/config"
)

// ShowFPS — показывать ли счётчик FPS/TPS (переключается в настройках паузы).
var ShowFPS bool

// DrawFPS рисует счётчик кадров/тиков в левом нижнем углу (если включён).
func DrawFPS(screen *ebiten.Image) {
	if !ShowFPS {
		return
	}
	ebitenutil.DebugPrintAt(screen,
		fmt.Sprintf("FPS %.0f  TPS %.0f", ebiten.ActualFPS(), ebiten.ActualTPS()),
		4, config.ScreenH-28)
}

// DrawCursor рисует игровой курсор-прицел в позиции мыши (системный курсор скрыт
// в main). Двойная обводка — читаемость на любом фоне.
func DrawCursor(screen *ebiten.Image) {
	mx, my := ebiten.CursorPosition()
	x, y := float32(mx), float32(my)
	line := color.RGBA{0xf2, 0xf6, 0xff, 0xff}
	shadow := color.RGBA{0x08, 0x0a, 0x10, 0xd0}

	vector.StrokeCircle(screen, x, y, 6, 2, shadow, true)
	vector.StrokeCircle(screen, x, y, 6, 1, line, true)
	for _, t := range [][4]float32{{-9, 0, -4, 0}, {9, 0, 4, 0}, {0, -9, 0, -4}, {0, 9, 0, 4}} {
		vector.StrokeLine(screen, x+t[0], y+t[1], x+t[2], y+t[3], 2, shadow, true)
		vector.StrokeLine(screen, x+t[0], y+t[1], x+t[2], y+t[3], 1, line, true)
	}
	vector.DrawFilledCircle(screen, x, y, 1, line, true)
}
