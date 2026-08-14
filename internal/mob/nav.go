package mob

import (
	"math"

	"github.com/vladislav/game/internal/engine"
	"github.com/vladislav/game/internal/physics"
)

// Навигация врагов: одна карта расстояний до цели на всех.
//
// Считать путь каждому отдельно незачем — цель у всех одна. Поэтому от цели
// один раз разливается волна по грубой сетке (Дейкстра с восемью соседями), а
// враги просто читают из своей клетки, куда шагнуть и как далеко до цели.
// Перестройка стоит один обход сетки на всю толпу, а не поиск на каждого.
//
// Та же карта отвечает и на второй вопрос — насколько далеко слышно. Звук идёт
// по коридорам, а не сквозь скалу, и «расстояние по проходимым клеткам» — это
// ровно то, что нужно: за стеной шум глохнет, даже если по прямой рукой подать.
//
// Сетка грубее физической: шаг навигации кратен под-клетке поля. Точность здесь
// не нужна — вдоль стен ведёт физика, а навигация отвечает только на вопрос
// «в какую сторону вообще».

// navUnreached — клетка, до которой волна не дошла.
const navUnreached int32 = -1

// Стоимость шага: прямой и диагональный. Целые, чтобы не считать в float.
const (
	navStraight int32 = 10
	navDiag     int32 = 14
)

// NavField — карта расстояний до цели по проходимым клеткам, отдельная на
// каждый этаж (низ и макушка плато связаны лестницами).
type NavField struct {
	f     *physics.Field
	step  float64 // сторона клетки навигации в пикселях
	w, h  int
	body  physics.Body // тело, которым меряется проходимость сетки
	pass  [2][]bool    // проходимость по этажам
	link  [2][]uint8   // маска соседей, к которым есть проход (по биту на сторону)
	ramp  []bool       // клетки-лестницы: связывают этажи
	dist  [2][]int32
	built bool
	goal  engine.Vec2
}

// NewNav строит сетку навигации поверх поля физики. radius — радиус тела, под
// которое меряется проходимость: сетка одна на всех, поэтому берётся типичный
// ходок, а протискиваться в щели по-своему каждый будет уже физикой.
//
// Возвращает nil, если поля нет: навигация не обязательна, и её отсутствие
// означает лишь «идти напрямик».
func NewNav(f *physics.Field, step, radius float64) *NavField {
	if f == nil || step <= 0 {
		return nil
	}
	fw, fh := f.Size()
	sub := f.Sub()
	n := &NavField{
		f:    f,
		step: step,
		w:    int(math.Ceil(float64(fw) * sub / step)),
		h:    int(math.Ceil(float64(fh) * sub / step)),
		body: physics.Body{Radius: radius},
	}
	if n.w <= 0 || n.h <= 0 {
		return nil
	}
	size := n.w * n.h
	n.ramp = make([]bool, size)
	for floor := range 2 {
		n.pass[floor] = make([]bool, size)
		n.dist[floor] = make([]int32, size)
	}
	for y := range n.h {
		for x := range n.w {
			c := n.center(x, y)
			i := y*n.w + x
			n.ramp[i] = f.CellAt(c) == physics.Ramp
			for floor := range 2 {
				b := n.body
				b.Floor = uint8(floor)
				n.pass[floor][i] = f.Fits(c, b)
			}
		}
	}
	n.buildLinks()
	return n
}

// navDirs — восемь соседей в порядке битов маски связей.
var navDirs = [8][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {1, -1}, {-1, 1}, {-1, -1}}

