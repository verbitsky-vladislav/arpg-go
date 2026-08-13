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

// stagePlateauShape — морфологическая чистка силуэта возвышенностей.
// Порог по высоте даёт рваные ленты в 1-2 клетки: на них не встаёт ни обрыв,
// ни лестница, а автотайл на такой ширине вырождается в пунктир. Открытие
// (эрозия→дилатация) убирает ленты и усы, закрытие (дилатация→эрозия) заливает
// дыры-проколы. Итог — компактные «столовые горы» с гладким контуром.
func (g *Generator) stagePlateauShape() {
	W, H := g.P.Width, g.P.Height
	set := make([]bool, W*H)
	for i, lv := range g.Level.Data {
		set[i] = lv == Plateau
	}
	// r=1 открытие убирает ленты шириной 1-2, закрытие радиусом 2 заливает дыры
	set = morph(set, W, H, false, 1) // эрозия
	set = morph(set, W, H, true, 2)  // дилатация (открытие + начало закрытия)
	set = morph(set, W, H, false, 1) // эрозия (конец закрытия)
	for i := range g.Level.Data {
		switch {
		case set[i] && g.Level.Data[i] == Ground:
			g.Level.Data[i] = Plateau
		case !set[i] && g.Level.Data[i] == Plateau:
			g.Level.Data[i] = Ground
		}
	}
}

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
				for d := 1; d <= cliffHeight; d++ {
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
		}
	}
	// юбка встала — фиксируем её клетки
	g.Cliff = map[[2]int]bool{}
	for y := 0; y < g.P.Height; y++ {
		for x := 0; x < g.P.Width; x++ {
			if !isP(x, y) || isP(x, y+1) {
				continue
			}
			for d := 1; d <= cliffHeight; d++ {
				g.Cliff[[2]int{x, y + d}] = true
			}
		}
	}
}

// stagePlateauFix — шаг 7: срезать плато уже 4 тайлов (на нём не помещается
// лестница). Клетка плато срезается, если у неё нет плато-соседа на расстоянии
// ≥4 в какой-либо из осей (грубая проверка «толщины»).
func (g *Generator) stagePlateauFix() {
	isP := func(x, y int) bool { return g.Level.In(x, y) && g.Level.At(x, y) == Plateau }
	// срез узких клеток порождает новые узкие — повторяем до стабилизации
	for pass := 0; pass < 8; pass++ {
		toCut := make([]int, 0)
		for y := 0; y < g.P.Height; y++ {
			for x := 0; x < g.P.Width; x++ {
				if g.Level.At(x, y) == Plateau && !thickEnough(isP, x, y, 4) {
					toCut = append(toCut, y*g.P.Width+x)
				}
			}
		}
		if len(toCut) == 0 {
			return
		}
		for _, i := range toCut {
			g.Level.Data[i] = Ground
		}
	}
}

// thickEnough — есть ли непрерывная полоса плато шириной w и по горизонтали,
// и по вертикали, проходящая через (x,y).
func thickEnough(isP func(x, y int) bool, x, y, w int) bool {
	run := func(dx, dy int) int {
		c := 1
		for k := 1; k < w; k++ {
			if isP(x+dx*k, y+dy*k) {
				c++
			} else {
				break
			}
		}
		for k := 1; k < w; k++ {
			if isP(x-dx*k, y-dy*k) {
				c++
			} else {
				break
			}
		}
		return c
	}
	return run(1, 0) >= w && run(0, 1) >= w
}

// dropPlateau срезает все клетки плато до нижней земли. Временная мера Фазы 1:
// плато с обрывом ещё не собрано (SUMMARY §1.7), а его краевые тайлы в blob-схеме
// давали визуальный мусор. Фаза 4 заменит это настоящей геометрией обрыва.
func (g *Generator) dropPlateau() {
	for i, lv := range g.Level.Data {
		if lv == Plateau {
			g.Level.Data[i] = Ground
		}
	}
}

// stageStairs — шаг 8: лестницы (узкое место связности, §6). Реализация в M3.
func (g *Generator) stageStairs() {}

// stageConnectivity — шаг 9: достижимость плато. Реализация в M3.
func (g *Generator) stageConnectivity() {}
