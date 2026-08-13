package main

// stage_plateau.go — плато: травяной верх + скальный обрыв на юг (рок-плато
// художника, см. elevated_test). Разделение поверхности верха БЕЗ наложения:
//   передняя (южная) кромка         → grass_top   (травяной свес над скалой);
//   зад/бока/интерьер               → grass_cliff (Ground_grass);
//   тело обрыва                     → cliff_face  (spots_rock);
//   подножие (ряд ниже тела)        → grass_bottom.
// Ключи — dual-grid по ОБЪЕДИНЕНИЮ плато+скала (SUMMARY §1.7).

const cliffHeight = 3 // на сколько тайлов скала свисает ниже южной кромки плато

func (g *Generator) stagePlateau() {
	isPlateau := func(x, y int) bool { return g.Level.In(x, y) && g.Level.At(x, y) == Plateau }
	frontEdge := func(x, y int) bool { return isPlateau(x, y) && !isPlateau(x, y+1) } // южная кромка

	// тело скалы: юбка cliffHeight ниже передней кромки
	rock := map[[2]int]bool{}
	for y := 0; y < g.P.Height; y++ {
		for x := 0; x < g.P.Width; x++ {
			if !frontEdge(x, y) {
				continue
			}
			for d := 1; d <= cliffHeight; d++ {
				ry := y + d
				if !g.Level.In(x, ry) || isPlateau(x, ry) {
					break
				}
				rock[[2]int{x, ry}] = true
			}
		}
	}
	inRock := func(x, y int) bool { return rock[[2]int{x, y}] }
	inU := func(x, y int) bool { return isPlateau(x, y) || inRock(x, y) }

	plateau := g.Manifest.Terrains["plateau"]
	rtop := g.Manifest.Terrains["rock_top"]
	rface := g.Manifest.Terrains["rock_face"]
	rbot := g.Manifest.Terrains["rock_bottom"]

	g.PlateauLayer = NewGrid[uint16](g.P.Width, g.P.Height)
	putDense := func(t Terrain, x, y, salt int) {
		if ids, ok := resolveCorner(t.Corner, cornerKey(inU, x, y)); ok {
			g.PlateauLayer.Set(x, y, uint16(variantAt(g.Seed, x, y, salt, ids)+1))
		}
	}
	// putU — ключ по объединению плато+скала (тело/верх/бок-на-скале);
	// putR — ключ по телу скалы (фут и правое обрамление за пределами тела).
	putU := func(t Terrain, x, y, salt int) {
		if ids, ok := resolveCorner(t.Corner, cornerKey(inU, x, y)); ok {
			g.addSparse("cliff", t.Sheet, x, y, variantAt(g.Seed, x, y, salt, ids), nil)
		}
	}
	putR := func(t Terrain, x, y, salt int) {
		if ids, ok := resolveCorner(t.Corner, cornerKey(inRock, x, y)); ok {
			g.addSparse("cliff", t.Sheet, x, y, variantAt(g.Seed, x, y, salt, ids), nil)
		}
	}

	// 1) Верх плато: grass_cliff на клетки плато КРОМЕ передней кромки.
	for y := 0; y < g.P.Height; y++ {
		for x := 0; x < g.P.Width; x++ {
			if isPlateau(x, y) && !frontEdge(x, y) {
				putDense(plateau, x, y, 4)
			}
		}
	}

	// 2) Скала + свес + подножие. Перебираем на шаг шире (dual-grid «видит»
	//    вверх-влево, поэтому правая/нижняя кромка живёт в клетке за пределами тела).
	isSide := func(k [4]bool) bool { // целая боковая колонка: X..X (лево-в-скале) / .XX. (право)
		return (k[0] && !k[1] && !k[2] && k[3]) || (!k[0] && k[1] && k[2] && !k[3])
	}
	minx, miny, maxx, maxy := g.P.Width, g.P.Height, -1, -1
	for c := range rock {
		minx, miny = min(minx, c[0]), min(miny, c[1])
		maxx, maxy = max(maxx, c[0]), max(maxy, c[1])
	}
	for y := miny - 1; y <= maxy+2; y++ {
		for x := minx - 1; x <= maxx+2; x++ {
			if !g.Level.In(x, y) {
				continue
			}
			switch {
			case frontEdge(x, y): // передняя кромка плато → травяной свес (ключ по объединению)
				putU(rtop, x, y, 5)
			case inRock(x, y): // тело скалы + левое обрамление (ключ по объединению)
				putU(rface, x, y, 6)
			default:
				kR := cornerKey(inRock, x, y)
				if !(kR[0] || kR[1] || kR[2] || kR[3]) {
					continue // не касается скалы
				}
				if isSide(kR) { // линия кончилась → обрамление скалы (правый бок)
					putR(rface, x, y, 6)
				} else { // ровный/ступенчатый низ → подножие (ключ по телу скалы)
					putR(rbot, x, y, 7)
				}
			}
		}
	}
}
