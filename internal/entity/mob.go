package entity

import (
	"github.com/hajimehoshi/ebiten/v2"

	"github.com/vladislav/game/internal/art"
	"github.com/vladislav/game/internal/combat"
	"github.com/vladislav/game/internal/engine"
)

// MobKind — тип моба. Разные статы, урон и спрайт.
type MobKind int

const (
	Grunt  MobKind = iota // базовый
	Brute                 // медленный танк, бьёт больно
	Zapper                // быстрый, слабый
)

type mobDef struct {
	life    float64
	speed   float64
	damage  combat.Damage
	attackI int // интервал атаки в тиках
	radius  float64
	variant int // какой спрайт из art.Mob
}

var mobDefs = map[MobKind]mobDef{
	Grunt:  {life: 20, speed: 1.2, damage: combat.Damage{Type: combat.Physical, Amount: 6}, attackI: 40, radius: 8, variant: 0},
	Brute:  {life: 60, speed: 0.8, damage: combat.Damage{Type: combat.Physical, Amount: 14}, attackI: 55, radius: 12, variant: 1},
	Zapper: {life: 12, speed: 2.0, damage: combat.Damage{Type: combat.Lightning, Amount: 5}, attackI: 30, radius: 7, variant: 2},
}

type Mob struct {
	Pos    engine.Vec2
	Stats  combat.Stats
	Kind   MobKind
	Radius float64
	Damage combat.Damage

	attackI int // базовый интервал
	atkCD   int // текущий откат атаки
	sprite  *ebiten.Image
}

func NewMob(kind MobKind, pos engine.Vec2) *Mob {
	d := mobDefs[kind]
	return &Mob{
		Pos:     pos,
		Stats:   combat.Stats{MaxLife: d.life, Life: d.life, MoveSpeed: d.speed},
		Kind:    kind,
		Radius:  d.radius,
		Damage:  d.damage,
		attackI: d.attackI,
		sprite:  art.Mob(d.variant),
	}
}

// Update: моб ползёт к игроку. Сам удар наносится в world при столкновении.
func (m *Mob) Update(w World) {
	if m.atkCD > 0 {
		m.atkCD--
	}
	dir := w.Player().Pos.Sub(m.Pos).Normalized()
	m.Pos = m.Pos.Add(dir.Scale(m.Stats.MoveSpeed))
}

// TryAttack наносит урон, если откат прошёл. Возвращает true при ударе.
func (m *Mob) TryAttack(target *combat.Stats) bool {
	if m.atkCD > 0 {
		return false
	}
	target.TakeDamage(m.Damage)
	m.atkCD = m.attackI
	return true
}

func (m *Mob) Alive() bool { return m.Stats.Alive() }

func (m *Mob) Draw(screen *ebiten.Image, cam *engine.Camera) {
	drawSprite(screen, m.sprite, cam, m.Pos, 0)
}
