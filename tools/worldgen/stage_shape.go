package main

// stage_shape.go — шаги 5-9 (worldgen.spec §5): сглаживание формы, острова,
// срез узких плато, лестницы, связность. Работают над сеткой уровней.

// stageSmooth — шаг 5: клеточный автомат, 3 итерации, правило большинства
// 5 из 8. Убирает одиночные клетки и рваные края уровней.
func (g *Generator) stageSmooth() {
	for it := 0; it < 3; it++ {
		next := NewGrid[Level](g.P.Width, g.P.Height)
		copy(next.Data, g.Level.Data)
		for y := 0; y < g.P.Height; y++ {
			for x := 0; x < g.P.Width; x++ {
				cur := g.Level.At(x, y)
				// голосование по «земельности» и «плато»
				land, plat, liq := 0, 0, 0
				for _, d := range nb8 {
					n := g.Level.AtOr(x+d[0], y+d[1], cur)
					switch {
					case n == Plateau:
						plat++
						land++
					case n.isLand():
						land++
					default:
						liq++
					}
				}
				switch {
				case cur.isLiquid() && land >= 5:
					next.Set(x, y, Ground)
				case cur.isLand() && liq >= 5:
					next.Set(x, y, LiquidShallow)
				case cur == Ground && plat >= 5:
					next.Set(x, y, Plateau)
				case cur == Plateau && plat <= 2:
					next.Set(x, y, Ground)
				}
			}
		}
		g.Level = next
	}
}

// stageIslands — шаг 6: заливка мелких кусков. Суша < islandMin затапливается,
// плато < plateauMin срезается до нижней земли.
const (
	islandMin  = 80
	plateauMin = 40
)

func (g *Generator) stageIslands() {
	// куски суши
	labels, n := components(g.Level, func(l Level) bool { return l.isLand() })
	sizes := componentSizes(labels, n)
	for i := range g.Level.Data {
		if id := labels.Data[i]; id > 0 && sizes[id] < islandMin {
			g.Level.Data[i] = LiquidShallow
		}
	}
	// куски плато
	pl, pn := components(g.Level, func(l Level) bool { return l == Plateau })
	psz := componentSizes(pl, pn)
	for i := range g.Level.Data {
		if id := pl.Data[i]; id > 0 && psz[id] < plateauMin {
			g.Level.Data[i] = Ground
		}
	}
}

// Морфологическая чистка силуэта, бюджет доли плато и врезка ступеней снизу
// удалены: форма, размер и количество возвышенностей теперь задаются явным
// размещением кусков (plateau_kind.go), а не подтачиванием изолинии шума.

// morph — одна морфологическая операция радиуса r по 8-связности.
// grow=true — дилатация (клетка включается, если рядом есть включённая),
// grow=false — эрозия (клетка выживает, только если все соседи включены).
// Вне массива считаем «выключено», поэтому плато не липнет к рамке.
func morph(src []bool, W, H int, grow bool, r int) []bool {
	cur := src
	for it := 0; it < r; it++ {
		next := make([]bool, W*H)
		for y := 0; y < H; y++ {
			for x := 0; x < W; x++ {
				v := cur[y*W+x]
				for _, d := range nb8 {
					nx, ny := x+d[0], y+d[1]
					n := nx >= 0 && ny >= 0 && nx < W && ny < H && cur[ny*W+nx]
					if grow {
						v = v || n
					} else {
						v = v && n
					}
				}
				next[y*W+x] = v
			}
		}
		cur = next
	}
	return cur
}

// stagePlateauApron — резервирует клетки тела обрыва под южной кромкой плато.
// Обрыв рисуется вниз на cliffHeight клеток и должен лечь на ЧИСТУЮ нижнюю
// землю: если под кромкой вода, край массива или другое плато, ставить скалу
// некуда — такую клетку плато срезаем и повторяем, пока юбка не встанет везде.
// Без этого шага обрыв упирался в воду и линия рвалась (те самые «кривые тайлы»
// на концах). Юбка кладётся в g.Cliff: она непроходима (nav) и её не трогают
// вода и тропы.
func (g *Generator) stagePlateauApron() {
	isP := func(x, y int) bool { return g.Level.In(x, y) && g.Level.At(x, y) == Plateau }
	for pass := 0; pass < 12; pass++ {
		var cut [][2]int
		for y := 0; y < g.P.Height; y++ {
			for x := 0; x < g.P.Width; x++ {
				if !isP(x, y) || isP(x, y+1) {
					continue // не южная кромка
				}
				// глубина своя у каждого типа возвышенности
				for d := 1; d <= g.cliffDepthAt(x, y); d++ {
					if !g.Level.In(x, y+d) || g.Level.At(x, y+d) != Ground {
						cut = append(cut, [2]int{x, y})
						break
					}
				}
			}
		}
		if len(cut) == 0 {
			break
		}
		for _, c := range cut {
			g.Level.Set(c[0], c[1], Ground)
			if g.Kind != nil {
				g.Kind.Set(c[0], c[1], uint8(kindNone))
			}
		}
	}
	// юбка встала — фиксируем её клетки
	g.Cliff = map[[2]int]bool{}
	for y := 0; y < g.P.Height; y++ {
		for x := 0; x < g.P.Width; x++ {
			if !isP(x, y) || isP(x, y+1) {
				continue
			}
			for d := 1; d <= g.cliffDepthAt(x, y); d++ {
				g.Cliff[[2]int{x, y + d}] = true
			}
		}
	}
}

// stagePlateauFix — шаг 7: убрать с макушки ленты и усы шириной 1-2 клетки.
// На них не встаёт ни обрыв, ни лестница, а автотайл вырождается в пунктир.
// Критерий — морфологическое открытие: клетка выживает, если попадает в ядро
// (эрозия на 1) или примыкает к нему. Прямоугольности он НЕ требует, поэтому
// силуэт остаётся округлым и неровным.
func (g *Generator) stagePlateauFix() {
	W, H := g.P.Width, g.P.Height
	// срез узких клеток порождает новые узкие — повторяем до стабилизации
	for pass := 0; pass < 8; pass++ {
		set := make([]bool, W*H)
		for i, lv := range g.Level.Data {
			set[i] = lv == Plateau
		}
		keep := morph(morph(set, W, H, false, 1), W, H, true, 1)
		cut := 0
		for i := range g.Level.Data {
			if set[i] && !keep[i] {
				g.Level.Data[i] = Ground
				if g.Kind != nil {
					g.Kind.Data[i] = uint8(kindNone)
				}
				cut++
			}
		}
		if cut == 0 {
			return
		}
	}
}

// plateauThin — сколько клеток плато не переживают открытие на 1, то есть
// сидят на ленте или усе шириной меньше трёх (проверка E4).
func plateauThin(lv *Grid[Level]) int {
	set := make([]bool, len(lv.Data))
	for i, v := range lv.Data {
		set[i] = v == Plateau
	}
	keep := morph(morph(set, lv.W, lv.H, false, 1), lv.W, lv.H, true, 1)
	thin := 0
	for i := range set {
		if set[i] && !keep[i] {
			thin++
		}
	}
	return thin
}

// stageStairs — шаг 8: лестницы (узкое место связности, §6) — в stage_stairs.go.

// stageConnectivity — шаг 9: достижимость плато. Реализация в M3.
func (g *Generator) stageConnectivity() {}
