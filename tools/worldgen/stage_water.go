package main

// stage_water.go — Фаза 2: внутренняя вода (пруды + извилистые реки) и грунтовые
// тропы. Пруды/реки вырезаются в сетке уровней ДО автотайлинга; тропы копятся
// набором клеток и позже красятся угловым mud поверх травы. Порт из
// generate_map.py: пруды — круги с шумовой каймой, реки/тропы — конвейер
// A*→меандр→Чайкин→кисть (curves.go).

import "math"

// stageWater вырезает пруды и реки в g.Level (клетки становятся LiquidDeep).
// Работает только по исходной суше; океан и рамку не трогает.
func (g *Generator) stageWater() {
	W, H := g.P.Width, g.P.Height
	land := func(x, y int) bool { return g.Level.In(x, y) && g.Level.At(x, y).isLand() }

	type pond struct {
		cx, cy int
		rr     float64
		seed   uint64
	}
	ponds := []pond{
		{int(float64(W) * 0.72), int(float64(H) * 0.26), 7, 101},
		{int(float64(W) * 0.24), int(float64(H) * 0.70), 5.5, 103},
		{int(float64(W) * 0.55), int(float64(H) * 0.82), 4.5, 107},
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

	// извилистые реки между прудами: A* по суше, стоимость от шума
	rivers := [][4]int{
		{int(float64(W) * 0.72), int(float64(H) * 0.26), int(float64(W) * 0.24), int(float64(H) * 0.70)},
		{int(float64(W) * 0.55), int(float64(H) * 0.82), int(float64(W) * 0.72), int(float64(H) * 0.26)},
	}
	seed := 201
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
	land := func(x, y int) bool { return g.Level.In(x, y) && g.Level.At(x, y).isLand() }

	fr := func(fx, fy float64) [2]int { return [2]int{int(float64(W) * fx), int(float64(H) * fy)} }
	raw := [][2]int{
		fr(0.12, 0.46), fr(0.28, 0.20), fr(0.46, 0.34), fr(0.62, 0.62),
		fr(0.84, 0.38), fr(0.78, 0.80), fr(0.44, 0.84), fr(0.20, 0.74),
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
