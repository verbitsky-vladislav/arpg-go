package scene

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/hero"
	"github.com/vladislav/game/internal/ui"
)

// Menu — стартовый экран: играть или открыть галерею героев.
type Menu struct {
	reg  *hero.Registry
	play ui.Button
	pick ui.Button
}

// NewMenu создаёт меню поверх реестра героев (нужен экрану выбора).
func NewMenu(reg *hero.Registry) *Menu {
	cx := float32(config.ScreenW / 2)
	return &Menu{
		reg:  reg,
		play: ui.Button{X: cx - 90, Y: config.ScreenH/2 + 8, W: 180, H: 28, Label: "PLAY  (Enter)"},
		pick: ui.Button{X: cx - 90, Y: config.ScreenH/2 + 44, W: 180, H: 28, Label: "CHOOSE HEROES"},
	}
}

func (m *Menu) Update() (Scene, error) {
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		return NewPlay(), nil
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyH) {
		return NewHeroSelect(m.reg, m), nil
	}
	mx, my := ebiten.CursorPosition()
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		switch {
		case m.play.Contains(mx, my):
			return NewPlay(), nil
		case m.pick.Contains(mx, my):
			return NewHeroSelect(m.reg, m), nil
		}
	}
	return m, nil
}

func (m *Menu) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{0x10, 0x12, 0x1a, 0xff})
	ui.TextCentered(screen, "P I X E L   A R P G", config.ScreenW/2, config.ScreenH/2-56)
	ui.TextCentered(screen, "WASD - move | Mouse - aim | LMB/Space - cast",
		config.ScreenW/2, config.ScreenH/2-32)
	ui.TextCentered(screen, "(H - hero gallery)", config.ScreenW/2, config.ScreenH/2+84)

	mx, my := ebiten.CursorPosition()
	m.play.Draw(screen, m.play.Contains(mx, my))
	m.pick.Draw(screen, m.pick.Contains(mx, my))
}
