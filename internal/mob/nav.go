package mob

import (
	"math"

	"github.com/vladislav/game/internal/engine"
	"github.com/vladislav/game/internal/physics"
)

// Навигация врагов: карты расстояний до цели и поиск пути по грубой сетке.
//
// Две разные задачи решаются разными способами, и это не случайность.
//
// К игроку идут все сразу, поэтому считать путь каждому незачем: от цели один
// раз разливается волна (Дейкстра по восьми соседям), и враг читает из своей
// клетки, куда шагнуть. Перестройка стоит один обход сетки на всю толпу.
//
// А вот к точке, где игрока видели в последний раз, каждый идёт своей: точки у
// всех разные, общей волны тут не построишь. Для этого есть Path — A* от врага
// до его собственной цели, считается редко и живёт в самом враге.
//
// Та же сетка отвечает и на вопрос слышимости: звук идёт по коридорам, а не
// сквозь скалу, и «расстояние по проходимым клеткам» — ровно то, что нужно.
//
// Сетка грубее физической: точность здесь не нужна, вдоль стен ведёт физика, а
// навигация отвечает только на вопрос «в какую сторону вообще».

// navUnreached — клетка, до которой волна не дошла.
const navUnreached int32 = -1

// Стоимость шага: прямой и диагональный. Целые, чтобы не считать в float.
const (
	navStraight int32 = 10
	navDiag     int32 = 14
)

// navRadii — размеры тел, под которые строятся полосы. Одной сетки на всех не
// хватает: проход, в который проскочит крыса, крупному врагу узок, и общая
// карта уверенно вела бы его в щель, где он застрянет.
//
// Три полосы, а не по одной на каждого: тел много, а ширин, различимых на сетке
// в 32 px, всего несколько.
// Числа взяты по факту: радиус тела — четверть ширины рамки пака, у мелких
// врагов это 15–16 px, у крупных (128-пиксельные паки) 26–32.
var navRadii = [3]float64{10, 16, 32}

// navDirs — восемь соседей в порядке битов маски связей.
var navDirs = [8][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {1, -1}, {-1, 1}, {-1, -1}}

// navLane — сетка под тело одного размера: проходимость, связи и волна.
type navLane struct {
	radius float64
	pass   [2][]bool
	link   [2][]uint8
	dist   [2][]int32
}

// NavField — навигация карты: по полосе на размер тела, по этажу в каждой
// полосе (низ и макушка плато связаны лестницами).
type NavField struct {
	f     *physics.Field
	step  float64
	w, h  int
	ramp  []bool
	lanes []*navLane
	built bool
	goal  engine.Vec2

	// Рабочие массивы поиска пути: A* зовут редко, но каждый вызов иначе
	// выделял бы сетку целиком. Метка версии избавляет от очистки.
	seen  []uint32
	mark  uint32
	gcost []int32
	from  []int32
}

// NewNav строит навигацию поверх поля физики. Возвращает nil, если поля нет:
// навигация не обязательна, и её отсутствие означает «идти напрямик».
func NewNav(f *physics.Field, step float64) *NavField {
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
	}
	if n.w <= 0 || n.h <= 0 {
		return nil
	}
	size := n.w * n.h
	n.ramp = make([]bool, size)
	n.seen = make([]uint32, size*2)
	n.gcost = make([]int32, size*2)
	n.from = make([]int32, size*2)

	for y := range n.h {
		for x := range n.w {
			n.ramp[y*n.w+x] = f.CellAt(n.center(x, y)) == physics.Ramp
		}
	}
	for _, r := range navRadii {
		n.lanes = append(n.lanes, n.buildLane(r))
	}
	return n
}

