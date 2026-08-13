package main

// stage_stairs.go — лестницы на плато (worldgen.spec §5 шаг 8, §6 связность).
//
// Без лестницы плато — декорация: тело обрыва лежит в g.Cliff и в nav стоит
// стеной, залезть наверх физически нельзя. Поэтому подъём обязателен для типов
// со stairs=true (среднее и низкое), а высокое остаётся декоративным.
//
// Лестница СОБИРАЕТСЯ под высоту конкретного обрыва из конструктора художника:
// start (примыкание к макушке) + block×N (тело) + end (подножие), всё из ОДНОГО
// материала. У start и end есть ещё полупрозрачный свес, который ложится на
// клетку ниже своего ряда — он и даёт мягкий стык с травой.
//
// Место под лестницу должно быть РОВНЫМ: обе колонки южной кромки на одном
// ряду, под ними полная юбка обрыва, снизу — чистая земля для подхода, а по
// бокам — тоже обрыв (иначе лестница окажется на торце и повиснет в воздухе).
// Площадки ищутся по каждому куску отдельно, чтобы недостижимым не остался ни один.

import "sort"

// stairsPerCells — на сколько клеток макушки приходится одна лестница.
// Крупный кусок получает несколько подъёмов, но не больше stairsMaxPer.
const (
	stairsPerCells = 90
	stairsMaxPer   = 2
	stairsSpacing  = 10 // минимальный разнос лестниц одного куска, в клетках
	// stairsWaterClear — сколько клеток суши обязано отделять лестницу от воды.
	// Спуск, который упирается в берег через клетку, игрок пройти не может.
	stairsWaterClear = 2
)

func (g *Generator) stageStairs() {
	st := g.Manifest.Stairs
	if st.Sheet == "" || st.Width <= 0 || len(st.kits) == 0 {
		return
	}
	mats := st.stairMaterials()
	w := st.Width
	isP := func(x, y int) bool { return g.Level.In(x, y) && g.Level.At(x, y) == Plateau }
	labels, n := components(g.Level, func(l Level) bool { return l == Plateau })
	if n == 0 {
		return
	}
	sizes := componentSizes(labels, n)

	// Кандидаты по кускам плато; куски без подъёма (высокие) пропускаются.
	// Ищем в два захода: сначала площадки с опорой по обоим бокам, и только если
	// у куска таких нет — с ослабленным условием. У округлых макушек нижняя
	// кромка ступенчатая, и строгого места на ней может не найтись вовсе, а
	// оставить кусок без подъёма нельзя.
	spots := make(map[int][][2]int, n)
	relaxed := make(map[int][][2]int, n)
	for y := 0; y < g.P.Height; y++ {
		for x := 0; x+w <= g.P.Width; x++ {
			if !specOf(plateauKind(g.Kind.At(x, y))).stairs || !g.fitsStairs(isP, x, y, w) {
				continue
			}
			id := labels.At(x, y)
			if g.stairsHaveSides(x, y, w) {
				spots[id] = append(spots[id], [2]int{x, y})
			} else {
				relaxed[id] = append(relaxed[id], [2]int{x, y})
			}
		}
	}

	for id := 1; id <= n; id++ {
		cand := spots[id]
		if len(cand) == 0 {
			cand = relaxed[id]
		}
		if len(cand) == 0 {
			continue // площадки нет — кусок останется без подъёма, ловится в E3
		}
		want := sizes[id]/stairsPerCells + 1
		if want > stairsMaxPer {
			want = stairsMaxPer
		}
		// порядок обхода детерминированный: перебираем кандидатов не подряд, а
		// с шагом от хеша сида, иначе все лестницы липнут к левому краю куска.
		sort.Slice(cand, func(a, b int) bool {
			if cand[a][1] != cand[b][1] {
				return cand[a][1] < cand[b][1]
			}
			return cand[a][0] < cand[b][0]
		})
		step := 1 + int(hash2(id*40503, id*2654435761, g.Seed)*float64(len(cand)))
		// материал един на весь кусок: одна возвышенность — одна кладка
		rows := st.kits[mats[int(hash2(id*6151, id*24593, g.Seed)*float64(len(mats)))%len(mats)]]
		var placed [][2]int
		for k := 0; k < len(cand) && len(placed) < want; k++ {
			c := cand[(k*step)%len(cand)]
			ok := true
			for _, p := range placed {
				if absInt(p[0]-c[0]) < stairsSpacing+w && absInt(p[1]-c[1]) < stairsSpacing {
					ok = false
					break
				}
			}
			if ok {
				placed = append(placed, c)
			}
		}
		for _, c := range placed {
			g.putStairs(st, rows, w, c[0], c[1])
		}
	}
}

