package scene

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/ui"
)

// Menu — стартовый экран-заглушка. Контент (герои, мир, забег) вырезан под
// переделку; сцена лишь показывает, что движок жив и рисует.
type Menu struct{}

// NewMenu создаёт стартовое меню.
func NewMenu() *Menu { return &Menu{} }

func (m *Menu) Update() (Scene, error) {
	return m, nil
}

func (m *Menu) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{0x10, 0x12, 0x1a, 0xff})
	ui.TextCentered(screen, "P I X E L   A R P G", config.ScreenW/2, config.ScreenH/2-16)
	ui.TextCentered(screen, "clean slate - content pending", config.ScreenW/2, config.ScreenH/2+8)
}
