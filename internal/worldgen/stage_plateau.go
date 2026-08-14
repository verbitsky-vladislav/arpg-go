package worldgen

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

// maxCliffDepth — самая глубокая юбка среди типов возвышенностей. Конкретная
// глубина берётся из типа клетки (cliffDepthAt); здесь только верхняя граница
// для проходов, которым нужно осмотреть всё тело обрыва разом.
const maxCliffDepth = 4

func (g *Generator) stagePlateau() {
	W, H := g.P.Width, g.P.Height
	g.PlateauLayer = NewGrid[uint16](W, H)

	inTop := func(x, y int) bool { return g.Level.In(x, y) && g.Level.At(x, y) == Plateau }
	inRock := func(x, y int) bool { return g.Cliff[[2]int{x, y}] }

	plateau := g.Manifest.Terrains["plateau"]
	rtop := g.Manifest.Terrains["rock_top"]
	rface := g.Manifest.Terrains["rock_face"]
	rbot := g.Manifest.Terrains["rock_bottom"]

	// 1) ТЕЛО И ПОДНОЖИЕ обрыва по ключу ROCK. Верхний ряд юбки здесь НЕ
	//    трогаем: его целиком рисует grass_top на шаге 2.
	for y := 0; y <= H; y++ {
		for x := 0; x <= W; x++ {
			k := cornerKey(inRock, x, y)
			if !anyCorner(k) {
				continue
			}
			switch {
			case !k[0] && !k[1]:
				continue // гребень — не наше дело, см. шаг 2
			case !k[2] && !k[3]: // южная кромка юбки — подножие
				g.putSparseCorner("cliff", rbot, k, x, y, 7)
			default: // стенка, её бока и углы ступеней
				g.putCliffFace(rface, k, inRock, x, y)
			}
		}
	}

	// 2) ГРЕБЕНЬ обрыва — набор grass_top по ключу TOP. Это не «свес поверх
	//    камня», а самостоятельный верхний ряд: его тайлы непрозрачны и несут
	//    сразу и скалу, и траву на ней. Поэтому подкладывать под них сплошной
	//    камень нельзя — раньше именно эта подкладка и торчала кубами на
	//    ступенях, а угловые кадры набора вставали поверх неё как попало.
	//
	//    Ключ берётся по МАКУШКЕ: у клетки гребня верхние углы внутри плато,
	//    нижние снаружи, что и даёт размеченные комбинации — ровный участок
	//    1,1,0,0, ступени 1,1,0,1 и 1,1,1,0, торцы 0,1,0,0 и 1,0,0,0.
	for y := 0; y <= H; y++ {
		for x := 0; x <= W; x++ {
			if !g.Level.In(x, y) {
				continue
			}
			r := cornerKey(inRock, x, y)
			if !anyCorner(r) || r[0] || r[1] {
				continue // гребень — только там, где выше скалы уже нет
			}
			tk := cornerKeyStr(cornerKey(inTop, x, y))
			// Ключи с ОДНИМ верхним углом рисуются двумя разными кадрами:
			// маленьким уголком, если гора здесь кончается, и залитым, если это
			// изгиб и кромка идёт дальше. Различаем по клетке за углом; в
			// разметке первым идёт уголок (он меньше по id), вторым залитый.
			if ids, ok := rtop.Corner[tk]; ok && len(ids) > 1 {
				if side, has := map[string]int{
					// Единственный угол макушки показывает, с какой стороны
					// осталась гора, а значит — где у ленты КРАЙ. Угол NW: макушка
					// слева, край справа. Угол NE: макушка справа, край слева.
					// Продолжение ленты проверяем именно со стороны края.
					"1,0,0,0": +1, // край справа
					"0,1,0,0": -1, // край слева
				}[tk]; has {
					// Смещения несимметричны: угол NW живёт в клетке (x-1,y-1),
					// угол NE — в (x,y-1), поэтому соседняя колонка скалы слева
					// это x-2, а справа x+1.
					nx := x + 1
					if side < 0 {
						nx = x - 2
					}
					// лента идёт дальше — изгиб (залитый кадр), обрывается —
					// конец горы (маленький уголок, первый в разметке)
					i := 0
					if inRock(nx, y) || inRock(nx, y+1) {
						i = 1
					}
					g.addSparse("cliff", rtop.Sheet, x, y, ids[i], nil)
					continue
				}
			}
			ids, ok := rtop.Corner[tk]
			if !ok || len(ids) == 0 {
				// Комбинации нет в разметке — клетка остаётся травой, и в линии
				// обрыва получается дыра. Считаем такие случаи: это прямой список
				// того, что художнику осталось разметить (E13).
				g.cornerMiss[rtop.Wangset+" "+tk]++
				continue
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

// putCliffFace кладёт тайл тела обрыва. Для ключей с вырезом в ВЕРХНЕМ углу
// (0,1,1,1 и 1,0,1,1) одного ключа мало: клетка выглядит по-разному, смотря
// продолжается ли скала за вырезом.
//
//	продолжается → это внутренний угол, стык двух участков ленты → первый тайл;
//	не продолжается → лента здесь кончается, это торец → второй тайл, с просветом.
//
// Порядок вариантов в разметке и есть договорённость: сначала внутренний угол,
// потом торцевой. Для остальных ключей вариант выбирается как обычно, по позиции.
func (g *Generator) putCliffFace(t Terrain, k [4]bool, inRock func(x, y int) bool, x, y int) {
	key := cornerKeyStr(k)
	cut, hasCut := map[string][2][2]int{
		// ключ → две клетки за вырезом: если хоть в одной скала, угол внутренний
		"0,1,1,1": {{-2, -1}, {-1, -2}}, // вырез в NW
		"1,0,1,1": {{1, -1}, {0, -2}},   // вырез в NE
	}[key]
	if !hasCut {
		if ids, ok := t.Corner[key]; ok && len(ids) > 0 && g.Level.In(x, y) {
			g.addSparse("cliff", t.Sheet, x, y, variantAt(g.Seed, x, y, 6, ids), nil)
			return
		}
		if fill, ok := t.Corner["1,1,1,1"]; ok && len(fill) > 0 && g.Level.In(x, y) {
			g.addSparse("cliff", t.Sheet, x, y, variantAt(g.Seed, x, y, 6, fill), nil)
		}
		return
	}
	ids := t.Corner[key]
	if len(ids) == 0 {
		// Комбинации нет — и это норма: геометрию обрыва несёт гребень
		// (grass_top по ключу МАКУШКИ), а тело внутри контура ровное. Ключи с
		// вырезом в верхнем углу возникали только потому, что раньше ключ тела
		// считался по самой скале; подставлять сюда чужой кадр нельзя, кладём
		// заливку.
		if fill, ok := t.Corner["1,1,1,1"]; ok && len(fill) > 0 && g.Level.In(x, y) {
			g.addSparse("cliff", t.Sheet, x, y, variantAt(g.Seed, x, y, 6, fill), nil)
		}
		return
	}
	inner := inRock(x+cut[0][0], y+cut[0][1]) || inRock(x+cut[1][0], y+cut[1][1])
	i := 0
	if !inner && len(ids) > 1 {
		i = 1
	}
	if g.Level.In(x, y) {
		g.addSparse("cliff", t.Sheet, x, y, ids[i], nil)
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