// buildLinks считает, между какими соседними клетками вообще можно пройти.
//
// Проверять надо именно переходы, а не содержимое клеток. Стена толщиной в одну
// под-клетку не накрывает ни одного центра, и волна утекала бы сквозь неё; а
// если объявлять непроходимой всю клетку, где есть кусок скалы, то закрываются
// узкие проходы — сетка грубее прохода. Отрезок между центрами отвечает на
// вопрос честно и считается один раз: геометрия карты не меняется.
func (n *NavField) buildLinks() {
	for floor := range 2 {
		n.link[floor] = make([]uint8, n.w*n.h)
		for y := range n.h {
			for x := range n.w {
				if !n.passable(floor, x, y) {
					continue
				}
				var mask uint8
				for b, d := range navDirs {
					nx, ny := x+d[0], y+d[1]
					if !n.passable(floor, nx, ny) {
						continue
					}
					// Диагональ — только когда открыты обе смежные клетки:
					// иначе враг срезает угол скалы.
					if d[0] != 0 && d[1] != 0 &&
						(!n.passable(floor, x+d[0], y) || !n.passable(floor, x, y+d[1])) {
						continue
					}
					if !n.clear(n.center(x, y), n.center(nx, ny)) {
						continue
					}
					mask |= 1 << b
				}
				n.link[floor][y*n.w+x] = mask
			}
		}
	}
}

// clear — свободен ли отрезок между центрами от скалы.
func (n *NavField) clear(a, b engine.Vec2) bool {
	d := b.Sub(a)
	steps := int(math.Ceil(d.Len() / (n.f.Sub() / 2)))
	for i := 1; i < steps; i++ {
		if n.f.CellAt(a.Add(d.Scale(float64(i)/float64(steps)))) == physics.Solid {
			return false
		}
	}
	return true
}

// Rebuild разливает волну от цели. Вызывать раз в несколько тиков: цель за пару
// кадров далеко не уходит, а полный обход сетки стоит дороже, чем шаг врага.
func (n *NavField) Rebuild(goal engine.Vec2, floor uint8) {
	if n == nil {
		return
	}
	n.goal, n.built = goal, true
	for f := range 2 {
		d := n.dist[f]
		for i := range d {
			d[i] = navUnreached
		}
	}
	gx, gy := n.cell(goal)
	if !n.inside(gx, gy) {
		return
	}
	gf := int(floor)
	if gf > 1 {
		gf = 0
	}
	if !n.passable(gf, gx, gy) {
		// Цель в стене (встала на пропс, стоит на лестнице) — разливаем от
		// ближайшей проходимой клетки, иначе волны не будет вовсе.
		if fx, fy, ff, ok := n.nearestOpen(gx, gy, gf); ok {
			gx, gy, gf = fx, fy, ff
		} else {
			return
		}
	}

	// Дейкстра на ведре: стоимостей всего две, поэтому очередь с приоритетом
	// заменяется обычной очередью с повторной вставкой — сетка мелкая, и это
	// быстрее любой кучи.
	type node struct{ x, y, f int }
	queue := []node{{gx, gy, gf}}
	n.dist[gf][gy*n.w+gx] = 0
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		base := n.dist[cur.f][cur.y*n.w+cur.x]

		mask := n.link[cur.f][cur.y*n.w+cur.x]
		for b, d := range navDirs {
			if mask&(1<<b) == 0 {
				continue
			}
			cost := navStraight
			if d[0] != 0 && d[1] != 0 {
				cost = navDiag
			}
			if n.relax(cur.f, cur.x+d[0], cur.y+d[1], base+cost) {
				queue = append(queue, node{cur.x + d[0], cur.y + d[1], cur.f})
			}
		}
		// Лестница связывает этажи: с неё волна перетекает на соседний.
		if n.ramp[cur.y*n.w+cur.x] {
			other := 1 - cur.f
			if n.passable(other, cur.x, cur.y) && n.relax(other, cur.x, cur.y, base+navStraight) {
				queue = append(queue, node{cur.x, cur.y, other})
			}
		}
	}
}

func (n *NavField) relax(f, x, y int, v int32) bool {
	i := y*n.w + x
	if d := n.dist[f][i]; d != navUnreached && d <= v {
		return false
	}
	n.dist[f][i] = v
	return true
}

