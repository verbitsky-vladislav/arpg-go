package main

// autotile.go — превращение сетки уровней в конкретные id тайлов.
// Площадные роли (ground/plateau/liquid) — по 8-соседней blob-маске; переходы
// и обрыв — по 4-битной Wang-маске (N,E,S,W). Ключ набора в манифесте — это
// НОРМАЛИЗОВАННАЯ маска (целое), одинаково вычисляемая при добыче из .tmx
// (tmxhint) и при генерации, поэтому соответствие «маска → тайл» сходится.

import (
	"math/bits"
	"sort"
)

// normalizeBlob обнуляет «висящие» угловые биты: угол значим, только если
// выставлены оба смежных с ним ортогональных соседа. Так 256 комбинаций
// сводятся к 47 различимым (классический blob-47).
// Порядок битов совпадает с nb8: 0=N 1=NE 2=E 3=SE 4=S 5=SW 6=W 7=NW.
func normalizeBlob(m uint8) uint8 {
	n := m&1 != 0
	e := m&4 != 0
	s := m&16 != 0
	w := m&64 != 0
	out := m & 0b01010101 // оставить ортогональные биты (N,E,S,W)
	if n && e {
		out |= m & 2 // NE
	}
	if s && e {
		out |= m & 8 // SE
	}
	if s && w {
		out |= m & 32 // SW
	}
	if n && w {
		out |= m & 128 // NW
	}
	return out
}

// blobKey — строковый ключ набора для нормализованной маски.
func blobKey(m uint8) string { return itoa(int(normalizeBlob(m))) }

// resolveFromSet ищет тайл по ключу маски; при промахе берёт ближайший по
// Хеммингу среди имеющихся ключей, иначе -1. Гарантирует, что дырки покрытия
// не роняют рендер (worldgen.spec §3, риск покрытия).
func resolveFromSet(set map[string]int, key uint8, keyBits int) (int, bool) {
	if set == nil {
		return -1, false
	}
	if id, ok := set[itoa(int(key))]; ok {
		return id, true
	}
	// ближайший по числу различающихся бит
	best, bestID, found := 99, -1, false
	for k, id := range set {
		kv, ok := atoi(k)
		if !ok {
			continue
		}
		d := bits.OnesCount8(uint8(kv) ^ key)
		if d < best {
			best, bestID, found = d, id, true
		}
	}
	return bestID, found
}

// resolveCorner ищет ВСЕ варианты тайла по угловому ключу; при промахе — варианты
// ближайшего по Хеммингу ключа (дырки покрытия не роняют рендер). id локальные.
func resolveCorner(set map[string][]int, k [4]bool) ([]int, bool) {
	if len(set) == 0 {
		return nil, false
	}
	key := cornerKeyStr(k)
	if ids, ok := set[key]; ok && len(ids) > 0 {
		return ids, true
	}
	// Порядок перебора map случаен, поэтому среди РАВНОУДАЛЁННЫХ ключей выбор
	// прыгал бы между запусками и один сид давал бы разные карты. Идём по
	// отсортированным ключам: подбор остаётся приблизительным, но воспроизводимым.
	keys := make([]string, 0, len(set))
	for sk := range set {
		keys = append(keys, sk)
	}
	sort.Strings(keys)
	best := 5
	var bestIDs []int
	for _, sk := range keys {
		ids := set[sk]
		var kk [4]bool
		p := 0
		for i := 0; i < 7; i += 2 { // позиции 0,2,4,6 строки "b,b,b,b"
			if sk[i] == '1' {
				kk[p] = true
			}
			p++
		}
		d := 0
		for i := 0; i < 4; i++ {
			if kk[i] != k[i] {
				d++
			}
		}
		if d < best && len(ids) > 0 {
			best, bestIDs = d, ids
		}
	}
	return bestIDs, bestIDs != nil
}

// variantAt детерминированно выбирает вариант из списка по позиции и соли —
// одинаковый seed → одинаковая карта, но соседние клетки берут разные тайлы.
func variantAt(seed uint64, x, y, salt int, ids []int) int {
	if len(ids) == 1 {
		return ids[0]
	}
	h := hash2(x*2654435761+salt*40503, y*40503+salt, seed)
	return ids[int(h*float64(len(ids)))%len(ids)]
}

