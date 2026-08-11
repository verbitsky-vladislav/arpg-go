package world

import (
	"math/rand"

	"github.com/vladislav/game/internal/entity"
)

// randFloat — [0,1). Глобальный rand в Go ≥1.20 сам засеян случайно.
func randFloat() float64 { return rand.Float64() }

// randomKind выбирает тип моба по весам. Grunt'ов больше, brute реже.
func randomKind() entity.MobKind {
	switch n := rand.Intn(10); {
	case n < 6:
		return entity.Grunt
	case n < 8:
		return entity.Zapper
	default:
		return entity.Brute
	}
}
