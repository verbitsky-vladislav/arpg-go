package main

// stage_tiles.go — шаги 10-15 (worldgen.spec §5): переходные и краевые тайлы,
// раскладываемые в разрежённые слои.

import "math/rand"

// stageCoast — шаг 10: береговые тайлы. В исходном тайлсете берег — отдельный
// слой на КРАЕВЫХ клетках суши (маска относительно суши), поверх ровной травы.
func (g *Generator) stageCoast() {
	tr, ok := g.Manifest.Transitions["liquid->ground_a"]
	if !ok || len(tr.Set) == 0 {
		return
	}
	isLand := func(l Level) bool { return l.isLand() }
	for y := 0; y < g.P.Height; y++ {
		for x := 0; x < g.P.Width; x++ {
			if !g.Level.At(x, y).isLand() {
				continue
			}
			nm := normalizeBlob(mask8(g.Level, x, y, isLand))
			if nm == 255 {
				continue // внутренняя клетка суши — берег не нужен
			}
			if id, found := resolveFromSet(tr.Set, nm, 8); found {
				g.addSparse("coast", tr.Sheet, x, y, id, nil)
			}
		}
	}
}

// stageCliff — шаг 11: грани обрыва. Плотный слой plateau уже несёт краевые
// тайлы Ground_grass (blob), которые и читаются как обрыв; отдельный слой пока
// не нужен.
func (g *Generator) stageCliff() {}

// stageShadow — шаг 12: тень плато падает на нижнюю землю у южной грани обрыва.
// Ставится на не-плато клетки суши, над которыми (в пределах 2 тайлов севернее)
// есть плато; тайл выбирается по маске формы плато.
func (g *Generator) stageShadow() {
	sh, ok := g.Manifest.Terrains["shadow"]
	if !ok {
		return
	}
	isPlateau := func(l Level) bool { return l == Plateau }
	for y := 0; y < g.P.Height; y++ {
		for x := 0; x < g.P.Width; x++ {
			lv := g.Level.At(x, y)
			if lv != Ground {
				continue
			}
			// плато сверху в пределах 2 тайлов?
			above := (g.Level.In(x, y-1) && g.Level.At(x, y-1) == Plateau) ||
				(g.Level.In(x, y-2) && g.Level.At(x, y-2) == Plateau)
			if !above {
				continue
			}
			id := sh.Fill
			if sh.Blob != nil {
				if v, found := resolveFromSet(sh.Blob, normalizeBlob(mask8(g.Level, x, y, isPlateau)), 8); found {
					id = v
				}
			}
			g.addSparse("plateau_shadow", sh.Sheet, x, y, id, nil)
		}
	}
}

// stageHangers — шаг 13: лианы на южных гранях обрыва.
func (g *Generator) stageHangers() {}

// stageSpots — шаг 15: напольные пятна травы/земли, разбавляют ровный фон.
// Разрежённый слой, случайно по сиду, только на нижней земле (не на плато/воде).
func (g *Generator) stageSpots() {
	sp := g.Manifest.Spots
	if sp.Sheet == "" || len(sp.Variants) == 0 {
		return
	}
	rng := rand.New(rand.NewSource(int64(g.Seed) ^ 0xB16B00B5))
	for y := 0; y < g.P.Height; y++ {
		for x := 0; x < g.P.Width; x++ {
			if g.Level.At(x, y) != Ground {
				continue
			}
			if rng.Float64() >= 0.06 {
				continue
			}
			id := sp.Variants[rng.Intn(len(sp.Variants))]
			g.addSparse("ground_spots", sp.Sheet, x, y, id, nil)
		}
	}
}
