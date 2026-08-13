package main

// stage_props.go — шаги 16-18 (worldgen.spec §5, §7): объекты (Пуассон по
// футпринту), поверхность (кувшинки/камыш) и маркеры.

import "math/rand"

// baseRadius — базовый радиус Пуассона по зоне (тайлов), §7.
func baseRadius(zone string) float64 {
	switch zone {
	case "dense":
		return 3
	case "mid":
		return 4
	case "plateau":
		return 4
	default: // open
		return 6
	}
}

// zoneAt определяет зону клетки по уровню и влажности.
func (g *Generator) zoneAt(x, y int) string {
	if g.Level.At(x, y) == Plateau {
		return "plateau"
	}
	m := g.Moisture.At(x, y)
	switch {
	case m > 0.58:
		return "dense"
	case m > 0.42:
		return "mid"
	default:
		return "open"
	}
}

// stageSpawn выбирает точку появления. Отдельная стадия, а не часть stageProps:
// раньше спавн вычислялся внутри неё, а она выходит сразу, если у биома нет
// пропсов — у forest их нет, поэтому маркер спавна не появлялся никогда (E5).
func (g *Generator) stageSpawn() {
	if x, y, ok := g.pickSpawn(); ok {
		g.spawn = [2]int{x, y}
		g.hasSpawn = true
	}
}

// pickSpawn выбирает точку появления — клетку нижней земли ближе к центру,
// не занятую телом обрыва.
func (g *Generator) pickSpawn() (int, int, bool) {
	cx, cy := g.P.Width/2, g.P.Height/2
	best, bx, by := 1<<30, -1, -1
	for y := 1; y < g.P.Height-1; y++ {
		for x := 1; x < g.P.Width-1; x++ {
			if g.Level.At(x, y) != Ground || g.Cliff[[2]int{x, y}] {
				continue
			}
			d := (x-cx)*(x-cx) + (y-cy)*(y-cy)
			if d < best {
				best, bx, by = d, x, y
			}
		}
	}
	return bx, by, bx >= 0
}

// stageProps — шаг 16: расстановка объектов Пуассоном с радиусом от футпринта.
func (g *Generator) stageProps() {
	if len(g.Manifest.Props) == 0 {
		return
	}
	rng := rand.New(rand.NewSource(int64(g.Seed) ^ 0x5DEECE66D))
	W, H := g.P.Width, g.P.Height

	// сетка занятости (радиус Пуассона) и маска запрета анкеров
	blocked := make([]bool, W*H)
	if g.hasSpawn {
		markDisc(blocked, W, H, g.spawn[0], g.spawn[1], 6) // §7: 6 тайлов вокруг спавна
	}

	// индекс пропсов по зонам для быстрого взвешенного выбора
	byZone := map[string][]int{}
	for i, p := range g.Manifest.Props {
		for _, z := range p.Zone {
			byZone[z] = append(byZone[z], i)
		}
	}

	// число попыток пропорционально площади — радиус сам ограничит плотность
	attempts := W * H
	for a := 0; a < attempts; a++ {
		cx := rng.Intn(W)
		cy := rng.Intn(H)
		if blocked[cy*W+cx] {
			continue
		}
		lv := g.Level.At(cx, cy)
		if !lv.isLand() {
			continue
		}
		zone := g.zoneAt(cx, cy)
		cands := byZone[zone]
		if len(cands) == 0 {
			continue
		}
		pi := g.pickWeighted(rng, cands, lv)
		if pi < 0 {
			continue
		}
		p := g.Manifest.Props[pi]
		if g.placeProp(blocked, p, cx, cy) {
			// радиус от футпринта: max(base, (w+h)/2+1)
			r := baseRadius(zone)
			if fr := float64(p.Footprint[0]+p.Footprint[1])/2 + 1; fr > r {
				r = fr
			}
			markDisc(blocked, W, H, cx, cy, int(r))
		}
	}
}

// pickWeighted выбирает пропс из cands по весу, отсекая несовместимые с уровнем.
func (g *Generator) pickWeighted(rng *rand.Rand, cands []int, lv Level) int {
	total := 0
	for _, i := range cands {
		if g.propFitsLevel(g.Manifest.Props[i], lv) {
			total += g.Manifest.Props[i].Weight
		}
	}
	if total == 0 {
		return -1
	}
	r := rng.Intn(total)
	for _, i := range cands {
		p := g.Manifest.Props[i]
		if !g.propFitsLevel(p, lv) {
			continue
		}
		if r < p.Weight {
			return i
		}
		r -= p.Weight
	}
	return -1
}

func (g *Generator) propFitsLevel(p Prop, lv Level) bool {
	want := "ground_a"
	if lv == Plateau {
		want = "plateau"
	}
	for _, o := range p.On {
		if o == want {
			return true
		}
	}
	return false
}

// placeProp пытается поставить пропс якорем в (cx,cy). Проверяет, что весь
// футпринт лежит на подходящей земле и не занят. Возвращает true при успехе.
func (g *Generator) placeProp(blocked []bool, p Prop, cx, cy int) bool {
	w, h := p.Footprint[0], p.Footprint[1]
	x0 := cx - w/2
	y0 := cy - h + 1
	if x0 < 0 || y0 < 0 || x0+w > g.P.Width || y0+h > g.P.Height {
		return false
	}
	for yy := y0; yy < y0+h; yy++ {
		for xx := x0; xx < x0+w; xx++ {
			if !g.Level.At(xx, yy).isLand() || blocked[yy*g.P.Width+xx] {
				return false // вода под футпринтом или уже занято другим пропсом
			}
		}
	}
	// пометить весь футпринт занятым — против пересечений (E8)
	for yy := y0; yy < y0+h; yy++ {
		for xx := x0; xx < x0+w; xx++ {
			blocked[yy*g.P.Width+xx] = true
		}
	}
	file := p.File
	if p.SwapOnGroundB != "" && g.groundBAt(cx, cy) {
		file = p.SwapOnGroundB
	}
	g.Props = append(g.Props, PropInst{
		ID: p.ID, File: file, X: x0, Y: y0, W: w, H: h,
		Anchor: [2]int{w / 2, h}, SortY: cy, Collides: p.Collides,
	})
	return true
}

// groundBAt — под якорем покрытие ground_b (влажностная зона). Заглушка до M4.
func (g *Generator) groundBAt(x, y int) bool { return false }

// markDisc помечает занятыми клетки в радиусе r вокруг (cx,cy).
func markDisc(blocked []bool, W, H, cx, cy, r int) {
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			if x < 0 || y < 0 || x >= W || y >= H {
				continue
			}
			dx, dy := x-cx, y-cy
			if dx*dx+dy*dy <= r*r {
				blocked[y*W+x] = true
			}
		}
	}
}

// stageMarkers — шаг 18: точка появления и выходы.
func (g *Generator) stageMarkers() {
	if g.hasSpawn {
		g.Marks = append(g.Marks, Marker{Kind: "spawn", X: g.spawn[0], Y: g.spawn[1]})
	}
}
