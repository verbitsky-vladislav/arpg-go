package combat_test

import (
	"math/rand/v2"
	"testing"

	"github.com/vladislav/game/internal/combat"
)

func rng() *rand.Rand { return rand.New(rand.NewPCG(1, 2)) }

// TestRollStaysInRange — бросок не выходит за границы и берёт обе.
func TestRollStaysInRange(t *testing.T) {
	r := combat.Roll{Min: 2, Max: 4}
	src := rng()
	seen := map[int]bool{}
	for range 200 {
		v := r.Value(src)
		if v < r.Min || v > r.Max {
			t.Fatalf("бросок %d вне %s", v, r)
		}
		seen[v] = true
	}
	for _, want := range []int{2, 3, 4} {
		if !seen[want] {
			t.Errorf("за 200 бросков ни разу не выпало %d: %s разыгрывается не целиком", want, r)
		}
	}
}

// TestRollDegenerate — вырожденный диапазон отдаёт одно и то же число, а
// отрицательный урон не наносится вовсе.
func TestRollDegenerate(t *testing.T) {
	if v := (combat.Roll{Min: 3, Max: 3}).Value(rng()); v != 3 {
		t.Errorf("3-3 дало %d", v)
	}
	if v := (combat.Roll{Min: 3, Max: 1}).Value(rng()); v != 3 {
		t.Errorf("перевёрнутый диапазон дал %d вместо нижней границы", v)
	}
	if v := (combat.Roll{Min: -5, Max: -1}).Value(rng()); v != 0 {
		t.Errorf("отрицательный диапазон дал %d вместо нуля", v)
	}
}

// TestAddSumsBounds — база и оружие складываются границами, а не бросками:
// «1-2» плюс «2-4» — это «3-6», один бросок на удар.
func TestAddSumsBounds(t *testing.T) {
	base := combat.Rolls{Physical: combat.Roll{Min: 1, Max: 2}}
	weapon := combat.Rolls{
		Physical: combat.Roll{Min: 2, Max: 4},
		Fire:     combat.Roll{Min: 5, Max: 5},
	}
	got := base.Add(weapon)
	if got.Physical != (combat.Roll{Min: 3, Max: 6}) {
		t.Errorf("физический урон %s, ожидался 3-6", got.Physical)
	}
	// Вид, которого нет у одного из слагаемых, приходит от второго целиком.
	if got.Fire != (combat.Roll{Min: 5, Max: 5}) {
		t.Errorf("огненный урон %s, ожидался 5", got.Fire)
	}
	if !got.Cold.Empty() {
		t.Errorf("холод взялся из ниоткуда: %s", got.Cold)
	}
	src := rng()
	for range 100 {
		if d := got.Value(src); d.Physical < 3 || d.Physical > 6 || d.Total() != d.Physical+5 {
			t.Fatalf("бросок суммы дал %+v", d)
		}
	}
}

// TestLandRollsEachEffect — состояния разыгрываются по отдельности: верное
// накладывается всегда, невозможное — никогда.
func TestLandRollsEachEffect(t *testing.T) {
	fx := []combat.Effect{
		{Kind: combat.Bleed, Chance: 1, Ticks: 60, Power: 1},
		{Kind: combat.Stun, Chance: 0, Ticks: 30},
	}
	src := rng()
	for range 50 {
		got := combat.Land(fx, src)
		if len(got) != 1 || got[0].Kind != combat.Bleed {
			t.Fatalf("наложилось %+v, ожидалось только кровотечение", got)
		}
	}
}

// TestWeaponProblems — таблица с бессмыслицей не должна молча становиться
// оружием: у площадного нет радиуса, у одиночного он лишний, скорость нулевая.
func TestWeaponProblems(t *testing.T) {
	ok := &combat.Weapon{
		Speed: 1, Shape: combat.ShapeSingle,
		Damage: combat.Rolls{Physical: combat.Roll{Min: 2, Max: 4}},
	}
	if probs := ok.Problems("палка"); len(probs) > 0 {
		t.Errorf("исправное оружие забраковано: %v", probs)
	}
	for name, w := range map[string]*combat.Weapon{
		"без скорости":        {Shape: combat.ShapeSingle},
		"без формы":           {Speed: 1},
		"область без радиуса": {Speed: 1, Shape: combat.ShapeArea},
		"радиус у одиночного": {Speed: 1, Shape: combat.ShapeSingle, Radius: 20},
		"перевёрнутый урон": {Speed: 1, Shape: combat.ShapeSingle,
			Damage: combat.Rolls{Physical: combat.Roll{Min: 5, Max: 2}}},
		"неизвестное состояние": {Speed: 1, Shape: combat.ShapeSingle,
			Effects: []combat.Effect{{Kind: "паника", Chance: 1, Ticks: 10}}},
	} {
		if probs := w.Problems("оружие"); len(probs) == 0 {
			t.Errorf("%s: проблем не найдено", name)
		}
	}
}

// TestNilWeaponIsHarmless — оружия может не быть вовсе, и спрашивать его о
// свойствах должно быть безопасно: голая рука — обычное состояние героя.
func TestNilWeaponIsHarmless(t *testing.T) {
	var w *combat.Weapon
	if w.Rate() != 1 {
		t.Errorf("скорость пустой руки %.2f", w.Rate())
	}
	if w.Area() || w.Note() != "" || len(w.Problems("рука")) != 0 {
		t.Error("пустая рука притворяется оружием")
	}
}
