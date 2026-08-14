package physics

import (
	"math"

	"github.com/vladislav/game/internal/engine"
)

// escapeStep — на сколько пикселей за тик тело выдавливается из стены, если
// всё-таки в ней оказалось (сменился этаж, поставили криво, отбросило ударом).
const escapeStep = 1.0

// Move двигает тело из pos на delta и возвращает новую позицию и этаж.
//
// Упёршись, тело скользит вдоль препятствия: сначала пробуется полный шаг,
// потом каждая ось по отдельности. Без этого герой намертво встаёт в углу и не
// может пройти вдоль стены по диагонали.
//
// Этаж меняется только на лестнице: сойдя с неё, тело получает этаж клетки, на
// которую сошло. Пока центр тела на лестнице, ему проходимы оба этажа — иначе с
// неё некуда шагнуть, круг всегда задевает и низ, и верх.
func (f *Field) Move(pos, delta engine.Vec2, b Body) (engine.Vec2, uint8) {
	if f == nil {
		return pos.Add(delta), b.Floor
	}
	eff := b
	if f.CellAt(pos) == Ramp {
		eff.Floor = FloorAny
	}
	if !f.Fits(pos, eff) {
		return f.escape(pos, eff), b.Floor // сначала выбраться, потом ходить
	}

	next := pos.Add(delta)
	switch {
	case f.Fits(next, eff):
	case f.Fits(engine.Vec2{X: next.X, Y: pos.Y}, eff):
		next = engine.Vec2{X: next.X, Y: pos.Y}
	case f.Fits(engine.Vec2{X: pos.X, Y: next.Y}, eff):
		next = engine.Vec2{X: pos.X, Y: next.Y}
	default:
		next = pos
	}
	return next, f.floorAt(next, b.Floor)
}

// floorAt — этаж тела в точке p, если раньше оно было на этаже cur. На связке
// этаж не меняется: он определится там, где тело с неё сойдёт.
func (f *Field) floorAt(p engine.Vec2, cur uint8) uint8 {
	c := f.CellAt(p)
	if c == Ramp {
		return cur
	}
	return c.Floor()
}

// escapeRings — как далеко искать выход из стены, в под-клетках. Дальше искать
// незачем: тело, оказавшееся в середине скалы, туда не дошло, а было поставлено
// — это чинит Place на появлении, а не шаг за шагом.
const escapeRings = 4

// escape выдавливает тело из непроходимой клетки: шаг escapeStep в сторону
// ближайшей проходимой под-клетки. Возвращает исходную точку, если выхода рядом
// нет. Толкать «от центров стен» нельзя: тело ровно в центре одиночной стены
// получило бы нулевую сумму и осталось бы там навсегда.
func (f *Field) escape(pos engine.Vec2, b Body) engine.Vec2 {
	sx0, sy0 := f.sx(pos.X), f.sx(pos.Y)
	for ring := 1; ring <= escapeRings; ring++ {
		best, found := engine.Vec2{}, false
		bestDist := math.Inf(1)
		for dy := -ring; dy <= ring; dy++ {
			for dx := -ring; dx <= ring; dx++ {
				if dx != -ring && dx != ring && dy != -ring && dy != ring {
					continue // только кромка кольца
				}
				if !f.Passable(f.At(sx0+dx, sy0+dy), b) {
					continue
				}
				q := f.center(sx0+dx, sy0+dy)
				if d := engine.Dist(q, pos); d < bestDist {
					best, bestDist, found = q, d, true
				}
			}
		}
		if found {
			return pos.Add(best.Sub(pos).Normalized().Scale(escapeStep))
		}
	}
	return pos
}

// center — мировая точка в середине под-клетки.
func (f *Field) center(sx, sy int) engine.Vec2 {
	return engine.Vec2{X: (float64(sx) + 0.5) * f.sub, Y: (float64(sy) + 0.5) * f.sub}
}

// Place — ближайшая к p точка, где тело помещается целиком (появление,
// воскрешение, подсев зверей). Ищет по кольцам под-клеток в радиусе maxR
// пикселей; ok=false — места нет.
func (f *Field) Place(p engine.Vec2, b Body, maxR float64) (engine.Vec2, bool) {
	if f == nil {
		return p, true
	}
	if f.Fits(p, b) {
		return p, true
	}
	cx, cy := f.sx(p.X), f.sx(p.Y)
	rings := int(maxR/f.sub) + 1
	for ring := 1; ring <= rings; ring++ {
		for dy := -ring; dy <= ring; dy++ {
			for dx := -ring; dx <= ring; dx++ {
				// только кромка кольца: внутренность уже осмотрена
				if dx != -ring && dx != ring && dy != -ring && dy != ring {
					continue
				}
				sx, sy := cx+dx, cy+dy
				q := f.center(sx, sy)
				if engine.Dist(q, p) > maxR {
					continue // кольцо считается по клеткам, а предел задан в пикселях
				}
				nb := b
				nb.Floor = f.At(sx, sy).Floor()
				if f.Fits(q, nb) {
					return q, true
				}
			}
		}
	}
	return p, false
}

// Separate — на сколько разойтись двум пересёкшимся телам. Возвращает смещение
// для первого тела (второму — противоположное) и false, если они не пересеклись.
//
// Каждое отходит на половину перекрытия: так толпа расталкивается сама, без
// масс и импульсов. Совпавшие в точку тела разводит по фиксированному
// направлению — иначе делить на ноль, а слипшуюся пару ничего не расцепит.
func Separate(a engine.Vec2, ar float64, b engine.Vec2, br float64) (engine.Vec2, bool) {
	d := a.Sub(b)
	dist := d.Len()
	gap := ar + br
	if dist >= gap {
		return engine.Vec2{}, false
	}
	if dist == 0 {
		return engine.Vec2{X: gap / 2}, true
	}
	return d.Scale((gap - dist) / 2 / dist), true
}
