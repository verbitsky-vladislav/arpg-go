package sprite

import (
	"math"
	"testing"

	"github.com/vladislav/game/internal/engine"
)

// TestDirFrom8 — вектор движения ложится в свой октант. Границы стоят
// посередине между направлениями, поэтому чистое «вниз» и чистая
// диагональ не дрожат на стыке.
func TestDirFrom8(t *testing.T) {
	cases := []struct {
		v    engine.Vec2
		want Dir
	}{
		{engine.Vec2{}, Down},
		{engine.Vec2{X: 0, Y: 1}, Down},
		{engine.Vec2{X: 0, Y: -1}, Up},
		{engine.Vec2{X: -1, Y: 0}, Left},
		{engine.Vec2{X: 1, Y: 0}, Right},
		{engine.Vec2{X: 1, Y: 1}, DownRight},
		{engine.Vec2{X: -1, Y: 1}, DownLeft},
		{engine.Vec2{X: 1, Y: -1}, UpRight},
		{engine.Vec2{X: -1, Y: -1}, UpLeft},
		// Чуть в стороне от оси — всё ещё сторона света, не диагональ.
		{engine.Vec2{X: 0.2, Y: 1}, Down},
		{engine.Vec2{X: 1, Y: 0.2}, Right},
		// Чуть в стороне от диагонали — всё ещё диагональ.
		{engine.Vec2{X: 1, Y: 0.7}, DownRight},
		{engine.Vec2{X: 0.7, Y: 1}, DownRight},
	}
	for _, c := range cases {
		if got := DirFrom8(c.v); got != c.want {
			t.Errorf("DirFrom8(%v) = %v, жду %v", c.v, got, c.want)
		}
	}
}

// TestDirFrom8CoversCircle — обойдя круг, побывать в каждом из восьми
// направлений, и ровно по одной непрерывной дуге на каждое.
func TestDirFrom8CoversCircle(t *testing.T) {
	seen := map[Dir]int{}
	prev := Dir(255)
	const steps = 720
	for i := range steps {
		a := 2 * math.Pi * float64(i) / steps
		d := DirFrom8(engine.Vec2{X: math.Cos(a), Y: math.Sin(a)})
		if d != prev {
			seen[d]++
			prev = d
		}
	}
	// Начало обхода приходится на середину дуги Right, и она же его
	// завершает, поэтому у неё две засечки вместо одной.
	for d, n := range seen {
		if want := 1; n != want && d != Right {
			t.Errorf("%v встретилось %d раз, жду %d непрерывную дугу", d, n, want)
		}
	}
	if len(seen) != DirCount {
		t.Errorf("за круг встретилось %d направлений из %d", len(seen), DirCount)
	}
}

// TestDirVecMatchesDirFrom8 — вектор направления читается обратно в то же
// направление. На этом держится сектор удара: он строится по Vec.
func TestDirVecMatchesDirFrom8(t *testing.T) {
	for d := range Dir(DirCount) {
		v := d.Vec()
		if l := v.Len(); math.Abs(l-1) > 1e-9 {
			t.Errorf("%v: длина вектора %v, жду 1", d, l)
		}
		if got := DirFrom8(v); got != d {
			t.Errorf("DirFrom8(%v.Vec()) = %v", d, got)
		}
	}
}

// TestFallbackCoversDiagonals — у каждой диагонали есть на что опереться,
// и цепочка ведёт к стороне света, а не по кругу.
func TestFallbackCoversDiagonals(t *testing.T) {
	for _, d := range []Dir{DownRight, DownLeft, UpRight, UpLeft} {
		chain := fallback[d]
		if len(chain) == 0 {
			t.Errorf("%v: некуда откатиться", d)
			continue
		}
		for _, alt := range chain {
			if alt >= DownRight {
				t.Errorf("%v: откат на диагональ %v, а нужен на сторону света", d, alt)
			}
		}
	}
}
