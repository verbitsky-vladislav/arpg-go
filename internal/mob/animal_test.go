package mob_test

import (
	"math/rand/v2"
	"testing"

	"github.com/vladislav/game/internal/engine"
	"github.com/vladislav/game/internal/mob"
	"github.com/vladislav/game/internal/physics"
	"github.com/vladislav/game/internal/sprite"
)

// field собирает поле 640×640 px из правила «что в этой точке».
func field(kind func(p engine.Vec2) physics.Cell) *physics.Field {
	const sub = 8.0
	const side = 80
	cells := make([]physics.Cell, side*side)
	for sy := range side {
		for sx := range side {
			cells[sy*side+sx] = kind(engine.Vec2{
				X: (float64(sx) + 0.5) * sub,
				Y: (float64(sy) + 0.5) * sub,
			})
		}
	}
	return physics.NewField(side, side, sub, cells)
}

// plainField — вся суша, воды нет.
func plainField() *physics.Field {
	return field(func(engine.Vec2) physics.Cell { return physics.Ground })
}

// pondField — правее x=200 вода (мель у берега, дальше глубина): проверка, что
// зверя не уносит туда, где ему нечем плыть.
func pondField() *physics.Field {
	return field(func(p engine.Vec2) physics.Cell {
		switch {
		case p.X > 240:
			return physics.Deep
		case p.X > 200:
			return physics.Shallow
		}
		return physics.Ground
	})
}

func newRNG() *rand.Rand { return rand.New(rand.NewPCG(7, 11)) }

// animals собирает по особи на каждый вид.
func animals(t *testing.T) []*mob.Animal {
	t.Helper()
	l, cat := catalog(t)
	rng := newRNG()
	out := make([]*mob.Animal, 0, len(cat.Species))
	for _, id := range cat.IDs() {
		sp := cat.Get(id)
		p, err := sprite.Load(l, animalsDir+"/"+sp.Art)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		out = append(out, mob.NewAnimal(sp, p, engine.Vec2{X: 100, Y: 100}, rng))
	}
	return out
}

// TestAnimalsLive — каждый вид проживает долгий отрезок с угрозой рядом, не
// паникуя и не оставаясь без кадра. Это проверка деградации: наборы анимаций
// рваные, но состояние обязано отрисовываться у всех.
func TestAnimalsLive(t *testing.T) {
	for _, a := range animals(t) {
		id := a.Species.ID
		start := a.Pos
		c := mob.Ctx{Field: plainField(), Threat: engine.Vec2{X: 140, Y: 100}, HasThreat: true}
		moved := 0.0
		// 1800 тиков — полминуты: за это время выпадает полтора десятка решений,
		// так что «ни разу не тронулся» уже не невезение, а поломка.
		for i := range 1800 {
			a.Update(c)
			if a.Alive() && a.Frame() == nil {
				t.Fatalf("%s: тик %d, состояние %v — нечего рисовать", id, i, a.State())
			}
			moved = max(moved, engine.Dist(a.Pos, start))
		}
		if moved == 0 {
			t.Errorf("%s: за 1800 тиков ни разу не сдвинулся", id)
		}
	}
}

// TestDeathEnds — смерть доигрывается и особь убирается из мира даже у видов
// без клипа death.
func TestDeathEnds(t *testing.T) {
	for _, a := range animals(t) {
		a.Hit(a.Species.Stats.HP + 100)
		if a.Alive() {
			t.Fatalf("%s: пережил урон больше своего hp", a.Species.ID)
		}
		c := mob.Ctx{Field: plainField()}
		for range 600 {
			a.Update(c)
			if a.Gone() {
				break
			}
		}
		if !a.Gone() {
			t.Errorf("%s: труп не убрался за 600 тиков", a.Species.ID)
		}
	}
}

// TestNoIdleNoStanding — вид без клипа idle не встаёт в стойку: подменять её
// нечем, поэтому он всегда в движении.
func TestNoIdleNoStanding(t *testing.T) {
	for _, a := range animals(t) {
		if a.Pack.Has("idle") {
			continue
		}
		c := mob.Ctx{Field: plainField()}
		for i := range 1200 {
			a.Update(c)
			if a.State() == mob.Idle {
				t.Fatalf("%s: встал в idle на тике %d, хотя клипа idle нет", a.Species.ID, i)
			}
		}
	}
}

// TestStaysOutOfWater — на глубину выходит только тот, кому есть чем плыть: вид
// водоплавающий И в паке есть клип swim. Сухопутный зверь не заходит даже на
// мель, водоплавающий без клипа бредёт по мели, но не глубже.
func TestStaysOutOfWater(t *testing.T) {
	w := pondField()
	for _, a := range animals(t) {
		sp := a.Species
		if sp.Locomotion.Water && a.Pack.Has("swim") {
			continue
		}
		c := mob.Ctx{Field: w, Threat: engine.Vec2{X: 0, Y: 100}, HasThreat: true} // гонит вправо, к воде
		for range 900 {
			a.Update(c)
			switch cell := w.CellAt(a.Pos); {
			case cell == physics.Deep:
				t.Fatalf("%s: вышел на глубину (%.0f,%.0f) без клипа swim",
					sp.ID, a.Pos.X, a.Pos.Y)
			case cell == physics.Shallow && !sp.Locomotion.Water:
				t.Fatalf("%s: сухопутный зверь забрёл на мель (%.0f,%.0f)",
					sp.ID, a.Pos.X, a.Pos.Y)
			}
		}
	}
}

