package ui

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"

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
