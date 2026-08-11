package entity

import (
	"github.com/hajimehoshi/ebiten/v2"

	"github.com/vladislav/game/internal/art"
	"github.com/vladislav/game/internal/combat"
	"github.com/vladislav/game/internal/engine"
)

const projectileLife = 90 // тиков до самоуничтожения

type Projectile struct {
	Pos, Vel engine.Vec2
	Damage   combat.Damage
	Radius   float64
	life     int
	sprite   *ebiten.Image
}

func NewProjectile(pos, dir engine.Vec2, dmg combat.Damage, speed float64) *Projectile {
	return &Projectile{
		Pos:    pos,
		Vel:    dir.Scale(speed),
		Damage: dmg,
		Radius: 4,
		life:   projectileLife,
		sprite: art.Projectile(dmg.Type),
	}
}

func (p *Projectile) Update(w World) {
	p.Pos = p.Pos.Add(p.Vel)
	p.life--
}

func (p *Projectile) Alive() bool { return p.life > 0 }
func (p *Projectile) Kill()       { p.life = 0 }

func (p *Projectile) Draw(screen *ebiten.Image, cam *engine.Camera) {
	drawSprite(screen, p.sprite, cam, p.Pos, p.Vel.Angle())
}
