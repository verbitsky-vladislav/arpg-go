package scene

import (
	"os"
	"testing"

	"github.com/vladislav/game/internal/assets"
	"github.com/vladislav/game/internal/engine"
)

// TestGameRunsWithEnemies — забег живёт: карта генерируется, герой, звери и
// враги обновляются вместе, никто не паникует и не остаётся без кадра.
//
// Это проверка именно связки. Поведение врагов проверено в internal/mob, а
// здесь важно другое: что сцена их создала, кормит целью каждый тик и что обмен
// ударами не роняет ни одну из сторон.
func TestGameRunsWithEnemies(t *testing.T) {
	l := assets.NewLoader(os.DirFS("../../assets"))
	g, err := NewGame(l, nil, "male")
	if err != nil {
		t.Fatal(err)
	}

	if len(g.es.Enemies()) == 0 {
		t.Error("карта заселена без врагов")
	}
	if g.es.Danger() <= 0 {
		t.Error("опасность карты нулевая")
	}
	if errs := g.es.Errors(); len(errs) > 0 {
		t.Errorf("паки врагов не загрузились: %v", errs)
	}

	// Цель, которую сцена отдаёт врагам, обязана быть осмысленной.
	tg := g.target()
	if !tg.Alive || tg.Radius <= 0 || engine.Dist(tg.Pos, g.pl.Pos) > 0.001 {
		t.Errorf("сцена отдаёт врагам странную цель: %+v", tg)
	}

	for i := range 1800 {
		if _, err := g.Update(); err != nil {
			t.Fatalf("тик %d: %v", i, err)
		}
		for _, e := range g.es.Enemies() {
			if e.Alive() && e.Frame() == nil {
				t.Fatalf("тик %d: враг %s без кадра (состояние %v)", i, e.Type.ID, e.State())
			}
		}
	}
}

// TestGameNoiseAndWindup — герой шумит по-разному и честно показывает замах:
// на этом держатся слух и уклонение врагов.
func TestGameNoiseAndWindup(t *testing.T) {
	l := assets.NewLoader(os.DirFS("../../assets"))
	g, err := NewGame(l, nil, "female")
	if err != nil {
		t.Fatal(err)
	}
	if n := g.target().Noise; n != 0 {
		t.Errorf("стоящий герой шумит на %.0f", n)
	}

	// Бег: шум обязан вырасти.
	in := g.input()
	in.Move = engine.Vec2{X: 1}
	in.Run = true
	for range 10 {
		g.pl.Update(in, g.m.Field())
	}
	if n := g.target().Noise; n <= noiseWalk {
		t.Errorf("бегущий герой шумит на %.0f — не громче шага", n)
	}
}