// fitsStairs — влезает ли лестница левым верхним углом в (x,y), где y — ряд
// южной кромки плато, а тело лестницы занимает ряды y+1..y+depth+1.
func (g *Generator) fitsStairs(isP func(x, y int) bool, x, y, w int) bool {
	depth := g.cliffDepthAt(x, y)
	for dx := 0; dx < w; dx++ {
		// ровная южная кромка одного типа: плато есть, под ним плато уже нет
		if !isP(x+dx, y) || isP(x+dx, y+1) || g.cliffDepthAt(x+dx, y) != depth {
			return false
		}
		// юбка обрыва под кромкой стоит на всю высоту
		for dy := 1; dy <= depth; dy++ {
			if !g.Cliff[[2]int{x + dx, y + dy}] {
				return false
			}
		}
		// подход снизу: клетка под подножием — чистая проходимая земля
		if !g.Level.In(x+dx, y+depth+1) || g.Level.At(x+dx, y+depth+1) != Ground ||
			g.Cliff[[2]int{x + dx, y + depth + 1}] {
			return false
		}
	}
	// у подножия должно быть где стоять: вода вплотную или через клетку от
	// лестницы означает, что спуск упирается в берег
	return !g.waterNear(x, y, w, depth, stairsWaterClear)
}

// waterNear — есть ли жидкость в полосе clear клеток вокруг всей лестницы,
// считая её footprint от кромки до подножия.
func (g *Generator) waterNear(x, y, w, depth, clear int) bool {
	for cy := y - clear; cy <= y+depth+1+clear; cy++ {
		for cx := x - clear; cx < x+w+clear; cx++ {
			if g.Level.In(cx, cy) && g.Level.At(cx, cy).isLiquid() {
				return true
			}
		}
	}
	return false
}

// stairsHaveSides — есть ли у площадки опора по обоим бокам, то есть лестница
// не садится на торец обрыва, где она повисла бы в воздухе.
func (g *Generator) stairsHaveSides(x, y, w int) bool {
	return g.Cliff[[2]int{x - 1, y + 1}] && g.Cliff[[2]int{x + w, y + 1}]
}

// putStairs собирает лестницу под высоту обрыва в (x,y) и открывает её для nav.
// Первый ряд набора примыкает к макушке, последний — подножие, между ними тело;
// при высоте 2 (низкое плато) тела нет вовсе, верх сразу переходит в низ, а при
// высоте больше набора средние ряды повторяются по кругу.
func (g *Generator) putStairs(st Stairs, rows [][]int, w, x, y int) {
	h := g.cliffDepthAt(x, y) + 1 // видимая высота обрыва в клетках
	top := y + 1
	put := func(ids []int, cy int) {
		for dx := 0; dx < w && dx < len(ids); dx++ {
			if !g.Level.In(x+dx, cy) {
				continue
			}
			g.addSparse("stairs", st.Sheet, x+dx, cy, ids[dx], nil)
			g.Stair[[2]int{x + dx, cy}] = true
		}
	}
	put(rows[0], top)
	body := rows[1 : len(rows)-1]
	for i := 1; i < h-1; i++ {
		if len(body) == 0 {
			put(rows[0], top+i)
			continue
		}
		put(body[(i-1)%len(body)], top+i)
	}
	if h > 1 {
		put(rows[len(rows)-1], top+h-1)
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
