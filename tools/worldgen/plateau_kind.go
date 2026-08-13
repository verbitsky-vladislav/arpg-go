package main

// plateau_kind.go — три типа возвышенностей и их размещение на карте.
//
// Раньше плато рождалось порогом по полю высоты: сколько его выйдет и какой
// формы — лотерея сида, а размер приходилось потом подтачивать эрозией. Теперь
// куски РАЗМЕЩАЮТСЯ явно: тип задаёт высоту обрыва, предел макушки и наличие
// подъёма, а поле высоты используется только как подсказка, где возвышенность
// уместна (семена ищутся по высоким местам).
//
// Высота считается в видимых клетках: юбка глубиной d рисуется как d+1 клеток —
// гребень, (d-1) тела и подножие. Отсюда cliff = видимая высота − 1.

import "math"

type plateauKind uint8

const (
	kindNone plateauKind = iota
	kindHigh             // 5 клеток: гребень + 3 тела + подножие
	kindMid              // 3 клетки: гребень + тело + подножие
	kindLow              // 2 клетки: гребень и сразу подножие
)

// plateauSpec — контракт одного типа возвышенности.
type plateauSpec struct {
	kind     plateauKind
	name     string
	cliff    int  // глубина юбки; видимая высота = cliff+1
	capCells int  // предел клеток на макушке
	minCells int  // ниже этого размера кусок не ставим
	min, max int  // сколько кусков этого типа на карте
	stairs   bool // можно ли на него подняться
	lobes    int  // на сколько долей ломается силуэт при росте
}

// plateauSpecs — порядок важен: крупные по площади куски размещаются первыми,
// им труднее найти место. Мелкие высокие потом втискиваются в остатки.
//
// Частота обратна высоте: низких пологих возвышенностей на карте больше всего,
// средних умеренно, высокая скала — редкость и потому заметное место.
//
// У высокого типа своя внешность. Прежний потолок в 22 клетки при обязательной
// толщине не оставлял выбора: любой такой кусок вырождался в квадратик без
// характера. Потолок поднят, а долей ему дано больше, чем остальным — высокая
// скала растёт отрогами и читается как скалистый гребень, а не как плитка.
// Размеры заданы от высокого типа: у него коридор 30..41, у среднего и низкого
// прежние пороги подняты на 30% — пропорция между типами сохранена, вся тройка
// просто стала крупнее.
var plateauSpecs = []plateauSpec{
	{kindLow, "низкое", 1, 169, 91, 4, 5, true, 2},
	{kindMid, "среднее", 2, 91, 40, 2, 3, true, 2},
	{kindHigh, "высокое", 4, 41, 30, 1, 2, false, 3},
}

func specOf(k plateauKind) plateauSpec {
	for _, s := range plateauSpecs {
		if s.kind == k {
			return s
		}
	}
	return plateauSpec{kind: kindNone, cliff: 1, capCells: 1, lobes: 1}
}

// cliffDepthAt — глубина юбки под кромкой плато в клетке (x,y).
func (g *Generator) cliffDepthAt(x, y int) int {
	if g.Kind == nil || !g.Kind.In(x, y) {
		return 1
	}
	return specOf(plateauKind(g.Kind.At(x, y))).cliff
}

// plateauFloorCells — нижняя граница макушки для любого типа: меньше 15 клеток
// не помещается ни устойчивая к чистке форма, ни лестница шириной 2.
const plateauFloorCells = 15

// plateauGap — свободная полоса вокруг куска: обрывы соседей не срастаются и
// тени не наезжают друг на друга. Это лишь минимум; куда сильнее куски разводит
// выбор семени, который штрафует близость к уже поставленным.
const plateauGap = 10

// plateauSpread — на каком расстоянии кусок считается «достаточно далеко» от
// соседей. Ближе — семя штрафуется, но не запрещается: на тесной суше лучше
// поставить кусок рядом, чем не поставить вовсе.
const plateauSpread = 40

