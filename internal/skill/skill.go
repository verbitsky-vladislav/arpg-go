// Package skill — описания умений (снаряды, заклинания). Здесь только ДАННЫЕ:
// сколько маны, кулдаун, урон, скорость снаряда. Как из этого рождается
// снаряд в мире — решает пакет entity. Так skill не зависит от entity
// (и не возникает цикл импортов).
package skill

import "github.com/vladislav/game/internal/combat"

// Skill — умение, выпускающее снаряд в сторону прицела.
type Skill struct {
	Name            string
	Mana            float64
	Cooldown        int // тиков между применениями
	Damage          combat.Damage
	ProjectileSpeed float64
}

// Fireball — стартовое умение мага: огненный снаряд.
func Fireball() Skill {
	return Skill{
		Name:            "Fireball",
		Mana:            6,
		Cooldown:        18,
		Damage:          combat.Damage{Type: combat.Fire, Amount: 12},
		ProjectileSpeed: 5,
	}
}

// Spark — быстрые слабые молнии (пример второго умения).
func Spark() Skill {
	return Skill{
		Name:            "Spark",
		Mana:            3,
		Cooldown:        9,
		Damage:          combat.Damage{Type: combat.Lightning, Amount: 6},
		ProjectileSpeed: 7,
	}
}
