package worldgen

// stage_props_place.go — расстановка объектов ПО ГРУППАМ (см. props_group.go).
//
// Каждая группа сеется своим проходом со своей плотностью: лес выходит густым,
// валуны редкими, руины — событием. Один общий Пуассон этого не давал: он
// разводил всё под одну гребёнку и, отталкиваясь от габарита спрайта, не давал
// кронам смыкаться вовсе.
//
// Место занимается ТЕЛОМ, а не картинкой. Спрайт дерева 128 px, ствол у земли
// 22-28: если резервировать под кроны, лес получается парковым, с равными
// промежутками. По телу кроны смыкаются и перекрывают друг друга — как в лесу и
// бывает, а порядок отрисовки разводит их по sort_y.

import (
	"math/rand"
	"sort"
)

// propSpawnClear — радиус вокруг точки появления, свободный от объектов.
const propSpawnClear = 6

// stageProps расставляет объекты. Порядок групп задан propGroups: тяжёлое
// занимает место первым, мелочь втискивается в остатки.
func (g *Generator) stageProps() {
	if len(g.Manifest.Props) == 0 {
		return
	}
	W, H := g.P.Width, g.P.Height
	occ := make([]bool, W*H) // клетки, занятые телами уже поставленных объектов
	if g.hasSpawn {
		markDisc(occ, W, H, g.spawn[0], g.spawn[1], propSpawnClear)
	}
	// объекты по группам, в порядке манифеста
	byGroup := map[string][]int{}
	for i, p := range g.Manifest.Props {
		byGroup[p.Group] = append(byGroup[p.Group], i)
	}
	for gi, grp := range propGroups {
		idx := byGroup[grp.Name]
		if len(idx) == 0 {
			continue
		}
		rng := rand.New(rand.NewSource(int64(g.Seed) ^ int64(0x5DEECE66D+gi*7919)))
		g.seedPropGroup(rng, occ, grp, idx)
	}
	g.repairPropWalls()
}

// seedPropGroup засевает группу — каждую зону влажности до своей доли. Зоны
// считаются порознь, иначе густой лес во влажной части «съедает» всю долю, и
// в сухой не остаётся ни дерева.
func (g *Generator) seedPropGroup(rng *rand.Rand, occ []bool, grp PropGroup, idx []int) {
	W, H := g.P.Width, g.P.Height
	byZone := map[string][][2]int{}
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			if !g.propCellOK(grp, x, y) {
				continue
			}
			z := g.zoneAt(x, y)
			if _, ok := grp.Cover[z]; !ok {
				continue
			}
			byZone[z] = append(byZone[z], [2]int{x, y})
		}
	}
	for _, z := range grp.zones() {
		cells := byZone[z]
		if len(cells) == 0 {
			continue
		}
		rng.Shuffle(len(cells), func(i, j int) { cells[i], cells[j] = cells[j], cells[i] })
		target := int(float64(len(cells)) * grp.Cover[z] * propDensity)
		placed := 0
		for _, c := range cells {
			if placed >= target {
				break
			}
			pi := g.pickPropOfGroup(rng, idx, g.Level.At(c[0], c[1]))
			if pi < 0 {
				continue
			}
			if n := g.placePropBody(occ, g.Manifest.Props[pi], c[0], c[1]); n > 0 {
				placed += n
			}
		}
	}
}

// propCellOK — годится ли клетка под якорь объекта этой группы.
func (g *Generator) propCellOK(grp PropGroup, x, y int) bool {
	if !g.Level.In(x, y) {
		return false
	}
	lv := g.Level.At(x, y)
	if !lv.isLand() || g.Cliff[[2]int{x, y}] || g.Stair[[2]int{x, y}] {
		return false
	}
	if g.Trail[[2]int{x, y}] && !grp.OnTrail {
		return false
	}
	if g.Bridge[[2]int{x, y}] {
		return false
	}
	// Подход к лестнице обязан остаться свободным: сама ступень исключена выше,
	// но объект, севший у её подножия, запирает единственный путь на плато —
	// на этом падал тест связности этажей.
	if grp.Collides {
		for dy := -propStairClear; dy <= propStairClear; dy++ {
			for dx := -propStairClear; dx <= propStairClear; dx++ {
				if g.Stair[[2]int{x + dx, y + dy}] {
					return false
				}
			}
		}
	}
	// роль поверхности: нижняя земля или макушка
	want := "ground_a"
	if lv == Plateau {
		want = "plateau"
	}
	ok := false
	for _, o := range grp.On {
		if o == want {
			ok = true
		}
	}
	if !ok {
		return false
	}
	// Кромка уровня. У края макушки стоит обрыв, у края нижней земли — его
	// подножие; объект, севший вплотную, лезет на переходный тайл и на физику,
	// которой там и так тесно.
	for dy := -grp.EdgeClear; dy <= grp.EdgeClear; dy++ {
		for dx := -grp.EdgeClear; dx <= grp.EdgeClear; dx++ {
			cx, cy := x+dx, y+dy
			if !g.Level.In(cx, cy) {
				return false // край карты
			}
			if g.Level.At(cx, cy) != lv || g.Cliff[[2]int{cx, cy}] {
				return false
			}
		}
	}
	return true
}

