// Package item — предметы: оружие, броня, лут с редкостью и модификаторами.
// Задел под гринд в духе PoE: базовый предмет + аффиксы дают итоговые статы.
package item

import "github.com/vladislav/game/internal/combat"

// Rarity — редкость предмета (влияет на количество аффиксов и цвет).
type Rarity int

const (
	Normal Rarity = iota
	Magic
	Rare
	Unique
)

func (r Rarity) String() string {
	switch r {
	case Magic:
		return "Magic"
	case Rare:
		return "Rare"
	case Unique:
		return "Unique"
	default:
		return "Normal"
	}
}

// Weapon — оружие. Даёт базовый урон и скорость атаки; умения масштабируются
// от этого урона.
type Weapon struct {
	Name        string
	Damage      combat.Damage
	AttackSpeed float64 // атак в секунду
	Rarity      Rarity
}

// StarterWeapon — стартовое оружие нового персонажа.
func StarterWeapon() Weapon {
	return Weapon{
		Name:        "Rusted Wand",
		Damage:      combat.Damage{Type: combat.Physical, Amount: 4},
		AttackSpeed: 1.4,
		Rarity:      Normal,
	}
}
