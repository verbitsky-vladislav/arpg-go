// Package combat — боевые правила: типы урона, характеристики, снижение урона.
// Чистая логика без графики и ebiten, чтобы её можно было тестировать отдельно.
package combat

// DamageType — стихия/вид урона (как в Path of Exile).
type DamageType int

const (
	Physical DamageType = iota
	Fire
	Cold
	Lightning
	Chaos

	NumDamageTypes = 5
)

func (t DamageType) String() string {
	switch t {
	case Physical:
		return "Physical"
	case Fire:
		return "Fire"
	case Cold:
		return "Cold"
	case Lightning:
		return "Lightning"
	case Chaos:
		return "Chaos"
	default:
		return "?"
	}
}

// Damage — порция урона: сколько и какого типа.
type Damage struct {
	Type   DamageType
	Amount float64
}