// pickPropOfGroup выбирает объект группы по весу.
func (g *Generator) pickPropOfGroup(rng *rand.Rand, idx []int, lv Level) int {
	total := 0
	for _, i := range idx {
		if g.propFitsLevel(g.Manifest.Props[i], lv) {
			total += g.Manifest.Props[i].Weight
		}
	}
	if total == 0 {
		return -1
	}
	r := rng.Intn(total)
	for _, i := range idx {
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

// placePropBody ставит объект якорем в (cx,cy), занимая клетки ТЕЛОМ. Возвращает
// число занятых клеток (0 — не влез).
//
// Под телом обязана быть суша, а вот спрайт волен свисать куда угодно: дерево у
// воды роняет крону на воду, и это правильно — так оно и нарисовано.
func (g *Generator) placePropBody(occ []bool, p Prop, cx, cy int) int {
	// Тропа должна остаться чистой НА ВИД, а не только под якорем: крона в 8
	// тайлов накрывает дорожку, даже когда ствол стоит рядом. Поэтому проверяется
	// весь прямоугольник спрайта — он же и виден. Квадрата вокруг якоря мало:
	// спрайт уходит вверх на всю свою высоту, а вниз не идёт вовсе.
	grp, hasGrp := propGroupByName(p.Group)
	if hasGrp && !grp.OnTrail {
		if g.propSpriteHits(p, cx, cy, g.Trail) {
			return 0
		}
	}
	// Лестница — единственная связь этажей, и прятать её нельзя: крона, накрывшая
	// ступени, оставляет игрока без видимого пути наверх, даже когда ствол стоит
	// в стороне. Поэтому у непроходимого проверяется и спрайт, а не только тело.
	if hasGrp && grp.Collides && g.propSpriteHits(p, cx, cy, g.Stair) {
		return 0
	}
	r := propBodyRadius(p)
	W, H := g.P.Width, g.P.Height
	var cells [][2]int
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			if x < 0 || y < 0 || x >= W || y >= H {
				return 0
			}
			if dx, dy := x-cx, y-cy; dx*dx+dy*dy > r*r {
				continue
			}
			if occ[y*W+x] || !g.Level.At(x, y).isLand() || g.Cliff[[2]int{x, y}] {
				return 0
			}
			cells = append(cells, [2]int{x, y})
		}
	}
	for _, c := range cells {
		occ[c[1]*W+c[0]] = true
	}
	w, h := p.Footprint[0], p.Footprint[1]
	file := p.File
	if p.SwapOnGroundB != "" && g.groundBAt(cx, cy) {
		file = p.SwapOnGroundB
	}
	inst := PropInst{
		ID: p.ID, File: file, X: cx - w/2, Y: cy - h + 1, W: w, H: h,
		Anchor: [2]int{w / 2, h}, SortY: cy, Collides: p.Collides, Body: p.Body,
	}
	if p.Anim != nil && p.Anim.Frames > 1 {
		inst.Frames = p.Anim.Frames
		inst.MS = p.Anim.MS
	}
	g.Props = append(g.Props, inst)
	return len(cells)
}

// propSpriteHits — накрывает ли спрайт пропса, поставленного якорем в (cx,cy),
// хоть одну клетку множества. Спрайт, а не тело: речь про то, что видно.
func (g *Generator) propSpriteHits(p Prop, cx, cy int, set map[[2]int]bool) bool {
	x0, y0 := cx-p.Footprint[0]/2, cy-p.Footprint[1]+1
	for y := y0; y < y0+p.Footprint[1]; y++ {
		for x := x0; x < x0+p.Footprint[0]; x++ {
			if set[[2]int{x, y}] {
				return true
			}
		}
	}
	return false
}

// propBodyRadius — радиус, который объект резервирует под себя, в клетках.
//
// Это НЕ его тело: у непроходимого к телу прибавляется зазор на проход. Барьер
// в физике теперь ровно по замеру спрайта, и если разводить объекты только по
// телу, соседние барьеры смыкаются — густой лес превращается в сплошную стену,
// и карта распадается на недостижимые куски (замер: 27 карт из 40).
//
// Ноль только у совсем мелкого (галька, пучок травы): такое лежит хоть вплотную
// к дереву. Всему остальному минимум клетка, иначе грибы и камни слипаются в
// кучи по пять штук — спрайт-то 32 px, даже когда тело 19.
func propBodyRadius(p Prop) int {
	if p.Body <= propDustBody {
		return 0
	}
	half := p.Body / 2
	if p.Collides {
		half += propPassGap
	}
	if r := (half + 15) / 16; r > 1 {
		return r
	}
	return 1
}

