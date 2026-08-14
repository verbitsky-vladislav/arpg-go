package worldgen

// stage_decor.go — кустики травы (группа штампов grass из decor.json, лист
// stairs_grass) по всей травяной поверхности: и по нижней земле, и по макушкам
// плато. Без них трава — ровная заливка одного тона.
//
// Слой ground_decor лежит НАД плато: иначе кустики на макушке закрывались бы
// самим плато, которое рисуется позже нижней земли. На нижнюю землю это не
// влияет — там, где плато нет, его слой пуст.

import "math/rand"

// grassDecorCover — доля травяных тайлов под кустиками. Кустики не наезжают
// друг на друга (общая карта занятости), так что это и есть плотность.
const grassDecorCover = 0.9

// stageGroundDecor рассыпает кустики по ЧИСТОЙ траве.
//
// Зона считается по углам тайла (dual-grid, autotiling.md §2): все четыре
// клетки обязаны быть травой ОДНОГО уровня. Смешанный тайл — это кромка: у
// границы плато на нём стоит переходный тайл, у тропы — травяная кайма, у воды —
// берег, и кустик на таком тайле вылезает краем на чужое покрытие.
func (g *Generator) stageGroundDecor() {
	if g.Decor == nil || len(g.Decor.Stamps["grass"]) == 0 {
		return
	}
	grassCell := func(x, y int) (Level, bool) {
		if !g.Level.In(x, y) {
			return 0, false
		}
		lv := g.Level.At(x, y)
		if !lv.isLand() || g.Trail[[2]int{x, y}] || g.Cliff[[2]int{x, y}] {
			return 0, false // вода, грунт тропы или тело обрыва — не трава
		}
		return lv, true
	}
	pureGrass := func(x, y int) bool {
		var want Level
		for i, c := range [4][2]int{{x - 1, y - 1}, {x, y - 1}, {x - 1, y}, {x, y}} {
			lv, ok := grassCell(c[0], c[1])
			if !ok || (i > 0 && lv != want) {
				return false
			}
			want = lv
		}
		return true
	}
	cells := make([][2]int, 0, g.P.Width*g.P.Height/2)
	for y := 0; y <= g.P.Height; y++ {
		for x := 0; x <= g.P.Width; x++ {
			if pureGrass(x, y) {
				cells = append(cells, [2]int{x, y})
			}
		}
	}
	rng := rand.New(rand.NewSource(int64(g.Seed) ^ 0x6DEC0))
	g.scatterStampsOn(rng, map[[2]int]bool{}, g.Decor.Stamps["grass"], "ground_decor",
		pureGrass, cells, grassDecorCover)
}
