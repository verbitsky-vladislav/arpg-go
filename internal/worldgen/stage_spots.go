package worldgen

// stage_spots.go — напольные пятна (группа штампов ground_spots из decor.json)
// на грунте троп. Кладутся ПОСЛЕ подложки mud/травы, но ДО плато: слой
// ground_spots в стопке лежит над землёй и под плато с его тенью, поэтому
// возвышенность и её обрыв закрывают пятна сами, без отдельной проверки.

import "math/rand"

// groundSpotsCover — доля клеток грунта под пятнами. Пятна не накладываются
// друг на друга (общая карта занятости), так что это и есть плотность.
const groundSpotsCover = 0.45

// stageGroundSpots рассыпает пятна по ЧИСТОМУ грунту троп.
//
// Тайл карты — dual-grid: он показывает четыре клетки вокруг своего верхнего
// левого угла (autotiling.md §2). Подложка mud лежит и под кромочными тайлами,
// где часть клетки — трава; пятно на такой тайл вылезает на траву краем. Поэтому
// зона считается по углам: все четыре клетки тайла обязаны быть тропой без
// травы. Плато и тело обрыва исключены — пятна под ними всё равно не видно.
func (g *Generator) stageGroundSpots() {
	if g.Decor == nil || len(g.Decor.Stamps["ground_spots"]) == 0 || g.MudLayer == nil {
		return
	}
	bareMud := func(x, y int) bool {
		return g.Level.In(x, y) && g.Level.At(x, y) == Ground &&
			g.Trail[[2]int{x, y}] && !g.Cliff[[2]int{x, y}]
	}
	// клетка зоны = тайл, у которого все четыре угловые клетки — голый грунт
	pureMud := func(x, y int) bool {
		k := cornerKey(bareMud, x, y)
		return k[0] && k[1] && k[2] && k[3]
	}
	cells := make([][2]int, 0, 1024)
	for y := 0; y < g.P.Height; y++ {
		for x := 0; x < g.P.Width; x++ {
			if pureMud(x, y) {
				cells = append(cells, [2]int{x, y})
			}
		}
	}
	rng := rand.New(rand.NewSource(int64(g.Seed) ^ 0x5B07))
	g.scatterStampsOn(rng, map[[2]int]bool{}, g.Decor.Stamps["ground_spots"], "ground_spots",
		pureMud, cells, groundSpotsCover)
}
