package worldgen

// stage_bridge.go — броды через воду (слой bridges, набор bridge_* листа
// Water_coasts).
//
// Река режется в stageWater уже ПОСЛЕ проверки связности суши, поэтому она
// спокойно разваливает остров на куски. Без переправы половина карты
// недостижима. Замер по сидам 1..40: броды ставятся на 13 из них, разорванной
// суши не остаётся ни на одном.
//
// Брод — это камни в воде, а не мост и не насыпь: вода под ним остаётся водой,
// и проходимость даёт только множество g.Bridge (как у лестниц, врезанных в
// обрыв). Набор художника вертикальный, поэтому брод пересекает воду с севера
// на юг.
//
// Стадия идёт ДО автотайлинга: чтобы жёсткий блок шириной 2 куда-нибудь влез,
// переправа подсыпает берег (см. bank ниже), а такую правку суши обязан увидеть
// автотайл.

import "sort"

const (
	// bridgeMaxSpan — самая широкая вода, которую брод берёт (клеток). Дальше
	// это уже не переправа по камням, а переправа вплавь.
	bridgeMaxSpan = 20
	// bridgeBodyRadius — под какое тело меряется связность. Совпадает с
	// spawnRadius: и старт, и переправы обязаны исходить из одного «проходимо».
	bridgeBodyRadius = spawnRadius
	// bridgeMinShare — доля проходимой площади, ниже которой область считается
	// пятачком и переправы не заслуживает.
	bridgeMinShare = 0.01
	// bridgeLabelReach — на сколько клеток от берега искать метку области. У
	// самой кромки тело не помещается, и проба в упор даёт 0.
	bridgeLabelReach = 3
	// bridgeMaxPasses — сколько переправ подряд стадия готова поставить. Каждая
	// стоит перемера связности, а больше трёх разрывов на карте не встречалось.
	bridgeMaxPasses = 4
	// bridgeTryCandidates — сколько мест пробовать постановкой за один проход.
	// Каждая проба стоит перемера, поэтому список коротко обрезан: годное место
	// почти всегда среди самых коротких.
	bridgeTryCandidates = 8
)

// stageBridges связывает карту переправами, пока значимые области не сольются.
//
// Ставит по ОДНОЙ переправе за проход и каждый раз меряет связность заново.
// Раньше стадия набирала все места сразу и считала, что раз концы лежат в разных
// областях, то переправа их и соединит. Это неправда: конец упирается в узкую
// косу, по которой тело уже не идёт, — брод построен, а пройти нельзя, и место
// при этом считалось занятым, вытесняя годный вариант. Перемер после каждой
// переправы такие пустышки отбраковывает сам.
func (g *Generator) stageBridges() {
	b := g.Manifest.Bridges
	if len(b.kit) < 2 || b.Width <= 0 {
		return
	}
	for pass := 0; pass < bridgeMaxPasses; pass++ {
		if !g.placeOneBridge(b) {
			return
		}
	}
}

