// Package core — корень движка: тип Game реализует интерфейс ebiten.Game
// и делегирует всё активной сцене.
package core

import (
	"github.com/hajimehoshi/ebiten/v2"

	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/scene"
	"github.com/vladislav/game/internal/ui"
)

type Game struct {
	scene scene.Scene
}

// New создаёт игру, начиная со сцены start.
func New(start scene.Scene) *Game {
	return &Game{scene: start}
}

func (g *Game) Update() error {
	// Закрытие окна перехвачено (ebiten.SetWindowClosingHandled в main), чтобы
	// забег успел сохраниться: крестик в углу — такой же выход из игры, как
	// пункт меню, и терять на нём прогресс нельзя.
	if ebiten.IsWindowBeingClosed() {
		if c, ok := g.scene.(scene.Closer); ok {
			c.Closing()
		}
		return ebiten.Termination
	}
	next, err := g.scene.Update()
	if err != nil {
		return err
	}
	g.scene = next
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	ui.Cursor = ui.CursorArrow // сцена переопределит, если ей нужен прицел
	g.scene.Draw(screen)
	ui.DrawFPS(screen)    // счётчик кадров (если включён в настройках)
	ui.DrawCursor(screen) // игровой курсор-прицел (системный скрыт)
}

func (g *Game) Layout(outsideW, outsideH int) (int, int) {
	return config.ScreenW, config.ScreenH
}