// buildLane считает проходимость и связи под тело радиуса r.
func (n *NavField) buildLane(r float64) *navLane {
	size := n.w * n.h
	l := &navLane{radius: r}
	for floor := range 2 {
		l.pass[floor] = make([]bool, size)
		l.dist[floor] = make([]int32, size)
		b := physics.Body{Radius: r, Floor: uint8(floor)}
		for y := range n.h {
			for x := range n.w {
				l.pass[floor][y*n.w+x] = n.f.Fits(n.center(x, y), b)
			}
		}
	}
	// Связи считаются один раз: геометрия карты не меняется.
	//
	// Проверять надо именно переходы, а не содержимое клеток. Стена толщиной в
	// одну под-клетку не накрывает ни одного центра, и волна утекала бы сквозь
	// неё; а если объявить непроходимой всю клетку, где есть кусок скалы, то
	// закроются узкие проходы — сетка грубее прохода.
	for floor := range 2 {
		l.link[floor] = make([]uint8, size)
		for y := range n.h {
			for x := range n.w {
				if !l.at(n, floor, x, y) {
					continue
				}
				var mask uint8
				for b, d := range navDirs {
					nx, ny := x+d[0], y+d[1]
					if !l.at(n, floor, nx, ny) {
						continue
					}
					// Диагональ — только когда открыты обе смежные клетки:
					// иначе враг срезает угол скалы.
					if d[0] != 0 && d[1] != 0 &&
						(!l.at(n, floor, x+d[0], y) || !l.at(n, floor, x, y+d[1])) {
						continue
					}
					if !n.clear(n.center(x, y), n.center(nx, ny)) {
						continue
					}
					mask |= 1 << b
				}
				l.link[floor][y*n.w+x] = mask
			}
		}
	}
	return l
}