// placeOneBridge ставит одну переправу и говорит, стоит ли пробовать ещё.
func (g *Generator) placeOneBridge(b Bridges) bool {
	land := func(x, y int) bool { return g.Level.In(x, y) && g.Level.At(x, y).isLand() }
	// Области считаются ПО ПРОХОДИМОСТИ ТЕЛОМ, а не по клеткам: перешеек шириной
	// в одну клетку даёт клеточную связность, но игрок через него не проходит, и
	// клеточный обход такой разрыв просто не видел — переправа не ставилась там,
	// где она нужнее всего.
	bg := g.bodyComponents(bridgeBodyRadius)
	big := bg.significant(bridgeMinShare)
	if len(big) < 2 {
		return false // карта проходима целиком
	}

	// Берег почти никогда не обрывается в воду ровно: у двух соседних колонок
	// кромка стоит на разных рядах, и жёсткий блок шириной 2 не влезает никуда.
	// Замер: при ширине 1 кандидатов десятки, при ширине 2 — ноль. Поэтому
	// переправе разрешено подсыпать берег: если на ряду кромки одна колонка уже
	// земля, а вторая ещё вода, эта одна клетка становится сушей. Правка идёт до
	// автотайлинга, так что подсыпка получает обычную береговую линию.
	// (px,py) — поперечник переправы: для брода сверху вниз это шаг вправо, для
	// поперечного — шаг вниз.
	bank := func(x, y, px, py, w int) ([][2]int, bool) {
		var fill [][2]int
		for i := 0; i < w; i++ {
			cx, cy := x+px*i, y+py*i
			switch {
			case land(cx, cy):
			case g.Level.In(cx, cy) && g.Level.At(cx, cy).isLiquid():
				fill = append(fill, [2]int{cx, cy})
			default:
				return nil, false // за краем карты
			}
		}
		if len(fill) >= w {
			return nil, false // берега нет вовсе, одна вода
		}
		return fill, true
	}

	// Кандидаты: полоса шириной w поперёк, по краям земля, между ними вода.
	type spot struct {
		x, y, span int
		dx, dy     int      // направление переправы
		a, c       int      // области по обе стороны
		fill       [][2]int // клетки берега под подсыпку
	}
	w := b.Width
	var spots []spot
	// Два направления. Сверху вниз набор ложится как нарисован; поперёк — теми
	// же тайлами, повёрнутыми на четверть (см. putBridgeAcross): своего
	// горизонтального набора у художника нет, а поворот кратен прямому углу и
	// пиксельную сетку не портит.
	for _, d := range [2][2]int{{0, 1}, {1, 0}} {
		dx, dy := d[0], d[1]
		px, py := dy, dx // поперечник
		for y := 1; y < g.P.Height-1; y++ {
			for x := 1; x < g.P.Width-1; x++ {
				if x+px*(w-1) >= g.P.Width || y+py*(w-1) >= g.P.Height {
					continue
				}
				fillTop, ok := bank(x-dx, y-dy, px, py, w)
				if !ok {
					continue
				}
				edge := true
				for i := 0; i < w; i++ {
					cx, cy := x+px*i, y+py*i
					if !g.Level.In(cx, cy) || !g.Level.At(cx, cy).isLiquid() {
						edge = false
						break
					}
				}
				if !edge {
					continue
				}
				// идём по воде до противоположного берега
				span, fillBot := 0, [][2]int(nil)
				for span < bridgeMaxSpan {
					bx, by := x+dx*span, y+dy*span
					allWater := true
					for i := 0; i < w; i++ {
						cx, cy := bx+px*i, by+py*i
						if !g.Level.In(cx, cy) || !g.Level.At(cx, cy).isLiquid() {
							allWater = false
						}
					}
					if f, ok := bank(bx, by, px, py, w); ok && len(f) < w && span >= 1 {
						fillBot = f
						break // берег (возможно, с подсыпкой на клетку)
					}
					if !allWater {
						span = -1
						break
					}
					span++
				}
				if span < 1 || span >= bridgeMaxSpan {
					continue // берега нет или вода слишком широка
				}
				// метка области берётся не в упор к воде, а в её окрестности: у
				// самой кромки тело не помещается и метки нет
				a := bg.bodyLabelNear(x-dx, y-dy, bridgeLabelReach, -dx, -dy)
				c := bg.bodyLabelNear(x+dx*span, y+dy*span, bridgeLabelReach, dx, dy)
				if a == 0 || c == 0 || a == c || !big[a] || !big[c] {
					continue
				}
				spots = append(spots, spot{x, y, span, dx, dy, a, c, append(fillTop, fillBot...)})
			}
		}
	}
	// Короткие переправы первыми: брод в три камня выглядит бродом, а в
	// двенадцать — уже мостом, которого у нас нет.
	sort.Slice(spots, func(i, j int) bool {
		if spots[i].span != spots[j].span {
			return spots[i].span < spots[j].span
		}
		// при равной длине предпочитаем брод сверху вниз: он ложится набором как
		// нарисован, без поворота
		if (spots[i].dy != 0) != (spots[j].dy != 0) {
			return spots[i].dy != 0
		}
		if spots[i].y != spots[j].y {
			return spots[i].y < spots[j].y
		}
		return spots[i].x < spots[j].x
	})

	// Место проверяется постановкой: строим переправу и перемеряем связность.
	// Если областей не убавилось, значит конец упёрся в косу, по которой тело не
	// идёт, — откатываем и берём следующее. Без отката такие пустышки копятся:
	// на карте стояло по четыре брода, и ни один не соединял.
	for i := 0; i < len(spots) && i < bridgeTryCandidates; i++ {
		s := spots[i]
		undo := g.snapshotBridge()
		for _, c := range s.fill {
			g.Level.Set(c[0], c[1], Ground)
		}
		if s.dy != 0 {
			g.putBridge(b, s.x, s.y, s.span)
		} else {
			g.putBridgeAcross(b, s.x, s.y, s.span)
		}
		if len(g.bodyComponents(bridgeBodyRadius).significant(bridgeMinShare)) < len(big) {
			return true
		}
		undo()
	}
	// переправе места нет — остаётся сомкнуть протоку, которая для неё слишком узка
	return g.closeNarrowStrait(bg, big)
}