// paintLand красит наземную поверхность с учётом берега.
// Ключ клетки — угловой, относительно present (членство в этой поверхности).
// Поверхность выбирается так:
//
//	ключ 1,1,1,1 (все углы — суша)         → fullRole (интерьер, напр. grass_ground)
//	иначе, клетка граничит с водой          → waterRole (берег, напр. grass_water)
//	иначе (граница с другой сушей/тропой)   → edgeRole (кромка того же материала)
//
// Пустые роли просто пропускаются (сквозь прозрачные углы видно воду-фон/низ).
// waterRole лежит на другом листе (Water_coasts), поэтому берег пишется в
// разрежённый слой coastLayer (свой лист), а интерьер/кромка того же материала
// (Ground_grass) — в плотный dst.
//
// Подложки здесь нет намеренно: наборы grass_shadow/mud_shadow, которые раньше
// клались под каждую клетку суши, оказались НЕ подложкой материала, а тенью
// возвышенности — их кладёт stagePlateauShadow вокруг плато.
func (g *Generator) paintLand(present func(x, y int) bool, fullRole, edgeRole, waterRole, coastLayer string, dst *Grid[uint16]) int {
	full := g.Manifest.Terrains[fullRole]
	edge := g.Manifest.Terrains[edgeRole]
	water := g.Manifest.Terrains[waterRole]
	n := 0
	for y := 0; y < g.P.Height; y++ {
		for x := 0; x < g.P.Width; x++ {
			k := cornerKey(present, x, y)
			if !(k[0] || k[1] || k[2] || k[3]) {
				continue // ни один угол не в террейне — пусто
			}
			switch {
			case k[0] && k[1] && k[2] && k[3]: // интерьер
				if ids, ok := resolveCorner(full.Corner, k); ok {
					dst.Set(x, y, uint16(variantAt(g.Seed, x, y, 2, ids)+1))
				}
			case g.touchesLiquid(x, y): // берег → другой лист, разрежённый слой
				if ids, ok := resolveCorner(water.Corner, k); ok {
					g.addSparse(coastLayer, water.Sheet, x, y, variantAt(g.Seed, x, y, 3, ids), nil)
				}
			default: // кромка того же материала (напр. mud над травой)
				if ids, ok := resolveCorner(edge.Corner, k); ok {
					dst.Set(x, y, uint16(variantAt(g.Seed, x, y, 2, ids)+1))
				}
			}
			n++
		}
	}
	return n
}

// paintUnder красит ПОДЛОЖКУ поверхности: там, где террейн present касается
// клетки хотя бы одним углом, кладётся ЗАЛИВОЧНЫЙ тайл роли (ключ 1,1,1,1), а не
// переходный. Форму подложке задаёт слой above СВЕРХУ — это он вырезает по ней
// свои переходные тайлы. Так собрана земля в карте автора (forest.tmx): в нижнем
// слое у mud всегда ключ 1,1,1,1, а очертания тропы даёт grass_ground над ним.
// Если подложку красить переходными тайлами, её кромка и кромка травы не
// совпадают попиксельно и между ними светится вода-фон.
//
// Клетки, где above сплошной (все четыре угла его), пропускаются: подложки там
// всё равно не видно, а слой она бы раздула на всю сушу.
// У воды сплошную подложку класть нельзя — она вылезет на воду. Свой берег
// (waterRole с листа Water_coasts) подложка получает, только если сверху её не
// кроет ничто, то есть тропа сама вышла к воде; иначе берег даёт слой above, и
// грунтовый берег под ним был бы не виден.
func (g *Generator) paintUnder(present, above func(x, y int) bool, fillRole, waterRole, coastLayer string, dst *Grid[uint16]) int {
	fill := g.Manifest.Terrains[fillRole]
	water := g.Manifest.Terrains[waterRole]
	solid, hasSolid := resolveCorner(fill.Corner, [4]bool{true, true, true, true})
	if !hasSolid && fill.Fill > 0 {
		solid, hasSolid = []int{fill.Fill}, true
	}
	n := 0
	for y := 0; y < g.P.Height; y++ {
		for x := 0; x < g.P.Width; x++ {
			k := cornerKey(present, x, y)
			if !(k[0] || k[1] || k[2] || k[3]) {
				continue // подложке здесь нечего подпирать
			}
			a := cornerKey(above, x, y)
			if a[0] && a[1] && a[2] && a[3] {
				continue // сверху сплошной тайл
			}
			if !(k[0] && k[1] && k[2] && k[3]) && g.touchesLiquid(x, y) {
				if !(a[0] || a[1] || a[2] || a[3]) {
					if ids, ok := resolveCorner(water.Corner, k); ok {
						g.addSparse(coastLayer, water.Sheet, x, y, variantAt(g.Seed, x, y, 3, ids), nil)
					}
				}
				continue
			}
			if hasSolid {
				dst.Set(x, y, uint16(variantAt(g.Seed, x, y, 2, solid)+1))
				n++
			}
		}
	}
	return n
}

// touchesLiquid — есть ли среди 8 соседей клетки жидкая.
func (g *Generator) touchesLiquid(x, y int) bool {
	for _, d := range nb8 {
		nx, ny := x+d[0], y+d[1]
		if g.Level.In(nx, ny) && g.Level.At(nx, ny).isLiquid() {
			return true
		}
	}
	return false
}
