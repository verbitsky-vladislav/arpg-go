// Package ui рисует интерфейс поверх мира: шары жизни/маны, счёт.
// Читает характеристики через combat.Stats — не зависит от entity/world.
package ui

import (
	"fmt"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/vladislav/game/internal/combat"
	"github.com/vladislav/game/internal/config"
)

// DrawHUD рисует шары жизни (слева) и маны (справа) и счёт.
func DrawHUD(screen *ebiten.Image, s *combat.Stats, score int) {
	const r = 26
	y := float32(config.ScreenH - r - 6)

	// Жизнь.
	lifeFrac := 0.0
	if s.MaxLife > 0 {
		lifeFrac = s.Life / s.MaxLife
	}
	drawGlobe(screen, r+6, y, r, lifeFrac,
		color.RGBA{0x5a, 0x12, 0x12, 0xff}, color.RGBA{0xe0, 0x30, 0x30, 0xff})

	// Мана.
	manaFrac := 0.0
	if s.MaxMana > 0 {
		manaFrac = s.Mana / s.MaxMana
	}
	drawGlobe(screen, config.ScreenW-r-6, y, r, manaFrac,
		color.RGBA{0x12, 0x22, 0x5a, 0xff}, color.RGBA{0x30, 0x70, 0xe0, 0xff})

	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Kills: %d", score), config.ScreenW/2-24, 8)
	ebitenutil.DebugPrintAt(screen,
		fmt.Sprintf("%.0f/%.0f", s.Life, s.MaxLife), int(r+6)-14, int(y)-4)
}

// drawGlobe: пустой шар + заполнение снизу по доле frac.
func drawGlobe(screen *ebiten.Image, cx, cy, r float32, frac float64, empty, full color.Color) {
	vector.DrawFilledCircle(screen, cx, cy, r, empty, true)
	if frac > 0 {
		vector.DrawFilledCircle(screen, cx, cy, r*float32(frac), full, true)
	}
	vector.StrokeCircle(screen, cx, cy, r, 2, color.RGBA{0, 0, 0, 0xff}, true)
}
