package main

// stage_plateau.go — возвышенность: травяной верх + скальный обрыв на юг
// (эталон художника — assets/biomes/forest/tiles/elevated_test.tsx.tmx,
// посмотреть: `worldgen ref assets/biomes/forest`).
//
// Четыре роли, и у КАЖДОЙ свой регион для dual-grid ключа (autotiling.md §2):
//
//	TOP  = клетки Plateau            ROCK = клетки g.Cliff (юбка под южной кромкой)
//
//	plateau     (grass_cliff)  ключ по TOP  — интерьер верха + кромка на север/запад/восток
//	rock_top    (grass_top)    ключ по TOP  — ЮЖНАЯ кромка: трава свисает на скалу
//	rock_face   (cliff_face)   ключ по ROCK — тело обрыва и его бока
//	rock_bottom (grass_bottom) ключ по ROCK — подножие: трава у основания скалы
//
// Прошлая версия считала ключ rock_top и rock_face по ОБЪЕДИНЕНИЮ TOP∪ROCK.
// Граница «верх ↔ скала» при этом исчезала: клетки южной кромки получали ключ
// 1,1,1,1 и на обрыв ложился ровный зелёный тайл, а тело скалы вылезало вверх
// в линию травы. Отсюда и рваный свес, и огрызки на концах обрыва.
//
// Проверка выбора набора — по составу разметки .tsx:
//   grass_top/grass_bottom размечены только ключами вида «верхние углы внутри,
//   нижние снаружи» (1,1,0,0 / 1,1,0,1 / 1,1,1,0 / 1,0,0,0 / 0,1,0,0) — это ровно
//   ЮЖНАЯ граница региона; cliff_face добавляет к ним бока (1,0,0,1 / 0,1,1,0) и
//   заливку 1,1,1,1, но НЕ имеет северной границы (0,0,1,1) — её рисует rock_top.

const cliffHeight = 4 // клеток тела обрыва ниже южной кромки плато

