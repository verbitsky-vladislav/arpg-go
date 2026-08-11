package scene

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/ui"
	"github.com/vladislav/game/internal/world"
)

// Play — основной игровой экран. Владеет миром и рисует HUD.
type Play struct {
	world *world.World
	over  bool
}

func NewPlay() *Play {
	return &Play{world: world.New()}
}

func (p *Play) Update() (Scene, error) {
	if p.over {
		if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
			p.world.Reset()
			p.over = false
		}
		return p, nil
	}

	p.world.Update()
	if p.world.PlayerDead() {
		p.over = true
	}
	return p, nil
}

func (p *Play) Draw(screen *ebiten.Image) {
	p.world.Draw(screen)
	ui.DrawHUD(screen, p.world.PlayerStats(), p.world.Score())

	if p.over {
		ebitenutil.DebugPrintAt(screen, "YOU DIED",
			config.ScreenW/2-24, config.ScreenH/2-16)
		ebitenutil.DebugPrintAt(screen, "Press ENTER to respawn",
			config.ScreenW/2-64, config.ScreenH/2)
	}
}
