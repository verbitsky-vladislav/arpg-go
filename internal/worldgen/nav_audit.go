package worldgen

// nav_audit.go — проверка готовой сетки физики (E15): всё ли, куда игрок обязан
// попасть, действительно связано с точкой появления.
//
// Проверка нужна именно после buildNav, а не по g.Stair: до неё лестница может
// существовать, но никуда не вести — например, если пропс сел на её нижнюю
// ступень или подножие упёрлось в воду. Ходит эта проверка по тем же данным и
// по тем же правилам, что и физика в игре, поэтому «в отчёте зелено, а в игре
// не залезть» невозможно.

import "github.com/vladislav/game/internal/physics"

// navReach обходит карту от точки появления по правилам физики и считает, у
// скольких клеток макушек обязательный подъём не работает.
//
// Считаются только куски со stairs=true: высокая скала декоративна по
// спецификации типа (plateau_kind.go), и её недостижимость — не ошибка.
func (g *Generator) navReach(mp *MapV1) (unreachable, total int) {
	n := mp.Nav
	if n.Scale <= 0 || len(n.Cells) != n.Width*n.Height {
		return 0, 0
	}
	at := func(sx, sy int) physics.Cell {
		if sx < 0 || sy < 0 || sx >= n.Width || sy >= n.Height {
			return physics.Deep
		}
		return physics.Cell(n.Cells[sy*n.Width+sx])
	}
	// walk — можно ли пешком по этой клетке. Тело в обход не берётся: узость
	// прохода — забота E4, здесь важна только связность.
	walk := func(c physics.Cell) bool {
		return c != physics.Deep && c != physics.Solid
	}

	start, ok := g.spawnSub(mp)
	if !ok {
		return 0, 0
	}
	seen := make([]bool, len(n.Cells))
	seen[start[1]*n.Width+start[0]] = true
	queue := [][2]int{start}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		from := at(c[0], c[1])
		for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			sx, sy := c[0]+d[0], c[1]+d[1]
			if sx < 0 || sy < 0 || sx >= n.Width || sy >= n.Height || seen[sy*n.Width+sx] {
				continue
			}
			to := at(sx, sy)
			if !walk(to) {
				continue
			}
			// Между этажами — только через лестницу: то же правило, что в physics.
			if from.Floor() != to.Floor() && from != physics.Ramp && to != physics.Ramp {
				continue
			}
			seen[sy*n.Width+sx] = true
			queue = append(queue, [2]int{sx, sy})
		}
	}

	for i, v := range n.Cells {
		if physics.Cell(v) != physics.Plateau {
			continue
		}
		x, y := g.navCellOf(i%n.Width), g.navCellOf(i/n.Width)
		if !g.Kind.In(x, y) || !specOf(plateauKind(g.Kind.At(x, y))).stairs {
			continue // декоративная скала: подъёма на неё и не должно быть
		}
		total++
		if !seen[i] {
			unreachable++
		}
	}
	return unreachable, total
}

// spawnSub — под-клетка точки появления.
func (g *Generator) spawnSub(mp *MapV1) ([2]int, bool) {
	for _, mk := range mp.Markers {
		if mk.Kind != "spawn" {
			continue
		}
		return [2]int{mk.X*navScale + navScale/2, mk.Y*navScale + navScale/2}, true
	}
	return [2]int{}, false
}

// navCellOf — клетка уровней, из которой взята под-клетка s (обратно к сдвигу
// dual-grid, см. buildNav).
func (g *Generator) navCellOf(s int) int {
	if s < navScale/2 {
		return -1
	}
	return (s - navScale/2) / navScale
}