// Dist — расстояние до цели по проходимым клеткам, в пикселях. ok=false, если
// цель отсюда недостижима (за стеной без обхода) или волны ещё не было.
func (n *NavField) Dist(p engine.Vec2, floor uint8) (float64, bool) {
	if n == nil || !n.built {
		return 0, false
	}
	x, y := n.cell(p)
	f := int(floor)
	if f > 1 {
		f = 0
	}
	if !n.inside(x, y) {
		return 0, false
	}
	d := n.dist[f][y*n.w+x]
	if d == navUnreached {
		// Тело может стоять в клетке, которую сетка считает тесной. Смотрим
		// вокруг: путь есть, просто центр клетки занят.
		if v, ok := n.around(x, y, f); ok {
			d = v
		} else {
			return 0, false
		}
	}
	return float64(d) / float64(navStraight) * n.step, true
}

// Step — единичный вектор к следующей клетке по пути к цели. ok=false, если
// пути нет: тогда враг идёт напрямик, как раньше.
func (n *NavField) Step(p engine.Vec2, floor uint8) (engine.Vec2, bool) {
	if n == nil || !n.built {
		return engine.Vec2{}, false
	}
	x, y := n.cell(p)
	f := int(floor)
	if f > 1 {
		f = 0
	}
	if !n.inside(x, y) {
		return engine.Vec2{}, false
	}
	best := int32(math.MaxInt32)
	if d := n.dist[f][y*n.w+x]; d != navUnreached {
		best = d
	}
	var bx, by int
	found := false
	mask := n.link[f][y*n.w+x]
	for b, d := range navDirs {
		if mask&(1<<b) == 0 {
			continue // туда хода нет: между центрами скала
		}
		nx, ny := x+d[0], y+d[1]
		v := n.dist[f][ny*n.w+nx]
		if v == navUnreached || v >= best {
			continue
		}
		best, bx, by, found = v, nx, ny, true
	}
	if !found {
		return engine.Vec2{}, false
	}
	return n.center(bx, by).Sub(p).Normalized(), true
}

// Goal — точка, от которой разливалась последняя волна.
func (n *NavField) Goal() engine.Vec2 { return n.goal }

func (n *NavField) inside(x, y int) bool { return x >= 0 && y >= 0 && x < n.w && y < n.h }

func (n *NavField) passable(f, x, y int) bool {
	return n.inside(x, y) && n.pass[f][y*n.w+x]
}

func (n *NavField) cell(p engine.Vec2) (int, int) {
	return int(math.Floor(p.X / n.step)), int(math.Floor(p.Y / n.step))
}

func (n *NavField) center(x, y int) engine.Vec2 {
	return engine.Vec2{X: (float64(x) + 0.5) * n.step, Y: (float64(y) + 0.5) * n.step}
}

// around — лучшее расстояние среди соседей клетки.
func (n *NavField) around(x, y, f int) (int32, bool) {
	best, ok := int32(math.MaxInt32), false
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if !n.passable(f, x+dx, y+dy) {
				continue
			}
			if d := n.dist[f][(y+dy)*n.w+x+dx]; d != navUnreached && d < best {
				best, ok = d, true
			}
		}
	}
	return best, ok
}

// nearestOpen ищет ближайшую проходимую клетку кольцами вокруг (x,y).
func (n *NavField) nearestOpen(x, y, f int) (int, int, int, bool) {
	for r := 1; r <= 4; r++ {
		for dy := -r; dy <= r; dy++ {
			for dx := -r; dx <= r; dx++ {
				if abs(dx) != r && abs(dy) != r {
					continue
				}
				for _, ff := range [2]int{f, 1 - f} {
					if n.passable(ff, x+dx, y+dy) {
						return x + dx, y + dy, ff, true
					}
				}
			}
		}
	}
	return 0, 0, 0, false
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