// stagePlateauPlace расставляет куски всех типов. Порог по высоте, который
// насыпал stageLevels, здесь сносится: форма возвышенностей задаётся ростом от
// семени, а не изолинией шума.
func (g *Generator) stagePlateauPlace() {
	W, H := g.P.Width, g.P.Height
	g.Kind = NewGrid[uint8](W, H)
	for i := range g.Level.Data {
		if g.Level.Data[i] == Plateau {
			g.Level.Data[i] = Ground
		}
	}
	// blocked — куда семя и рост не заходят: занятые куски вместе с полосой
	// plateauGap вокруг. Вода и край проверяются отдельно, по месту под юбку.
	blocked := make([]bool, W*H)
	var centers [][2]int // центры уже поставленных кусков — от них разводим новые

	g.PlateauCount = map[plateauKind]int{}
	for si, sp := range plateauSpecs {
		want := sp.min
		if sp.max > sp.min {
			want += int(hash2(si*7919, si*104729, g.Seed) * float64(sp.max-sp.min+1))
			if want > sp.max {
				want = sp.max
			}
		}
		for placed := 0; placed < want; {
			// одно семя — не приговор: топ по высоте может целиком лежать в уже
			// занятой зоне или в клине, где кусок не дорастает до минимума.
			// Пробуем разные семена, пока место не найдётся.
			var region []int
			for attempt := 0; attempt < 16 && region == nil; attempt++ {
				region = g.growPlateau(sp, blocked, centers, si*1000+placed*100+attempt)
			}
			if region == nil {
				break // мест под этот тип больше нет
			}
			sx, sy := 0, 0
			for _, i := range region {
				g.Level.Data[i] = Plateau
				g.Kind.Data[i] = uint8(sp.kind)
				sx += i % W
				sy += i / W
			}
			centers = append(centers, [2]int{sx / len(region), sy / len(region)})
			markBlocked(blocked, region, W, H, plateauGap)
			placed++
			g.PlateauCount[sp.kind]++
		}
	}
}