// TestSpawnTables — по каждому упомянутому в данных биому есть кого спавнить,
// детёныши сами по себе не выпадают, размеры групп из данных.
func TestSpawnTables(t *testing.T) {
	_, cat := catalog(t)
	biomes := map[string]bool{}
	for _, id := range cat.IDs() {
		for b := range cat.Get(id).Habitat.Biomes {
			biomes[b] = true
		}
	}
	rng := newRNG()
	for b := range biomes {
		tb := mob.BuildTable(cat, b)
		if tb.Empty() {
			t.Errorf("биом %s: таблица спавна пуста", b)
			continue
		}
		for _, sp := range tb.Species() {
			if sp.Spawn.With != "" {
				t.Errorf("биом %s: %s спавнится самостоятельно, хотя должен идти при %s", b, sp.ID, sp.Spawn.With)
			}
		}
		for range 200 {
			g := tb.RollGroup(cat, rng)
			if g == nil {
				t.Fatalf("биом %s: пустой бросок при непустой таблице", b)
			}
			if g.Count < g.Species.Spawn.Group.Min || g.Count > g.Species.Spawn.Group.Max {
				t.Errorf("%s: группа %d вне диапазона %d..%d", g.Species.ID, g.Count,
					g.Species.Spawn.Group.Min, g.Species.Spawn.Group.Max)
			}
			if g.Young != nil && g.Young.Spawn.With != g.Species.ID {
				t.Errorf("%s: выводок %s не привязан к нему", g.Species.ID, g.Young.ID)
			}
		}
	}
}

// TestAnchors — точка опоры посчитана и лежит внутри кадра. Без неё зверя не
// поставить на тайл: он будет висеть или тонуть.
func TestAnchors(t *testing.T) {
	l, cat := catalog(t)
	for _, id := range cat.IDs() {
		p, err := sprite.Load(l, animalsDir+"/"+cat.Get(id).Art)
		if err != nil {
			t.Fatal(err)
		}
		if p.Anchor == nil {
			t.Errorf("%s: нет anchor в манифесте (прогнать tools/spriteanchor)", id)
			continue
		}
		f := p.Foot()
		if f.X < 0 || f.X > p.Frame.W || f.Y < 0 || f.Y > p.Frame.H {
			t.Errorf("%s: anchor (%d,%d) вне кадра %dx%d", id, f.X, f.Y, p.Frame.W, p.Frame.H)
		}
		b := p.Bounds()
		if b.W <= 0 || b.H <= 0 || b.X+b.W > p.Frame.W || b.Y+b.H > p.Frame.H {
			t.Errorf("%s: bbox %+v не помещается в кадр %dx%d", id, b, p.Frame.W, p.Frame.H)
		}
	}
}

// plateauField — низ до y=400 и макушка возвышенности за ним, БЕЗ лестницы.
func plateauField() *physics.Field {
	return field(func(p engine.Vec2) physics.Cell {
		if p.Y >= 400 {
			return physics.Plateau
		}
		return physics.Ground
	})
}

// TestNoClimbWithoutStairs — на возвышенность нельзя забраться, минуя лестницу.
// Зверя гонят прямо на неё: он обязан упереться в границу этажей, даже там, где
// обрыва не нарисовано.
func TestNoClimbWithoutStairs(t *testing.T) {
	w := plateauField()
	for _, a := range animals(t) {
		c := mob.Ctx{Field: w, Threat: engine.Vec2{X: 100, Y: 40}, HasThreat: true} // гонит вниз, к плато
		for range 900 {
			a.Update(c)
			if w.CellAt(a.Pos) == physics.Plateau {
				t.Fatalf("%s: залез на плато без лестницы (%.0f,%.0f)",
					a.Species.ID, a.Pos.X, a.Pos.Y)
			}
			if a.Floor() != physics.FloorLow {
				t.Fatalf("%s: сменил этаж без лестницы", a.Species.ID)
			}
		}
	}
}

// TestCrowdSeparates — толпа, слипшаяся в одну точку, расходится и больше не
// налезает. Это проверка сходимости попарного расталкивания: тем же способом
// разводит тела игровой слой (scene.Game.separate), и если бы оно дрожало —
// звери бы вечно дёргались друг об друга.
func TestCrowdSeparates(t *testing.T) {
	l, cat := catalog(t)
	sp := cat.Get(cat.IDs()[0])
	pack, err := sprite.Load(l, animalsDir+"/"+sp.Art)
	if err != nil {
		t.Fatal(err)
	}
	f := plainField()
	rng := newRNG()
	herd := make([]*mob.Animal, 12)
	for i := range herd {
		herd[i] = mob.NewAnimal(sp, pack, engine.Vec2{X: 300, Y: 300}, rng)
	}

	for range 120 {
		for i, a := range herd {
			for _, b := range herd[i+1:] {
				if d, ok := physics.Separate(a.Pos, a.Radius(), b.Pos, b.Radius()); ok {
					a.Push(f, d)
					b.Push(f, d.Scale(-1))
				}
			}
		}
	}

	for i, a := range herd {
		for _, b := range herd[i+1:] {
			gap := a.Radius() + b.Radius()
			if d := engine.Dist(a.Pos, b.Pos); d < gap-0.5 {
				t.Fatalf("тела остались внахлёст: %.2f при зазоре %.2f", d, gap)
			}
		}
		if !f.Fits(a.Pos, a.Body()) {
			t.Fatalf("особь %d вытолкнули в непроходимое: %v", i, a.Pos)
		}
	}
}
