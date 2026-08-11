// Package entity — игровые сущности: игрок, мобы, снаряды. Их поведение
// (движение, ИИ, применение умений). Разрешение столкновений и спавн живут
// в пакете world, который сущности видят через узкий интерфейс World —
// поэтому entity не импортирует world, и цикла импортов не возникает.
package entity

import (
	"github.com/hajimehoshi/ebiten/v2"

	"github.com/vladislav/game/internal/engine"
)

// World — то, что сущность может делать с миром во время своего Update.
// Реальную реализацию даёт пакет world.
type World interface {
	AddProjectile(*Projectile)
	Player() *Player
	Mobs() []*Mob
	CursorWorld() engine.Vec2 // позиция курсора в мировых координатах
}

// drawSprite рисует спрайт по центру мировой позиции с поворотом rot (радианы).
func drawSprite(screen *ebiten.Image, img *ebiten.Image, cam *engine.Camera, pos engine.Vec2, rot float64) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(-float64(w)/2, -float64(h)/2) // центр спрайта в (0,0)
	if rot != 0 {
		op.GeoM.Rotate(rot)
	}
	s := cam.WorldToScreen(pos)
	op.GeoM.Translate(s.X, s.Y)
	screen.DrawImage(img, op)
}