// growPlateau выращивает один кусок от семени до предела макушки и возвращает
// его клетки, либо nil, если подходящего места не нашлось.
func (g *Generator) growPlateau(sp plateauSpec, blocked []bool, centers [][2]int, salt int) []int {
	W, H := g.P.Width, g.P.Height
	// клетка годится под возвышенность, если она нижняя земля, свободна и под
	// ней есть место на всю юбку — иначе обрыв упрётся в воду или в чужой кусок
	fits := func(x, y int) bool {
		if !g.Level.In(x, y) || g.Level.At(x, y) != Ground || blocked[y*W+x] {
			return false
		}
		for d := 1; d <= sp.cliff+1; d++ {
			if !g.Level.In(x, y+d) || g.Level.At(x, y+d) != Ground || blocked[(y+d)*W+x] {
				return false
			}
		}
		// Кускам с подъёмом нужен зазор от воды по всей южной стороне: лестница
		// не ставится, если берег ближе stairsWaterClear, и кусок без этого
		// запаса рискует остаться недостижимым.
		if sp.stairs && g.waterNear(x, y, 1, sp.cliff, stairsWaterClear) {
			return false
		}
		return true
	}
	// семя — по высоте рельефа: возвышенность логично ставить там, где местность
	// и так выше. Кандидаты собираются, сортируются по высоте и один берётся по сиду.
	var cands []seedCand
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			if !fits(x, y) {
				continue
			}
			// у семени должен быть запас во все стороны, иначе кусок не вырастет
			ok := true
			for _, d := range nb8 {
				if !fits(x+d[0], y+d[1]) {
					ok = false
					break
				}
			}
			if !ok {
				continue
			}
			// Ранг кандидата — не только высота рельефа. Второй половиной идёт
			// удалённость от уже поставленных кусков: без неё все семена садятся
			// в один высокий массив и возвышенности кучкуются в углу карты.
			far := 1.0
			for _, c := range centers {
				d := float64(absInt(c[0]-x)+absInt(c[1]-y)) / plateauSpread
				if d < far {
					far = d
				}
			}
			cands = append(cands, seedCand{y*W + x, g.Height.At(x, y)*0.5 + far*0.5})
		}
	}
	if len(cands) == 0 {
		return nil
	}
	// верхняя треть по высоте, выбор внутри неё — по сиду
	top := len(cands)/3 + 1
	partialTopByHeight(cands, top)
	pick := int(hash2(salt*40503, salt*2654435761, g.Seed) * float64(top))
	if pick >= top {
		pick = top - 1
	}
	best := cands[pick]

	// рост: на каждом шаге берём клетку фронта с наибольшим числом уже занятых
	// соседей — так пятно остаётся компактным, а не расползается лентой.
	inRegion := map[int]bool{best.i: true}
	region := []int{best.i}
	frontier := map[int]bool{}
	addFront := func(i int) {
		x, y := i%W, i/W
		for _, d := range nb4 {
			nx, ny := x+d[0], y+d[1]
			if fits(nx, ny) && !inRegion[ny*W+nx] {
				frontier[ny*W+nx] = true
			}
		}
	}
	addFront(best.i)
	// Рост идёт по плотности соседей — пятно получается округлым и неровным по
	// краю. Прямоугольность сюда специально НЕ вводится: ровные таблички читаются
	// как заглушки, а не как рельеф.
	//
	// Поверх плотности работает тяга доли: рост разбит на sp.lobes фаз, у каждой
	// свой случайный курс, и клетки по этому курсу от центра массы ценятся выше.
	// Пятно уходит в одну сторону, потом в другую — получается силуэт с отрогами,
	// а не ровный блин. Чем больше долей, тем изломаннее скала.
	cx, cy := float64(best.i%W), float64(best.i/W)
	for len(region) < sp.capCells && len(frontier) > 0 {
		lobe := len(region) * maxInt(sp.lobes, 1) / maxInt(sp.capCells, 1)
		ang := hash2(salt*7919+lobe*104729, lobe*6151, g.Seed) * 2 * math.Pi
		dx, dy := math.Cos(ang), math.Sin(ang)
		bestI, bestScore := -1, math.Inf(-1)
		for i := range frontier {
			x, y := i%W, i/W
			n := 0
			for _, d := range nb8 {
				if inRegion[(y+d[1])*W+x+d[0]] {
					n++
				}
			}
			// дробная добавка от сида ломает ровные фронты, но не спорит с
			// главным критерием — плотностью соседей
			score := float64(n) + hash2(x*6151, y*24593, g.Seed)*1.4
			// косинус между курсом доли и направлением на клетку
			ox, oy := float64(x)-cx, float64(y)-cy
			if d := math.Hypot(ox, oy); d > 0.5 {
				score += 1.6 * (ox*dx + oy*dy) / d
			}
			if score > bestScore {
				bestI, bestScore = i, score
			}
		}
		if bestI < 0 {
			break
		}
		delete(frontier, bestI)
		inRegion[bestI] = true
		region = append(region, bestI)
		n := float64(len(region))
		cx += (float64(bestI%W) - cx) / n
		cy += (float64(bestI/W) - cy) / n
		addFront(bestI)
	}
	// Чистку формы делаем ЗДЕСЬ, а не только в stagePlateauFix: иначе кусок
	// коммитится, потом стачивается ниже минимума и тип остаётся недобранным,
	// а попытку выбрать другое место мы уже упустили.
	region = openRegion(region, W, g.P.Height)
	if len(region) < maxInt(sp.minCells, plateauFloorCells) {
		return nil
	}
	return region
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// openRegion — морфологическое открытие региона (эрозия, затем дилатация в
// пределах исходного множества). Убирает ленты и усы шириной 1-2 клетки, но
// сохраняет скругления: в отличие от проверки «полоса в 4 клетки по обеим осям»,
// открытие не требует, чтобы силуэт был прямоугольным.
func openRegion(region []int, W, H int) []int {
	in := make([]bool, W*H)
	for _, i := range region {
		in[i] = true
	}
	core := morph(in, W, H, false, 1)
	grown := morph(core, W, H, true, 1)
	out := region[:0]
	for _, i := range region {
		if grown[i] {
			out = append(out, i)
		}
	}
	return out
}

// seedCand — клетка-кандидат под семя возвышенности и её ранг (высота рельефа
// пополам с удалённостью от уже поставленных кусков).
type seedCand struct {
	i int
	h float64
}

// partialTopByHeight поднимает k лучших по рангу кандидатов в начало среза
// (частичная сортировка выбором — k мал, полная сортировка тут не нужна).
func partialTopByHeight(c []seedCand, k int) {
	for a := 0; a < k && a < len(c); a++ {
		mx := a
		for b := a + 1; b < len(c); b++ {
			if c[b].h > c[mx].h {
				mx = b
			}
		}
		c[a], c[mx] = c[mx], c[a]
	}
}

// markBlocked закрывает клетки куска и полосу gap вокруг него.
func markBlocked(blocked []bool, region []int, W, H, gap int) {
	for _, i := range region {
		cx, cy := i%W, i/W
		for dy := -gap; dy <= gap; dy++ {
			for dx := -gap; dx <= gap; dx++ {
				x, y := cx+dx, cy+dy
				if x >= 0 && y >= 0 && x < W && y < H {
					blocked[y*W+x] = true
				}
			}
		}
	}
}
