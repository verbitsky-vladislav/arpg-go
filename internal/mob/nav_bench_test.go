package mob_test

// Стоимость навигации. Ею выбран шаг сетки: 32 px дёшево, но грубо (узкие
// проходы «закрываются»), 16 px втрое дороже и проходимо честно. Замеры на
// карте 2048×2048 с тремя полосами.

import (
	"testing"

	"github.com/vladislav/game/internal/engine"
	"github.com/vladislav/game/internal/mob"
)

func BenchmarkNavRebuild(b *testing.B) {
	f := mazeField(2048, 1024, 900)
	for _, step := range []float64{32, 24, 16} {
		b.Run(map[float64]string{32: "step32", 24: "step24", 16: "step16"}[step], func(b *testing.B) {
			n := mob.NewNav(f, step)
			b.ResetTimer()
			for i := 0; b.Loop(); i++ {
				n.Rebuild(engine.Vec2{X: 300 + float64(i%50), Y: 300}, 0)
			}
		})
	}
}

func BenchmarkNavPath(b *testing.B) {
	f := mazeField(2048, 1024, 900)
	n := mob.NewNav(f, 16)
	for b.Loop() {
		n.Path(engine.Vec2{X: 200, Y: 200}, engine.Vec2{X: 1500, Y: 1500}, 0, 16)
	}
}