// sortProps упорядочивает объекты по глубине — так их и рисуют.
func sortProps(props []PropInst) {
	sort.SliceStable(props, func(i, j int) bool {
		if props[i].SortY != props[j].SortY {
			return props[i].SortY < props[j].SortY
		}
		return props[i].X < props[j].X
	})
}

// sortedProps — объекты в порядке отрисовки (по глубине).
func (g *Generator) sortedProps() []PropInst {
	out := make([]PropInst, len(g.Props))
	copy(out, g.Props)
	sortProps(out)
	return out
}

// auditProps — проверка правил размещения по НАСТОЯЩИМ множествам
// генератора (снаружи, из map.json, тропу и кромку точно не увидеть).
func (g *Generator) auditProps() (water, trail, edge, stair int) {
	for _, in := range g.Props {
		cx, cy := in.X+in.Anchor[0], in.Y+in.Anchor[1]-1
		if !g.Level.In(cx, cy) || !g.Level.At(cx, cy).isLand() {
			water++
		}
		var grp PropGroup
		for _, p := range g.Manifest.Props {
			if p.ID == in.ID {
				grp, _ = propGroupByName(p.Group)
				break
			}
		}
		// тропа считается по СПРАЙТУ: игрок видит крону, а не якорь
		if !grp.OnTrail {
			over := false
			for y := in.Y; y < in.Y+in.H && !over; y++ {
				for x := in.X; x < in.X+in.W; x++ {
					if g.Trail[[2]int{x, y}] {
						over = true
						break
					}
				}
			}
			if over {
				trail++
			}
		}
		// у непроходимого ни спрайт, ни подход не должны задевать лестницу
		if grp.Collides {
			var pr Prop
			for _, p := range g.Manifest.Props {
				if p.ID == in.ID {
					pr = p
					break
				}
			}
			near := false
			for dy := -propStairClear; dy <= propStairClear && !near; dy++ {
				for dx := -propStairClear; dx <= propStairClear; dx++ {
					if g.Stair[[2]int{cx + dx, cy + dy}] {
						near = true
						break
					}
				}
			}
			if near || (pr.ID != "" && g.propSpriteHits(pr, cx, cy, g.Stair)) {
				stair++
			}
		}
		if grp.EdgeClear > 0 {
			lv := g.Level.At(cx, cy)
			for dy := -grp.EdgeClear; dy <= grp.EdgeClear && edge == 0; dy++ {
				for dx := -grp.EdgeClear; dx <= grp.EdgeClear; dx++ {
					if !g.Level.In(cx+dx, cy+dy) || g.Level.At(cx+dx, cy+dy) != lv ||
						g.Cliff[[2]int{cx + dx, cy + dy}] {
						edge++
						break
					}
				}
			}
		}
	}
	return
}

// repairPropWalls вырубает просеку там, где сомкнувшиеся барьеры заперли кусок
// карты.
//
// Барьер объекта теперь размером с его спрайт, и густой лес физически может
// стать стеной: замер по сорока сидам — без зазора между объектами недостижимая
// область появлялась на 27 картах. Зазор снимает почти всё, но не всё: где-то
// деревья всё равно смыкаются с обрывом или берегом. Там лес и прореживается —
// точечно, вместо того чтобы разредить его на всей карте.
func (g *Generator) repairPropWalls() {
	for pass := 0; pass < propRepairPasses; pass++ {
		bg := g.bodyComponents(spawnRadius)
		big := bg.significant(bridgeMinShare)
		if len(big) < 2 {
			return
		}
		main := bg.biggest()
		// клетки отрезанных областей
		cut := map[[2]int]bool{}
		for y := 0; y < g.P.Height; y++ {
			for x := 0; x < g.P.Width; x++ {
				if v := bg.labels.At(x, y); v != 0 && v != main && big[v] {
					cut[[2]int{x, y}] = true
				}
			}
		}
		if len(cut) == 0 {
			return
		}
		// убираем непроходимые объекты по кромке отрезанного куска
		kept := g.Props[:0]
		removed := 0
		for _, in := range g.Props {
			if !in.Collides {
				kept = append(kept, in)
				continue
			}
			cx, cy := in.X+in.Anchor[0], in.Y+in.Anchor[1]-1
			near := false
			for dy := -propRepairReach; dy <= propRepairReach && !near; dy++ {
				for dx := -propRepairReach; dx <= propRepairReach; dx++ {
					if cut[[2]int{cx + dx, cy + dy}] {
						near = true
						break
					}
				}
			}
			if near {
				removed++
				continue
			}
			kept = append(kept, in)
		}
		g.Props = kept
		if removed == 0 {
			return // рубить нечего, кусок отрезан не лесом
		}
	}
}