func (g *Generator) stagePlateau() {
	W, H := g.P.Width, g.P.Height
	g.PlateauLayer = NewGrid[uint16](W, H)

	inTop := func(x, y int) bool { return g.Level.In(x, y) && g.Level.At(x, y) == Plateau }
	inRock := func(x, y int) bool { return g.Cliff[[2]int{x, y}] }

	plateau := g.Manifest.Terrains["plateau"]
	rtop := g.Manifest.Terrains["rock_top"]
	rface := g.Manifest.Terrains["rock_face"]
	rbot := g.Manifest.Terrains["rock_bottom"]

	// 1) ТЕЛО ОБРЫВА — первым, потому что травяной свес (grass_top) кладётся
	//    ПОВЕРХ него: его тайлы полупрозрачные (пучки травы и подсвеченный
	//    гребень, сквозь них должна просвечивать скала). Ключ по ROCK.
	//    Верхний ряд юбки закрываем сплошной скалой: в cliff_face северной
	//    границы (0,0,1,1) нет намеренно — по замыслу художника её закрывает трава.
	for y := 0; y <= H; y++ {
		for x := 0; x <= W; x++ {
			k := cornerKey(inRock, x, y)
			if !anyCorner(k) {
				continue
			}
			switch {
			case !k[0] && !k[1]: // северная кромка юбки — сплошная скала под траву
				if !k[2] || !k[3] {
					// в скале только ОДИН нижний угол (0,0,1,0 / 0,0,0,1) — это
					// косой угол на ступеньке или на торце линии обрыва. Сплошной
					// тайл занял бы всю клетку и торчал бы кубом камня выше общей
					// линии обрыва; отдельного тайла на четверть в наборе нет,
					// поэтому клетку оставляем траве.
					continue
				}
				if ids, ok := rface.Corner["1,1,1,1"]; ok && len(ids) > 0 && g.Level.In(x, y) {
					g.addSparse("cliff", rface.Sheet, x, y, variantAt(g.Seed, x, y, 6, ids), nil)
				}
			case !k[2] && !k[3]: // южная кромка юбки — подножие
				g.putSparseCorner("cliff", rbot, k, x, y, 7)
			default: // стенка и её левый/правый бок
				g.putSparseCorner("cliff", rface, k, x, y, 6)
			}
		}
	}

	// 2) ТРАВЯНОЙ СВЕС на гребне обрыва — ПОВЕРХ уже уложенной скалы. Ключ по TOP:
	//    на гребне выходит 1,1,0,0 (трава сверху, скала снизу — ровно то, чем
	//    размечен grass_top), ниже травы нет. Тайлы набора полупрозрачные, они
	//    не заменяют скалу, а ложатся на неё; поэтому шаг 1 и обязан был закрыть
	//    верхний ряд юбки сплошным камнем — без него из-под травы светила
	//    пустота и линия обрыва резалась прямым швом.
	for y := 0; y <= H; y++ {
		for x := 0; x <= W; x++ {
			if !g.Level.In(x, y) || !(inRock(x-1, y) || inRock(x, y)) {
				continue // свес живёт только над телом обрыва
			}
			ids, ok := rtop.Corner[cornerKeyStr(cornerKey(inTop, x, y))]
			if !ok || len(ids) == 0 {
				// Комбинация не размечена. Два разных случая:
				//  - клетка стоит на ВЕРХНЕМ срезе скалы (над ней скалы нет) —
				//    это ступенька линии обрыва; без травы там торчал бы голый
				//    камень выше общей линии. Кладём ровную бахрому.
				//  - иначе это торец обрыва (боковые ключи 1,0,0,1 / 0,1,1,0,
				//    которых в grass_top нет намеренно) — трава на торце не
				//    растёт, оставляем камень. Раньше подбор по Хеммингу совал
				//    сюда кусок от соседней комбинации — это и были огрызки.
				onRockTop := (inRock(x, y) && !inRock(x, y-1)) ||
					(inRock(x-1, y) && !inRock(x-1, y-1))
				if !onRockTop {
					continue
				}
				if ids, ok = rtop.Corner["1,1,0,0"]; !ok || len(ids) == 0 {
					continue
				}
			}
			g.addSparse("cliff", rtop.Sheet, x, y, variantAt(g.Seed, x, y, 5, ids), nil)
		}
	}

	// 3) ВЕРХ ПЛАТО. Проход на клетку шире региона: dual-grid смотрит вверх-влево,
	//    поэтому южная и восточная кромки живут в клетке ЗА пределами плато.
	for y := 0; y <= H; y++ {
		for x := 0; x <= W; x++ {
			k := cornerKey(inTop, x, y)
			if !anyCorner(k) {
				continue
			}
			if ids, ok := g.resolve("grass_cliff", plateau.Corner, k); ok && g.Level.In(x, y) {
				g.PlateauLayer.Set(x, y, uint16(variantAt(g.Seed, x, y, 4, ids)+1))
			}
		}
	}
}

// anyCorner — хоть один угол внутри региона (иначе клетка пустая).
func anyCorner(k [4]bool) bool { return k[0] || k[1] || k[2] || k[3] }

// resolve — обёртка над resolveCorner, которая считает промахи точного ключа.
// Промах означает дыру в ручной разметке: тайл всё равно встанет (ближайший по
// Хеммингу), но на стыке будет виден чужой кусок. Счётчик уходит в отчёт (E13).
func (g *Generator) resolve(role string, set map[string][]int, k [4]bool) ([]int, bool) {
	if len(set) == 0 {
		return nil, false
	}
	if ids, ok := set[cornerKeyStr(k)]; ok && len(ids) > 0 {
		return ids, true
	}
	ids, ok := resolveCorner(set, k)
	if ok {
		g.cornerMiss[role]++
	}
	return ids, ok
}

// putSparseCorner кладёт тайл роли в разрежённый слой по ТОЧНОМУ ключу.
// Возвращает false, если такого ключа в наборе нет — вызывающий решает, чем
// закрыть клетку. Приблизительный подбор здесь запрещён намеренно: именно он
// ставил зелёный тайл посреди скалы.
func (g *Generator) putSparseCorner(layer string, t Terrain, k [4]bool, x, y, salt int) bool {
	if len(t.Corner) == 0 || !g.Level.In(x, y) {
		return false
	}
	ids, ok := t.Corner[cornerKeyStr(k)]
	if !ok || len(ids) == 0 {
		// точного нет — берём ближайший, но помечаем как дыру покрытия
		ids, ok = resolveCorner(t.Corner, k)
		if !ok {
			return false
		}
		g.cornerMiss[t.Wangset+" "+cornerKeyStr(k)]++
	}
	g.addSparse(layer, t.Sheet, x, y, variantAt(g.Seed, x, y, salt, ids), nil)
	return true
}
