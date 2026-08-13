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

// stageTrails прокладывает сеть грунтовых троп по суше и копит их клетки в
// g.Trail. Ширина кисти 1.45 (диаметр ~3): у́же — у углового автотайла нет
// клетки заливки, и тропа рассыпается (SUMMARY §«Ширина троп»).
func (g *Generator) stageTrails() {
	g.Trail = map[[2]int]bool{}
	W, H := g.P.Width, g.P.Height
	// тропа идёт по нижней земле и обходит тело обрыва: скала непроходима,
	// грунт по ней рисовать нельзя.
	land := func(x, y int) bool {
		return g.Level.In(x, y) && g.Level.At(x, y) == Ground && !g.Cliff[[2]int{x, y}]
	}

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
		if land(p[0], p[1]) {
			wp = append(wp, p)
		}
	}
	cost := func(x, y int) float64 { return 1 + 3*math.Abs(cnoise(float64(x)*0.10, float64(y)*0.10, 401)) }
	for i := 0; i+1 < len(wp); i++ {
		path := astar(wp[i][0], wp[i][1], wp[i+1][0], wp[i+1][1], land, cost)
		if path == nil {
			continue
		}
		curve := chaikin(meander(simplify(path, 5), 3.0, uint64(500+i)), 4)
		for c := range brush(curve, func(t float64) float64 { return 1.45 }, land) {
			g.Trail[c] = true
		}
	}
}
