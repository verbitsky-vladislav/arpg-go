package worldgen

// stage_hangers.go — лианы на южной грани обрыва (слой hangers, лист lianas).
//
// Вариант свеса — многотайловый штамп: вертикальная плеть шириной в тайл или
// гирлянда-дуга шириной в два-четыре. Верх штампа садится на ГРЕБЕНЬ: лиана
// перекинута через кромку, а не приклеена к середине стены. Слой hangers
// рисуется последним, выше скалы и лестниц, поэтому плеть ложится поверх стены.
//
// Скала без единой лианы выглядит забытой, поэтому свесы гарантируются на
// КАЖДОЙ возвышенности: гребень группируется по кускам плато, и на каждом куске
// перебираются все его клетки, пока не встанет хотя бы один свес.

import (
	"math/rand"
	"sort"
)

// hangerChance — доля клеток гребня со свесом сверх обязательного первого.
// Плотность ограничена не только ею: занятые клетки закрыты для соседей, и
// гирлянда шириной в три тайла сама съедает три клетки гребня. Поэтому даже при
// значении близком к единице кромка не превращается в сплошную бахрому —
// свободные места остаются там, где рядом не поместился ни один вариант.
const hangerChance = 0.85

// stageHangers развешивает свесы: минимум один на каждый кусок плато, дальше по
// вероятности.
func (g *Generator) stageHangers() {
	h := g.Manifest.Hangers
	if h.Sheet == "" || len(h.Variants) == 0 {
		return
	}
	inRock := func(x, y int) bool { return g.Cliff[[2]int{x, y}] }
	// Полоса, где видно скалу, считается по ПАРАМ углов тайла, а не по одному:
	// тайл на боковой кромке обрыва показывает скалу лишь половиной, и свес на
	// нём повисает над травой рядом со стеной. Нижняя пара — гребень и тело,
	// верхняя — подножие (скала выше тайла, трава ниже).
	onWall := func(x, y int) bool {
		k := cornerKey(inRock, x, y)
		return (k[0] && k[1]) || (k[2] && k[3])
	}
	onStair := func(x, y int) bool {
		return anyCorner(cornerKey(func(cx, cy int) bool { return g.Stair[[2]int{cx, cy}] }, x, y))
	}
	isCrest := func(x, y int) bool { return onWall(x, y) && !onWall(x, y-1) }

	// Гребень группируется по куску плато, к которому относится: «на каждой
	// скале» — это про кусок, а не про карту. Метку даёт клетка плато над
	// гребнем (у тайла гребня верхние углы лежат уже на макушке).
	labels, _ := components(g.Level, func(l Level) bool { return l == Plateau })
	plateauAt := func(x, y int) int {
		for _, c := range [2][2]int{{x - 1, y - 1}, {x, y - 1}} {
			if labels.In(c[0], c[1]) {
				if v := labels.At(c[0], c[1]); v > 0 {
					return v
				}
			}
		}
		return 0
	}

	byPlateau := map[int][][2]int{}
	for y := 0; y <= g.P.Height; y++ {
		for x := 0; x <= g.P.Width; x++ {
			if !isCrest(x, y) || onStair(x, y) {
				continue
			}
			if p := plateauAt(x, y); p > 0 {
				byPlateau[p] = append(byPlateau[p], [2]int{x, y})
			}
		}
	}
	plats := make([]int, 0, len(byPlateau))
	for p := range byPlateau {
		plats = append(plats, p)
	}
	sort.Ints(plats)

	rng := rand.New(rand.NewSource(int64(g.Seed) ^ 0x11A4A))
	occ := map[[2]int]bool{}
	for _, p := range plats {
		crests := byPlateau[p]
		// порядок обхода перемешивается, иначе обязательный свес каждой скалы
		// всегда садится на её западный конец
		rng.Shuffle(len(crests), func(i, j int) { crests[i], crests[j] = crests[j], crests[i] })
		placed := 0
		for _, c := range crests {
			if placed > 0 && rng.Float64() > hangerChance {
				continue
			}
			if g.placeHanger(rng, h, occ, c[0], c[1], onWall, onStair, isCrest) {
				placed++
			}
		}
	}

	// Второй проход — по стене ниже гребня. Гребень разбирается первым: гирлянда
	// цепляется только за него, и отдавать её места плетям нельзя. Здесь встают
	// одни плети (гирлянду не пропустит проверка гребня), заполняя нижние ряды
	// высокой скалы, которые после первого прохода оставались голыми.
	var below [][2]int
	for y := 0; y <= g.P.Height; y++ {
		for x := 0; x <= g.P.Width; x++ {
			if onWall(x, y) && !isCrest(x, y) && !onStair(x, y) && !occ[[2]int{x, y}] {
				below = append(below, [2]int{x, y})
			}
		}
	}
	rng.Shuffle(len(below), func(i, j int) { below[i], below[j] = below[j], below[i] })
	for _, c := range below {
		if rng.Float64() > hangerChance {
			continue
		}
		g.placeHanger(rng, h, occ, c[0], c[1], onWall, onStair, isCrest)
	}
}

// placeHanger ставит в (x,y) случайный подходящий вариант. Подходящий — тот, у
// которого каждая клетка попадает на стену и не занята, а верхний ряд целиком
// лежит на гребне: иначе гирлянда одним концом висит в воздухе там, где обрыв
// уходит вниз уступом.
func (g *Generator) placeHanger(rng *rand.Rand, h Hangers, occ map[[2]int]bool, x, y int,
	onWall, onStair, isCrest func(x, y int) bool) bool {
	fits := func(s Stamp) bool {
		for _, c := range s.Cells {
			cx, cy := x+c.Dx, y+c.Dy
			if !onWall(cx, cy) || onStair(cx, cy) || occ[[2]int{cx, cy}] {
				return false
			}
			// Гирлянда переброшена ЧЕРЕЗ кромку, её верх обязан лежать на гребне
			// целиком — иначе дуга одним концом висит в воздухе там, где обрыв
			// уходит вниз уступом. Вертикальной плети это не нужно: она с тем же
			// успехом растёт по самой стене, и требование гребня лишь оставляло
			// нижние ряды высокой скалы пустыми.
			if s.W > 1 && c.Dy == 0 && !isCrest(cx, cy) {
				return false
			}
		}
		return true
	}
	// Подходящие собираются целиком, и выбирается случайный: брать первый
	// попавшийся значит всегда вешать самую узкую плеть — гирлянды при таком
	// выборе не появятся никогда.
	var ok []int
	for i, s := range h.Variants {
		if fits(s) {
			ok = append(ok, i)
		}
	}
	if len(ok) == 0 {
		return false
	}
	s := h.Variants[ok[rng.Intn(len(ok))]]
	for _, c := range s.Cells {
		cx, cy := x+c.Dx, y+c.Dy
		occ[[2]int{cx, cy}] = true
		sheet := c.Sheet
		if sheet == "" {
			sheet = h.Sheet
		}
		g.addSparse("hangers", sheet, cx, cy, c.Tile, nil)
	}
	return true
}
