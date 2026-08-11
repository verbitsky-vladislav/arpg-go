// Package core — корень движка: тип Game реализует интерфейс ebiten.Game
// и делегирует всё активной сцене.
package core

import (
	"github.com/hajimehoshi/ebiten/v2"

	"github.com/vladislav/game/internal/assets"
	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/hero"
	"github.com/vladislav/game/internal/scene"
)

type Game struct {
	scene scene.Scene
}

// New создаёт игру, начиная с меню. Загрузчик ассетов инъектируется снаружи
// (в проде — вшитая embed.FS из main), чтобы ядро не зависело от диска.
func New(l *assets.Loader) *Game {
	return &Game{scene: scene.NewMenu(hero.NewRegistry(l))}
}

func (g *Game) Update() error {
	next, err := g.scene.Update()
	if err != nil {
		return err
	}
	g.scene = next
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	g.scene.Draw(screen)
}

func (g *Game) Layout(outsideW, outsideH int) (int, int) {
	return config.ScreenW, config.ScreenH
}
