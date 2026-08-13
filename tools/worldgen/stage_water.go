package main

// stage_water.go — Фаза 2: внутренняя вода (пруды + извилистые реки) и грунтовые
// тропы. Пруды/реки вырезаются в сетке уровней ДО автотайлинга; тропы копятся
// набором клеток и позже красятся угловым mud поверх травы. Порт из
// generate_map.py: пруды — круги с шумовой каймой, реки/тропы — конвейер
// A*→меандр→Чайкин→кисть (curves.go).

import "math"

// stageWater вырезает пруды и реки в g.Level (клетки становятся LiquidDeep).
// Режет ТОЛЬКО нижнюю землю: возвышенность для воды — препятствие, река её
// обтекает. Раньше предикат брал isLand(), река шла сквозь плато и разваливала
// его на ленты уже после проверки толщины (E4).
// Позиции прудов и устий выводятся из сида — иначе гидрография на всех картах
// биома получалась одинаковой.
func (g *Generator) stageWater() {
	W, H := g.P.Width, g.P.Height
	land := func(x, y int) bool { return g.Level.In(x, y) && g.Level.At(x, y) == Ground }
	// jitter — смещение опорной точки на ±spread долей от сида
	jitter := func(base float64, salt int, spread float64) float64 {
		return base + (hash2(salt*7919, salt*104729, g.Seed)-0.5)*2*spread
	}

	type pond struct {
		cx, cy int
		rr     float64
		seed   uint64
	}
	ponds := []pond{
		{int(float64(W) * jitter(0.72, 1, 0.10)), int(float64(H) * jitter(0.26, 2, 0.10)), 7, 101 + g.Seed},
		{int(float64(W) * jitter(0.24, 3, 0.10)), int(float64(H) * jitter(0.70, 4, 0.10)), 5.5, 103 + g.Seed},
		{int(float64(W) * jitter(0.55, 5, 0.10)), int(float64(H) * jitter(0.82, 6, 0.08)), 4.5, 107 + g.Seed},
	}
	carve := map[[2]int]bool{}
	for _, p := range ponds {
		for y := 0; y < H; y++ {
			for x := 0; x < W; x++ {
				if !land(x, y) {
					continue
				}
				edge := p.rr + 3.2*cnoise(float64(x)*0.14, float64(y)*0.14, p.seed)*4
				if math.Hypot(float64(x-p.cx), float64(y-p.cy)) < edge {
					carve[[2]int{x, y}] = true
				}
			}
		}
	}

	// извилистые реки между прудами: A* по нижней земле, стоимость от шума
	rivers := [][4]int{
		{ponds[0].cx, ponds[0].cy, ponds[1].cx, ponds[1].cy},
		{ponds[2].cx, ponds[2].cy, ponds[0].cx, ponds[0].cy},
	}
	seed := 201 + int(g.Seed%1000)
	for _, r := range rivers {
		cost := func(seed uint64) func(x, y int) float64 {
			return func(x, y int) float64 { return 1 + 5*math.Abs(cnoise(float64(x)*0.09, float64(y)*0.09, seed)) }
		}(uint64(seed))
		path := astar(r[0], r[1], r[2], r[3], land, cost)
		if path != nil {
			curve := chaikin(meander(simplify(path, 4), 8.0, uint64(seed)), 4)
			w0, w1 := 0.85, 1.5
			for c := range brush(curve, func(t float64) float64 { return w0 + (w1-w0)*t }, land) {
				carve[c] = true
			}
		}
		seed += 2
	}

	for c := range carve {
		g.Level.Set(c[0], c[1], LiquidDeep)
	}
}

// Параметры троп. trailClearWant — запас до воды/плато, при котором травяная
// кайма получается у ВСЕЙ полосы: сама полоса уходит от осевой линии максимум на
// 1 клетку, ей нужен ещё 1 ряд травы, итого 3.
const (
	trailClearWant = 3
	trailClearMax  = 6 // дальше от воды разницы для стоимости нет
	trailApart     = 2 // на столько маршруты расходятся друг от друга
)

