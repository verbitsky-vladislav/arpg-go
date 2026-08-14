package worldgen

// decor.go — библиотека декора и её расстановка ШТАМПАМИ. Декор — не отдельные
// тайлы, а многотайловые куски (травяные кустики, камыш, кувшинки, рябь воды,
// напольные пятна), снятые с карты художника по двойному признаку смежности
// (в карте И по индексу в атласе). Без этого слоя карта — плоская заливка
// (SUMMARY §1.3, §1.5). Файл decor.json лежит рядом с manifest.json.

import (
	"encoding/json"
	"math/rand"
)

// StampCell — одна клетка штампа: смещение + лист и локальный id тайла.
type StampCell struct {
	Dx    int    `json:"dx"`
	Dy    int    `json:"dy"`
	Sheet string `json:"sheet"`
	Tile  int    `json:"tile"`
}

// Stamp — многотайловый кусок декора.
type Stamp struct {
	W     int         `json:"w"`
	H     int         `json:"h"`
	Cells []StampCell `json:"cells"`
}

// DecorLib — группы штампов (grass/reeds/lilies/water_detail/ground_spots).
type DecorLib struct {
	Stamps map[string][]Stamp `json:"stamps"`
}

// loadDecor читает decor.json биома (если есть). Отсутствие файла — не ошибка.
func loadDecor(m *Manifest) *DecorLib {
	raw, err := m.readFile("decor.json")
	if err != nil {
		return &DecorLib{Stamps: map[string][]Stamp{}}
	}
	var d DecorLib
	if json.Unmarshal(raw, &d) != nil || d.Stamps == nil {
		return &DecorLib{Stamps: map[string][]Stamp{}}
	}
	return &d
}

// scatterStamps кладёт штампы группы group в слой layer по зоне pred, пока
// покрытие зоны не достигнет frac. occ — общая карта занятости (штампы не лезут
// друг на друга), rng — общий поток случайности вызывающей стадии: и то и другое
// снаружи, чтобы несколько групп раскладывались согласованно.
func (g *Generator) scatterStamps(rng *rand.Rand, occ map[[2]int]bool, group, layer string,
	pred func(x, y int) bool, frac float64) {
	lib := g.Decor.Stamps[group]
	if len(lib) == 0 {
		return
	}
	zone := 0
	for y := 0; y < g.P.Height; y++ {
		for x := 0; x < g.P.Width; x++ {
			if pred(x, y) {
				zone++
			}
		}
	}
	target := int(float64(zone) * frac)
	placed, attempts := 0, 0
	for placed < target && attempts < target*40+200 {
		attempts++
		s := lib[rng.Intn(len(lib))]
		placed += g.tryStamp(occ, s, layer, rng.Intn(g.P.Width), rng.Intn(g.P.Height), pred)
	}
}

// scatterStampsOn — то же, но точка вброса берётся из готового списка клеток
// зоны, а не наугад по всей карте, и набор штампов передаётся явно. Для узких
// зон (грунт троп, русло реки — проценты площади) случайный вброс по всей карте
// почти всегда мимо, а крупные штампы в них не влезают вовсе.
func (g *Generator) scatterStampsOn(rng *rand.Rand, occ map[[2]int]bool, lib []Stamp, layer string,
	pred func(x, y int) bool, cells [][2]int, frac float64) {
	if len(lib) == 0 || len(cells) == 0 {
		return
	}
	target := int(float64(len(cells)) * frac)
	placed, attempts := 0, 0
	for placed < target && attempts < target*40+200 {
		attempts++
		s := lib[rng.Intn(len(lib))]
		c := cells[rng.Intn(len(cells))]
		placed += g.tryStamp(occ, s, layer, c[0], c[1], pred)
	}
}

// tryStamp кладёт штамп левым-верхним углом в (px,py), если ВСЕ его клетки в
// зоне и не заняты. Возвращает число уложенных клеток (0 — не влез).
func (g *Generator) tryStamp(occ map[[2]int]bool, s Stamp, layer string, px, py int,
	pred func(x, y int) bool) int {
	for _, c := range s.Cells {
		x, y := px+c.Dx, py+c.Dy
		if !pred(x, y) || occ[[2]int{x, y}] {
			return 0
		}
	}
	for _, c := range s.Cells {
		x, y := px+c.Dx, py+c.Dy
		occ[[2]int{x, y}] = true
		g.addSparse(layer, c.Sheet, x, y, c.Tile, g.sheetAnim(c.Sheet))
	}
	return len(s.Cells)
}