func (l *navLane) at(n *NavField, floor, x, y int) bool {
	return n.inside(x, y) && l.pass[floor][y*n.w+x]
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

// laneFor — полоса под тело радиуса r: самая узкая из тех, что его пропускают.
// Тело шире всех полос ведётся по самой широкой — лучше грубо, чем никак.
func (n *NavField) laneFor(r float64) *navLane {
	for _, l := range n.lanes {
		if l.radius >= r {
			return l
		}
	}
	return n.lanes[len(n.lanes)-1]
}

// Rebuild разливает волну от цели по всем полосам. Вызывать раз в несколько
// тиков: цель за пару кадров далеко не уходит, а обход сетки стоит дороже шага.
func (n *NavField) Rebuild(goal engine.Vec2, floor uint8) {
	if n == nil {
		return
	}
	n.goal, n.built = goal, true
	for _, l := range n.lanes {
		n.wave(l, goal, floor)
	}
}

func (n *NavField) wave(l *navLane, goal engine.Vec2, floor uint8) {
	for f := range 2 {
		d := l.dist[f]
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
	if !l.at(n, gf, gx, gy) {
		// Цель в тесноте (встала на пропс, стоит вплотную к скале) — разливаем
		// от ближайшей проходимой клетки, иначе волны не будет вовсе.
		if fx, fy, ff, ok := n.nearestOpen(l, gx, gy, gf); ok {
			gx, gy, gf = fx, fy, ff
		} else {
			return
		}
	}

	// Дейкстра на очереди с повторной вставкой: стоимостей всего две, и на
	// мелкой сетке это быстрее любой кучи.
	type node struct{ x, y, f int }
	queue := []node{{gx, gy, gf}}
	l.dist[gf][gy*n.w+gx] = 0
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		base := l.dist[cur.f][cur.y*n.w+cur.x]

		mask := l.link[cur.f][cur.y*n.w+cur.x]
		for b, d := range navDirs {
			if mask&(1<<b) == 0 {
				continue
			}
			cost := navStraight
			if d[0] != 0 && d[1] != 0 {
				cost = navDiag
			}
			nx, ny := cur.x+d[0], cur.y+d[1]
			i := ny*n.w + nx
			if v := l.dist[cur.f][i]; v != navUnreached && v <= base+cost {
				continue
			}
			l.dist[cur.f][i] = base + cost
			queue = append(queue, node{nx, ny, cur.f})
		}
		// Лестница связывает этажи: с неё волна перетекает на соседний.
		if n.ramp[cur.y*n.w+cur.x] {
			other := 1 - cur.f
			i := cur.y*n.w + cur.x
			if l.at(n, other, cur.x, cur.y) {
				if v := l.dist[other][i]; v == navUnreached || v > base+navStraight {
					l.dist[other][i] = base + navStraight
					queue = append(queue, node{cur.x, cur.y, other})
				}
			}
		}
	}
}

// Dist — расстояние до цели по проходимым клеткам для тела радиуса r, в
// пикселях. ok=false, если цель отсюда недостижима или волны ещё не было.
//
// Для слышимости зовут с нулевым радиусом: звук пролезает там, где пролезет
// самый мелкий, и толщина тела ему не помеха.
func (n *NavField) Dist(p engine.Vec2, floor uint8, r float64) (float64, bool) {
	if n == nil || !n.built {
		return 0, false
	}
	l := n.laneFor(r)
	x, y := n.cell(p)
	f := clampFloor(floor)
	if !n.inside(x, y) {
		return 0, false
	}
	d := l.dist[f][y*n.w+x]
	if d == navUnreached {
		// Тело может стоять в клетке, которую сетка считает тесной. Смотрим
		// вокруг: путь есть, просто центр клетки занят.
		v, ok := n.around(l, x, y, f)
		if !ok {
			return 0, false
		}
		d = v
	}
	return float64(d) / float64(navStraight) * n.step, true
}

// Step — единичный вектор к следующей клетке по пути к цели волны.
func (n *NavField) Step(p engine.Vec2, floor uint8, r float64) (engine.Vec2, bool) {
	if n == nil || !n.built {
		return engine.Vec2{}, false
	}
	l := n.laneFor(r)
	x, y := n.cell(p)
	f := clampFloor(floor)
	if !n.inside(x, y) {
		return engine.Vec2{}, false
	}
	best := int32(math.MaxInt32)
	if d := l.dist[f][y*n.w+x]; d != navUnreached {
		best = d
	}
	var bx, by int
	found := false
	mask := l.link[f][y*n.w+x]
	for b, d := range navDirs {
		if mask&(1<<b) == 0 {
			continue // туда хода нет: между центрами скала
		}
		nx, ny := x+d[0], y+d[1]
		v := l.dist[f][ny*n.w+nx]
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

// Path ищет дорогу от from до to для тела радиуса r и возвращает точки, по
// которым идти. Нужен там, где общей волны нет: к последней известной точке,
// домой, к своему месту в окружении — цели у всех разные.
//
// Возвращает nil, если пути нет. Первая точка — уже следующая: клетку, в
// которой стоишь, обходить незачем.
func (n *NavField) Path(from, to engine.Vec2, floor uint8, r float64) []engine.Vec2 {
	if n == nil {
		return nil
	}
	l := n.laneFor(r)
	f := clampFloor(floor)
	sx, sy := n.cell(from)
	gx, gy := n.cell(to)
	if !n.inside(sx, sy) || !n.inside(gx, gy) {
		return nil
	}
	if !l.at(n, f, gx, gy) {
		fx, fy, ff, ok := n.nearestOpen(l, gx, gy, f)
		if !ok {
			return nil
		}
		gx, gy, f = fx, fy, ff
	}
	start, goal := f*n.w*n.h+sy*n.w+sx, f*n.w*n.h+gy*n.w+gx
	if start == goal {
		return nil
	}

	n.mark++
	if n.mark == 0 { // переполнение метки: чистим и начинаем заново
		for i := range n.seen {
			n.seen[i] = 0
		}
		n.mark = 1
	}
	h := func(i int) int32 {
		x, y := (i%(n.w*n.h))%n.w, (i%(n.w*n.h))/n.w
		dx, dy := abs(x-gx), abs(y-gy)
		// Восьмисвязная эвристика: диагональ дешевле двух прямых.
		return int32(min(dx, dy))*navDiag + int32(abs(dx-dy))*navStraight
	}

	open := &navHeap{}
	n.seen[start], n.gcost[start], n.from[start] = n.mark, 0, -1
	open.push(navItem{idx: start, cost: h(start)})

	for open.len() > 0 {
		cur := open.pop().idx
		if cur == goal {
			return n.unwind(cur, from)
		}
		cf := cur / (n.w * n.h)
		ci := cur % (n.w * n.h)
		cx, cy := ci%n.w, ci/n.w
		base := n.gcost[cur]

		mask := l.link[cf][ci]
		for b, d := range navDirs {
			if mask&(1<<b) == 0 {
				continue
			}
			cost := navStraight
			if d[0] != 0 && d[1] != 0 {
				cost = navDiag
			}
			nx, ny := cx+d[0], cy+d[1]
			ni := cf*n.w*n.h + ny*n.w + nx
			if n.seen[ni] == n.mark && n.gcost[ni] <= base+cost {
				continue
			}
			n.seen[ni], n.gcost[ni], n.from[ni] = n.mark, base+cost, int32(cur)
			open.push(navItem{idx: ni, cost: base + cost + h(ni)})
		}
		if n.ramp[ci] {
			other := 1 - cf
			ni := other*n.w*n.h + ci
			if l.at(n, other, cx, cy) && (n.seen[ni] != n.mark || n.gcost[ni] > base+navStraight) {
				n.seen[ni], n.gcost[ni], n.from[ni] = n.mark, base+navStraight, int32(cur)
				open.push(navItem{idx: ni, cost: base + navStraight + h(ni)})
			}
		}
	}
	return nil
}

// unwind разворачивает найденный путь в список точек от ближней к дальней.
func (n *NavField) unwind(goal int, from engine.Vec2) []engine.Vec2 {
	var back []engine.Vec2
	for i := goal; i >= 0; i = int(n.from[i]) {
		ci := i % (n.w * n.h)
		back = append(back, n.center(ci%n.w, ci/n.w))
		if n.from[i] < 0 {
			break
		}
	}
	if len(back) <= 1 {
		return nil
	}
	// Первая точка — клетка, в которой уже стоим: она только тянула бы назад.
	out := make([]engine.Vec2, 0, len(back)-1)
	for i := len(back) - 2; i >= 0; i-- {
		out = append(out, back[i])
	}
	return out
}

// Width — ширина прохода в точке p поперёк направления dir, в пикселях.
//
// Считать «узость» числом открытых соседей нельзя: на мелкой сетке даже дверь в
// четыре клетки открыта со всех сторон, и коридор от поля не отличить. А вот
// ширина поперёк движения — ровно то, что делает место узким: сколько свободно
// вбок, столько его и не обойти.
func (n *NavField) Width(p engine.Vec2, dir engine.Vec2, floor uint8, r float64) float64 {
	if n == nil || dir.Len() == 0 {
		return math.Inf(1)
	}
	l := n.laneFor(r)
	f := clampFloor(floor)
	x, y := n.cell(p)
	if !l.at(n, f, x, y) {
		return 0
	}
	perp := engine.Vec2{X: -dir.Y, Y: dir.X}.Normalized()
	free := 1
	for _, sign := range [2]float64{1, -1} {
		for k := 1; k <= widthLook; k++ {
			q := p.Add(perp.Scale(sign * float64(k) * n.step))
			qx, qy := n.cell(q)
			if !l.at(n, f, qx, qy) {
				break
			}
			free++
		}
	}
	return float64(free) * n.step
}

// widthLook — насколько далеко вбок мерить ширину. Дальше десяти клеток проход
// уже не проход: перекрыть его одним телом всё равно нельзя.
const widthLook = 10

// Choke ищет узкое место — дверь, проход между скалами — в стороне dir от точки
// around, не дальше maxDist. Возвращает false, если вокруг чистое поле: там
// перекрывать нечего.
//
// Это и есть «займи выход»: в коридоре отрезающий встаёт в проход, а не рядом с
// целью, и обойти его нельзя.
func (n *NavField) Choke(around, dir engine.Vec2, floor uint8, r, maxDist float64) (engine.Vec2, bool) {
	if n == nil || dir.Len() == 0 {
		return engine.Vec2{}, false
	}
	l := n.laneFor(r)
	f := clampFloor(floor)
	d := dir.Normalized()
	cx, cy := n.cell(around)
	span := int(maxDist/n.step) + 1

	best := engine.Vec2{}
	bestWidth, bestDist := math.Inf(1), math.Inf(1)
	for dy := -span; dy <= span; dy++ {
		for dx := -span; dx <= span; dx++ {
			x, y := cx+dx, cy+dy
			if !l.at(n, f, x, y) {
				continue
			}
			p := n.center(x, y)
			v := p.Sub(around)
			dist := v.Len()
			if dist > maxDist || dist < n.step {
				continue
			}
			if v.Normalized().Dot(d) < 0.4 { // не в ту сторону
				continue
			}
			w := n.Width(p, d, floor, r)
			if w > bestWidth || (w == bestWidth && dist >= bestDist) {
				continue
			}
			best, bestWidth, bestDist = p, w, dist
		}
	}
	// Проход шире четырёх тел перекрывать бессмысленно: это уже поле, его
	// обойдут.
	if bestWidth > 8*r {
		return engine.Vec2{}, false
	}
	return best, true
}

// Goal — точка, от которой разливалась последняя волна.
func (n *NavField) Goal() engine.Vec2 { return n.goal }

// Step — сторона клетки навигации в пикселях (нужна тем, кто меряет допуски в
// клетках, а не в пикселях).
func (n *NavField) CellSize() float64 { return n.step }

func (n *NavField) inside(x, y int) bool { return x >= 0 && y >= 0 && x < n.w && y < n.h }

func (n *NavField) cell(p engine.Vec2) (int, int) {
	return int(math.Floor(p.X / n.step)), int(math.Floor(p.Y / n.step))
}

func (n *NavField) center(x, y int) engine.Vec2 {
	return engine.Vec2{X: (float64(x) + 0.5) * n.step, Y: (float64(y) + 0.5) * n.step}
}

// around — лучшее расстояние среди соседей клетки.
func (n *NavField) around(l *navLane, x, y, f int) (int32, bool) {
	best, ok := int32(math.MaxInt32), false
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if !l.at(n, f, x+dx, y+dy) {
				continue
			}
			if d := l.dist[f][(y+dy)*n.w+x+dx]; d != navUnreached && d < best {
				best, ok = d, true
			}
		}
	}
	return best, ok
}

// nearestOpen ищет ближайшую проходимую клетку кольцами вокруг (x,y).
func (n *NavField) nearestOpen(l *navLane, x, y, f int) (int, int, int, bool) {
	for r := 1; r <= 4; r++ {
		for dy := -r; dy <= r; dy++ {
			for dx := -r; dx <= r; dx++ {
				if abs(dx) != r && abs(dy) != r {
					continue
				}
				for _, ff := range [2]int{f, 1 - f} {
					if l.at(n, ff, x+dx, y+dy) {
						return x + dx, y + dy, ff, true
					}
				}
			}
		}
	}
	return 0, 0, 0, false
}

func clampFloor(f uint8) int {
	if f > 1 {
		return 0
	}
	return int(f)
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// navItem — узел очереди A*.
type navItem struct {
	idx  int
	cost int32
}

// navHeap — двоичная куча: обычная очередь здесь не годится, у A* стоимости
// разные.
type navHeap struct{ items []navItem }

func (h *navHeap) len() int { return len(h.items) }

func (h *navHeap) push(v navItem) {
	h.items = append(h.items, v)
	i := len(h.items) - 1
	for i > 0 {
		p := (i - 1) / 2
		if h.items[p].cost <= h.items[i].cost {
			break
		}
		h.items[p], h.items[i] = h.items[i], h.items[p]
		i = p
	}
}

func (h *navHeap) pop() navItem {
	top := h.items[0]
	last := len(h.items) - 1
	h.items[0] = h.items[last]
	h.items = h.items[:last]
	for i := 0; ; {
		l, r := 2*i+1, 2*i+2
		small := i
		if l < last && h.items[l].cost < h.items[small].cost {
			small = l
		}
		if r < last && h.items[r].cost < h.items[small].cost {
			small = r
		}
		if small == i {
			break
		}
		h.items[i], h.items[small] = h.items[small], h.items[i]
		i = small
	}
	return top
}
