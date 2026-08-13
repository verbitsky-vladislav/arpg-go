package main

// decor.go — библиотека декора и её расстановка ШТАМПАМИ. Декор — не отдельные
// тайлы, а многотайловые куски (травяные кустики, камыш, кувшинки, рябь воды,
// напольные пятна), снятые с карты художника по двойному признаку смежности
// (в карте И по индексу в атласе). Без этого слоя карта — плоская заливка
// (SUMMARY §1.3, §1.5). Файл decor.json лежит рядом с manifest.json.

import (
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
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
func loadDecor(biomeDir string) *DecorLib {
	raw, err := os.ReadFile(filepath.Join(biomeDir, "decor.json"))
	if err != nil {
		return &DecorLib{Stamps: map[string][]Stamp{}}
	}
	var d DecorLib
	if json.Unmarshal(raw, &d) != nil || d.Stamps == nil {
		return &DecorLib{Stamps: map[string][]Stamp{}}
	}
	return &d
}

// stageDecor раскладывает штампы декора по зонам (шаг после воды/земли/троп).
// Плотности — доли покрытия зоны, замерены по карте художника (SUMMARY §1.5).
func (g *Generator) stageDecor() {
	if g.Decor == nil || len(g.Decor.Stamps) == 0 {
		return
	}
	rng := rand.New(rand.NewSource(int64(g.Seed) ^ 0xDEC0DE))
	occ := map[[2]int]bool{}

	isWater := func(x, y int) bool { return g.Level.In(x, y) && g.Level.At(x, y).isLiquid() }
	nearLand := func(x, y int) bool {
		for a := -2; a <= 2; a++ {
			for b := -2; b <= 2; b++ {
				if g.Level.In(x+a, y+b) && g.Level.At(x+a, y+b).isLand() {
					return true
				}
			}
		}
		return false
	}
	landOpen := func(x, y int) bool { // трава: суша, не тропа
		return g.Level.In(x, y) && g.Level.At(x, y).isLand() && !g.Trail[[2]int{x, y}]
	}

	// зоны с оценкой размера (для целевого покрытия)
	countZone := func(pred func(x, y int) bool) int {
		n := 0
		for y := 0; y < g.P.Height; y++ {
			for x := 0; x < g.P.Width; x++ {
				if pred(x, y) {
					n++
				}
			}
		}
		return n
	}

	// scatter кладёт штампы группы, пока покрытие зоны не достигнет frac.
	scatter := func(group, layer string, pred func(x, y int) bool, frac float64) {
		lib := g.Decor.Stamps[group]
		if len(lib) == 0 {
			return
		}
		zone := countZone(pred)
		target := int(float64(zone) * frac)
		placed, attempts := 0, 0
		for placed < target && attempts < target*40+200 {
			attempts++
			s := lib[rng.Intn(len(lib))]
			px := rng.Intn(g.P.Width)
			py := rng.Intn(g.P.Height)
			ok := true
			for _, c := range s.Cells {
				x, y := px+c.Dx, py+c.Dy
				if !pred(x, y) || occ[[2]int{x, y}] {
					ok = false
					break
				}
			}
			if !ok {
				continue
			}
			for _, c := range s.Cells {
				x, y := px+c.Dx, py+c.Dy
				occ[[2]int{x, y}] = true
				g.addSparse(layer, c.Sheet, x, y, c.Tile, nil)
			}
			placed += len(s.Cells)
		}
	}

	shallowNear := func(x, y int) bool { return isWater(x, y) && nearLand(x, y) }
	deepWater := func(x, y int) bool { return isWater(x, y) && !nearLand(x, y) }

	// порядок: сначала вода (рябь, кувшинки, камыш), потом суша (пятна, трава)
	scatter("water_detail", "liquid_detail", isWater, 0.26)
	scatter("lilies", "surface_liquid", deepWater, 0.05)
	scatter("reeds", "surface_liquid", shallowNear, 0.30)
	scatter("ground_spots", "ground_spots", landOpen, 0.10)
	scatter("grass", "ground_spots", landOpen, 0.45)
}
