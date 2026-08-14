package worldgen

// stage_props.go — точка появления, зоны плотности и маркеры. Сама расстановка
// объектов живёт в stage_props_place.go: она разошлась по группам и больше в
// один файл со спавном не помещалась.

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

// spawnRadius — радиус тела, под которое ищется точка появления. Совпадает с
// physics.Body в world.spawnBody: герой 7 px, плюс запас, чтобы не появляться
// впритирку к скале.
const spawnRadius = 8

// pickSpawn выбирает точку появления — клетку нижней земли ближе к центру,
// не занятую телом обрыва.
//
// Клетки мало: связность суши считается по клеткам, поэтому перешеек шириной в
// одну клетку (16 px) её обеспечивает, а тело радиуса 8 через него не проходит.
// На таком куске игрок оказывался заперт: замер по сидам 1..40 — на восьми
// картах есть недостижимая область, и на одной из них старт выпадал именно в
// МЕНЬШУЮ часть (593 тайла против 4341). Поэтому точка берётся в самой большой
// компоненте, проходимой ТЕЛОМ, а не в первой подходящей клетке.
func (g *Generator) pickSpawn() (int, int, bool) {
	bg := g.bodyComponents(spawnRadius)
	main := bg.biggest()
	cx, cy := g.P.Width/2, g.P.Height/2
	best, bx, by := 1<<30, -1, -1
	for y := 1; y < g.P.Height-1; y++ {
		for x := 1; x < g.P.Width-1; x++ {
			if g.Level.At(x, y) != Ground || g.Cliff[[2]int{x, y}] {
				continue
			}
			if main != 0 && bg.labels.At(x, y) != main {
				continue
			}
			d := (x-cx)*(x-cx) + (y-cy)*(y-cy)
			if d < best {
				best, bx, by = d, x, y
			}
		}
	}
	if bx < 0 && main != 0 {
		// страховка: если телесная область почему-то пуста, старая логика
		// всё равно должна дать точку — карта без спавна хуже, чем спавн в тесноте
		return g.pickSpawnAnywhere()
	}
	return bx, by, bx >= 0
}

// pickSpawnAnywhere — прежний выбор: ближайшая к центру клетка нижней земли.
func (g *Generator) pickSpawnAnywhere() (int, int, bool) {
	cx, cy := g.P.Width/2, g.P.Height/2
	best, bx, by := 1<<30, -1, -1
	for y := 1; y < g.P.Height-1; y++ {
		for x := 1; x < g.P.Width-1; x++ {
			if g.Level.At(x, y) != Ground || g.Cliff[[2]int{x, y}] {
				continue
			}
			if d := (x-cx)*(x-cx) + (y-cy)*(y-cy); d < best {
				best, bx, by = d, x, y
			}
		}
	}
	return bx, by, bx >= 0
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