// stageTrails прокладывает сеть грунтовых троп по нижней земле и копит их
// клетки в g.Trail. Три правила тропы:
//
//  1. ширина ровно 2 клетки — рисуем brushBand, а не brush: круглая кисть
//     центрирует диск на клетке и даёт только нечётную ширину (раньше радиус
//     1.45, т.е. ~3 клетки);
//  2. тропа не идёт вплотную к воде и к плато — шаг A* рядом с ними стоит в
//     разы дороже (поле clearance), и маршрут сам уходит вглубь суши;
//  3. тропа обрамлена травой — это следствие (2), а не отдельный проход: при
//     clearance ≥ 2 у клетки тропы все 8 соседей — обычная нижняя земля, и mud
//     ложится на grass_ground со всех сторон. Там, где обойти нельзя (перешеек
//     между прудами, проход под обрывом, край карты), штраф просто платится и
//     каймы с этой стороны нет — как и задумано.
func (g *Generator) stageTrails() {
	g.Trail = map[[2]int]bool{}
	W, H := g.P.Width, g.P.Height
	// clr — расстояние Чебышёва до ближайшей «чужой» клетки: воды, плато, тела
	// обрыва или края карты. 1 — тропа вплотную к ней, 2 — через клетку травы.
	clr := g.trailClearance()
	// тропа идёт по нижней земле и обходит тело обрыва: скала непроходима,
	// грунт по ней рисовать нельзя.
	land := func(x, y int) bool { return g.Level.In(x, y) && clr.At(x, y) >= 1 }

	fr := func(fx, fy float64, salt int) [2]int {
		j := func(v float64, s int) float64 { return v + (hash2(s*6151, s*24593, g.Seed)-0.5)*0.12 }
		return [2]int{int(float64(W) * j(fx, salt)), int(float64(H) * j(fy, salt+50))}
	}
	raw := [][2]int{
		fr(0.12, 0.46, 1), fr(0.28, 0.20, 2), fr(0.46, 0.34, 3), fr(0.62, 0.62, 4),
		fr(0.84, 0.38, 5), fr(0.78, 0.80, 6), fr(0.44, 0.84, 7), fr(0.20, 0.74, 8),
	}
	var wp [][2]int
	for _, p := range raw {
		// опорную точку сдвигаем на просторное место: конец маршрута штрафом не
		// исправить, и тропа иначе упирается прямо в берег.
		if c, ok := nudgeToClear(clr, p[0], p[1], 6); ok {
			wp = append(wp, c)
		}
	}
	// near — окрестность уже проложенных троп. Без штрафа соседний маршрут идёт
	// впритирку к предыдущему, и две полосы по 2 клетки сливаются в кляксу на 4.
	// Именно штраф, а не запрет: пересечь тропу поперёк (перекрёсток) можно.
	near := NewGrid[bool](W, H)
	cost := func(x, y int) float64 {
		c := 1 + 3*math.Abs(cnoise(float64(x)*0.10, float64(y)*0.10, 401))
		if d := clr.At(x, y); d < trailClearWant {
			c += 12 * float64(trailClearWant-d) // 1 → +24, 2 → +12
		}
		if near.At(x, y) {
			c += 10
		}
		return c
	}
	for i := 0; i+1 < len(wp); i++ {
		path := astar(wp[i][0], wp[i][1], wp[i+1][0], wp[i+1][1], land, cost)
		if path == nil {
			continue
		}
		curve := chaikin(meander(simplify(path, 5), 3.0, uint64(500+i)), 4)
		cells := brushBand(curve, land)
		for c := range cells {
			g.Trail[c] = true
		}
		for c := range cells {
			for dy := -trailApart; dy <= trailApart; dy++ {
				for dx := -trailApart; dx <= trailApart; dx++ {
					if near.In(c[0]+dx, c[1]+dy) {
						near.Set(c[0]+dx, c[1]+dy, true)
					}
				}
			}
		}
	}
}

// trailClearance — поле расстояния Чебышёва до ближайшей клетки, по которой
// тропе ходить нельзя: вода, плато, тело обрыва и всё, что за краем карты
// (край считается «чужим», поэтому тропа не липнет к рамке). 0 — сама такая
// клетка, значения обрезаны сверху trailClearMax. Два прохода чамфера.
func (g *Generator) trailClearance() *Grid[int] {
	W, H := g.P.Width, g.P.Height
	d := NewGrid[int](W, H)
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			if g.Level.At(x, y) == Ground && !g.Cliff[[2]int{x, y}] {
				d.Set(x, y, trailClearMax)
			}
		}
	}
	relax := func(x, y int, offs [][2]int) {
		if d.At(x, y) == 0 {
			return
		}
		best := d.At(x, y)
		for _, o := range offs {
			nx, ny := x+o[0], y+o[1]
			v := 0 // за краем карты — «чужая» клетка
			if d.In(nx, ny) {
				v = d.At(nx, ny)
			}
			if v+1 < best {
				best = v + 1
			}
		}
		d.Set(x, y, best)
	}
	fwd := [][2]int{{-1, -1}, {0, -1}, {1, -1}, {-1, 0}}
	bwd := [][2]int{{1, 1}, {0, 1}, {-1, 1}, {1, 0}}
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			relax(x, y, fwd)
		}
	}
	for y := H - 1; y >= 0; y-- {
		for x := W - 1; x >= 0; x-- {
			relax(x, y, bwd)
		}
	}
	return d
}

// nudgeToClear ищет рядом с (x,y) клетку с запасом до воды/плато ≥ trailClearWant,
// беря ближайшую подходящую. Если такой нет — берёт самую просторную из
// найденных; false — если вокруг вообще нет земли под тропу.
func nudgeToClear(clr *Grid[int], x, y, maxR int) ([2]int, bool) {
	best, bestD := [2]int{}, 0
	for r := 0; r <= maxR; r++ {
		for dy := -r; dy <= r; dy++ {
			for dx := -r; dx <= r; dx++ {
				if absInt(dx) != r && absInt(dy) != r {
					continue // только кольцо радиуса r
				}
				nx, ny := x+dx, y+dy
				if !clr.In(nx, ny) {
					continue
				}
				if v := clr.At(nx, ny); v > bestD {
					best, bestD = [2]int{nx, ny}, v
					if v >= trailClearWant {
						return best, true
					}
				}
			}
		}
	}
	return best, bestD >= 1
}