// snapshotBridge запоминает состояние, которое меняет постановка переправы, и
// возвращает откат. Правок ровно три: тайлы слоя, клетки проходимости и подсыпка
// берега в сетке уровней.
func (g *Generator) snapshotBridge() func() {
	nTiles := len(g.Sparse["bridges"])
	level := map[[2]int]Level{}
	for y := 0; y < g.P.Height; y++ {
		for x := 0; x < g.P.Width; x++ {
			level[[2]int{x, y}] = g.Level.At(x, y)
		}
	}
	cells := make(map[[2]int]bool, len(g.Bridge))
	for c := range g.Bridge {
		cells[c] = true
	}
	return func() {
		g.Sparse["bridges"] = g.Sparse["bridges"][:nTiles]
		for c, lv := range level {
			g.Level.Set(c[0], c[1], lv)
		}
		g.Bridge = cells
	}
}

// closeNarrowStraits смыкает берега там, где переправе стоять не на чем.
//
// Часть областей разделена протокой в одну-две клетки. Брод туда не встаёт: в
// наборе минимум два ряда (примыкание к берегу сверху и снизу), а протока у́же,
// и вдобавок она бывает вертикальной — переправа же только с севера на юг.
// Замер: после бродов остаётся 3 карты из 40, и на всех трёх разделяет именно
// такая протока.
//
// Смыкается она сушей, а не камнями: полоса воды в клетку — это не река, а щель,
// и закрыть её честнее, чем строить над ней переправу. Ширина заливки 2 клетки —
// меньше нельзя, тело радиуса 8 не пройдёт по полосе в 16 px.
func (g *Generator) closeNarrowStrait(bg *bodyGraph, big map[int]bool) bool {
	const maxGap = 2 // клеток воды поперёк щели
	water := func(x, y int) bool {
		return g.Level.In(x, y) && g.Level.At(x, y).isLiquid()
	}
	// метка области, если шагать от (x,y) в сторону (dx,dy) мимо клеток без метки
	labelAlong := func(x, y, dx, dy int) int {
		for i := 0; i <= bridgeLabelReach; i++ {
			x, y = x+dx, y+dy
			if !bg.labels.In(x, y) || water(x, y) {
				return 0
			}
			if v := bg.labels.At(x, y); v != 0 {
				return v
			}
		}
		return 0
	}
	type gap struct {
		x, y, n, dx, dy int
		a, c            int
	}
	var gaps []gap
	for _, d := range [2][2]int{{1, 0}, {0, 1}} {
		for y := 1; y < g.P.Height-1; y++ {
			for x := 1; x < g.P.Width-1; x++ {
				if !water(x, y) || water(x-d[0], y-d[1]) {
					continue // не начало полосы воды
				}
				n := 0
				for n <= maxGap && water(x+d[0]*n, y+d[1]*n) {
					n++
				}
				if n == 0 || n > maxGap {
					continue
				}
				a := labelAlong(x, y, -d[0], -d[1])
				c := labelAlong(x+d[0]*(n-1), y+d[1]*(n-1), d[0], d[1])
				if a == 0 || c == 0 || a == c || !big[a] || !big[c] {
					continue
				}
				gaps = append(gaps, gap{x, y, n, d[0], d[1], a, c})
			}
		}
	}
	sort.Slice(gaps, func(i, j int) bool {
		if gaps[i].n != gaps[j].n {
			return gaps[i].n < gaps[j].n
		}
		if gaps[i].y != gaps[j].y {
			return gaps[i].y < gaps[j].y
		}
		return gaps[i].x < gaps[j].x
	})
	if len(gaps) == 0 {
		return false
	}
	{
		gp := gaps[0]
		// поперёк щели заливаем две соседние линии: одна даёт полосу в 16 px,
		// по которой тело радиуса 8 проходит впритирку и застревает у любой
		// неровности
		px, py := gp.dy, gp.dx // перпендикуляр к направлению щели
		for side := 0; side < 2; side++ {
			for i := 0; i < gp.n; i++ {
				cx, cy := gp.x+gp.dx*i+px*side, gp.y+gp.dy*i+py*side
				if water(cx, cy) {
					g.Level.Set(cx, cy, Ground)
				}
			}
		}
	}
	return true
}

