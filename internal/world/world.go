// Package world собирает сущности вместе: обновляет их, спавнит мобов,
// разрешает столкновения (снаряд↔моб, моб↔игрок) и рисует сцену с камерой.
// Реализует интерфейс entity.World.
package world

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/vladislav/game/internal/combat"
	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/engine"
	"github.com/vladislav/game/internal/entity"
)

const (
	baseSpawnRate = 70 // тиков между спавнами вначале
	minSpawnRate  = 18
	gridStep      = 100 // шаг фоновой сетки, мировые единицы
)

type World struct {
	player      *entity.Player
	mobs        []*entity.Mob
	projectiles []*entity.Projectile
	cam         *engine.Camera

	spawnTick, spawnRate int
	score                int
}

func New() *World {
	w := &World{cam: engine.NewCamera(config.ScreenW, config.ScreenH)}
	w.Reset()
	return w
}

func (w *World) Reset() {
	w.player = entity.NewPlayer(engine.Vec2{X: config.WorldW / 2, Y: config.WorldH / 2})
	w.mobs = w.mobs[:0]
	w.projectiles = w.projectiles[:0]
	w.spawnTick = 0
	w.spawnRate = baseSpawnRate
	w.score = 0
	w.cam.Follow(w.player.Pos, config.WorldW, config.WorldH)
}

// --- реализация entity.World ---

func (w *World) AddProjectile(p *entity.Projectile) { w.projectiles = append(w.projectiles, p) }
func (w *World) Player() *entity.Player             { return w.player }
func (w *World) Mobs() []*entity.Mob                { return w.mobs }

func (w *World) CursorWorld() engine.Vec2 {
	mx, my := ebiten.CursorPosition()
	return w.cam.ScreenToWorld(engine.Vec2{X: float64(mx), Y: float64(my)})
}

// --- геттеры для сцены/UI ---

func (w *World) Score() int                 { return w.score }
func (w *World) PlayerStats() *combat.Stats { return &w.player.Stats }
func (w *World) PlayerDead() bool           { return !w.player.Alive() }

func (w *World) Update() {
	w.player.Update(w)
	// Игрок не выходит за границы мира.
	w.player.Pos.X = engine.Clamp(w.player.Pos.X, 0, config.WorldW)
	w.player.Pos.Y = engine.Clamp(w.player.Pos.Y, 0, config.WorldH)
	w.cam.Follow(w.player.Pos, config.WorldW, config.WorldH)

	for _, m := range w.mobs {
		m.Update(w)
	}
	for _, p := range w.projectiles {
		p.Update(w)
	}

	w.collisions()
	w.cleanup()
	w.spawn()
}

func (w *World) collisions() {
	// Снаряд ↔ моб.
	for _, p := range w.projectiles {
		if !p.Alive() {
			continue
		}
		for _, m := range w.mobs {
			if !m.Alive() {
				continue
			}
			if engine.Dist(p.Pos, m.Pos) < p.Radius+m.Radius {
				m.Stats.TakeDamage(p.Damage)
				p.Kill()
				if !m.Alive() {
					w.score++
				}
				break
			}
		}
	}
	// Моб ↔ игрок (контактная атака по откату).
	for _, m := range w.mobs {
		if !m.Alive() {
			continue
		}
		if engine.Dist(m.Pos, w.player.Pos) < m.Radius+entity.PlayerRadius {
			m.TryAttack(&w.player.Stats)
		}
	}
}

func (w *World) cleanup() {
	mobs := w.mobs[:0]
	for _, m := range w.mobs {
		if m.Alive() {
			mobs = append(mobs, m)
		}
	}
	w.mobs = mobs

	projs := w.projectiles[:0]
	for _, p := range w.projectiles {
		if p.Alive() {
			projs = append(projs, p)
		}
	}
	w.projectiles = projs
}

func (w *World) spawn() {
	w.spawnTick++
	if w.spawnTick < w.spawnRate {
		return
	}
	w.spawnTick = 0
	if w.spawnRate > minSpawnRate {
		w.spawnRate--
	}

	// Спавним за краем видимой области, вокруг игрока.
	angle := randFloat() * 2 * math.Pi
	dist := float64(config.ScreenW)/2 + 60 + randFloat()*80
	pos := engine.Vec2{
		X: engine.Clamp(w.player.Pos.X+dist*math.Cos(angle), 0, config.WorldW),
		Y: engine.Clamp(w.player.Pos.Y+dist*math.Sin(angle), 0, config.WorldH),
	}
	w.mobs = append(w.mobs, entity.NewMob(randomKind(), pos))
}

func (w *World) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{0x14, 0x14, 0x1c, 0xff})
	w.drawGrid(screen)

	for _, m := range w.mobs {
		m.Draw(screen, w.cam)
	}
	for _, p := range w.projectiles {
		p.Draw(screen, w.cam)
	}
	w.player.Draw(screen, w.cam)
}

// drawGrid рисует фоновую сетку — чтобы движение по большому миру было видно.
func (w *World) drawGrid(screen *ebiten.Image) {
	c := color.RGBA{0x22, 0x22, 0x2e, 0xff}
	startX := int(w.cam.Pos.X-config.ScreenW/2) / gridStep * gridStep
	for x := startX; float64(x) < w.cam.Pos.X+config.ScreenW/2; x += gridStep {
		sx := w.cam.WorldToScreen(engine.Vec2{X: float64(x)}).X
		vector.StrokeLine(screen, float32(sx), 0, float32(sx), config.ScreenH, 1, c, false)
	}
	startY := int(w.cam.Pos.Y-config.ScreenH/2) / gridStep * gridStep
	for y := startY; float64(y) < w.cam.Pos.Y+config.ScreenH/2; y += gridStep {
		sy := w.cam.WorldToScreen(engine.Vec2{Y: float64(y)}).Y
		vector.StrokeLine(screen, 0, float32(sy), config.ScreenW, float32(sy), 1, c, false)
	}
}