// putBridge выкладывает брод от (x,y) вниз на span клеток воды.
//
// Раскладка набора: start ложится на СУШУ северного берега, end — на сушу
// южного, а вода между ними целиком закрывается телами по кругу. Концы набора
// нарисованы именно как берег (трава сверху у start, снизу у end), поэтому на
// воде им делать нечего — от этого раньше брод не собирался над протокой в одну
// клетку и терял по ряду камней с каждой стороны.
func (g *Generator) putBridge(b Bridges, x, y, span int) {
	put := func(ids []int, cy int, onWater bool) {
		for dx := 0; dx < b.Width && dx < len(ids); dx++ {
			if !g.Level.In(x+dx, cy) {
				continue
			}
			g.addSparse("bridges", b.Sheet, x+dx, cy, ids[dx], nil)
			if onWater {
				g.Bridge[[2]int{x + dx, cy}] = true
			}
		}
	}
	body := b.kit[1 : len(b.kit)-1]
	if len(body) == 0 {
		body = b.kit
	}
	put(b.kit[0], y-1, false) // берег
	for i := 0; i < span; i++ {
		put(body[i%len(body)], y+i, true)
	}
	put(b.kit[len(b.kit)-1], y+span, false) // берег
}

// putBridgeAcross выкладывает брод ПОПЕРЁК, от (x,y) вправо на span клеток.
//
// Набор у художника только вертикальный, поэтому тайлы кладутся повёрнутыми на
// 90° — поворот кратен прямому углу и потому обратим без ресемплинга. Берутся
// ТОЛЬКО тела набора: у его концов нарисована трава, и после поворота травинки
// лежат набок, что заметно. Берега здесь рисует обычный слой coast, и переправа
// выходит просто дорожкой камней от кромки до кромки.
func (g *Generator) putBridgeAcross(b Bridges, x, y, span int) {
	// Поворот ПРОТИВ часовой: верх набора уходит налево, и трава у start
	// оказывается на левом берегу, а у end — на правом, как и требуется. При
	// повороте по часовой берега поменялись бы местами.
	put := func(ids []int, cx int, onWater bool) {
		for dy := 0; dy < b.Width && dy < len(ids); dy++ {
			cy := y + dy
			if !g.Level.In(cx, cy) {
				continue
			}
			g.addSparseRot("bridges", b.Sheet, cx, cy, ids[b.Width-1-dy], 3, nil)
			if onWater {
				g.Bridge[[2]int{cx, cy}] = true
			}
		}
	}
	body := b.kit[1 : len(b.kit)-1]
	if len(body) == 0 {
		body = b.kit
	}
	put(b.kit[0], x-1, false) // берег
	for i := 0; i < span; i++ {
		put(body[i%len(body)], x+i, true)
	}
	put(b.kit[len(b.kit)-1], x+span, false) // берег
}
