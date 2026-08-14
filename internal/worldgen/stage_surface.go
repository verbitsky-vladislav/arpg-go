package worldgen

// stage_surface.go — то, что растёт из воды: кувшинки (группа lilies, лист
// water_lilis) и камыш (группа reeds, лист Objects). Оба уезжают в слой
// surface_liquid, он лежит над берегом и под тенью возвышенности.
//
// Зоны разные и не пересекаются по смыслу:
//
//	камыш    — только LiquidShallow, то есть та полоса, где герой бредёт вброд:
//	           камыш растёт со дна, ему нужно мелко;
//	kувшинки — вся светлая вода (полоса мели у берега + пруды и реки целиком,
//	           g.WaterShade == shadeShallow). В открытом море кувшинок не бывает.
//
// Камыш раскладывается ПЕРВЫМ: его штампы крупнее (до 3×3), и на узкой полосе
// мелководья ему нужно занять место до того, как туда сядут кувшинки.

import "math/rand"

// Доли зоны под растительностью. Держатся низкими: вода — это ещё и путь, и
// сплошной ковёр кувшинок читается как суша.
const (
	reedCover = 0.10
	lilyCover = 0.08
)

// stageSurface — шаг 17: кувшинки на воде, камыш на мелководье.
func (g *Generator) stageSurface() {
	if g.Decor == nil {
		return
	}
	// Углы тайла проверяются целиком: на смешанном тайле стоит береговой тайл,
	// и растение вылезло бы из воды на сушу (autotiling.md §2).
	allCorners := func(pred func(x, y int) bool) func(x, y int) bool {
		return func(x, y int) bool {
			k := cornerKey(pred, x, y)
			return k[0] && k[1] && k[2] && k[3]
		}
	}
	// клетки брода — уже занятая вода: растение на камне переправы выглядит как
	// растение на дороге
	free := func(x, y int) bool { return !g.Bridge[[2]int{x, y}] }
	shallowCell := func(x, y int) bool {
		return g.Level.In(x, y) && g.Level.At(x, y) == LiquidShallow && free(x, y)
	}
	liquidCell := func(x, y int) bool {
		return g.Level.In(x, y) && g.Level.At(x, y).isLiquid() && free(x, y)
	}
	onLiquid := allCorners(liquidCell)
	// светлая вода: мель у берега плюс вся внутренняя вода — её же считает
	// stageWaterShade, второй раз выводить незачем
	light := func(x, y int) bool {
		return g.WaterShade != nil && g.WaterShade.In(x, y) &&
			g.WaterShade.At(x, y) == shadeShallow
	}
	lightWater := func(x, y int) bool { return onLiquid(x, y) && light(x, y) }
	// Камышу мало уровня LiquidShallow: уровень берётся от порога высоты, а цвет
	// воды — от расстояния до суши, и на широкой отмели вдали от берега они
	// расходятся. Камыш там оказывался посреди воды, покрашенной как открытая, —
	// на картинке это «камыш на глубине», сколько бы ни говорила сетка уровней.
	// Поэтому нужны оба признака сразу: и мелко по уровню, и мелко по виду.
	onShallow := func(x, y int) bool { return allCorners(shallowCell)(x, y) && light(x, y) }

	collect := func(pred func(x, y int) bool) [][2]int {
		out := make([][2]int, 0, 512)
		for y := 0; y <= g.P.Height; y++ {
			for x := 0; x <= g.P.Width; x++ {
				if pred(x, y) {
					out = append(out, [2]int{x, y})
				}
			}
		}
		return out
	}

	rng := rand.New(rand.NewSource(int64(g.Seed) ^ 0x11115))
	occ := map[[2]int]bool{} // общая: кувшинка не садится в камыш и наоборот
	g.scatterStampsOn(rng, occ, g.Decor.Stamps["reeds"], "surface_liquid",
		onShallow, collect(onShallow), reedCover)
	g.scatterStampsOn(rng, occ, g.Decor.Stamps["lilies"], "surface_liquid",
		lightWater, collect(lightWater), lilyCover)
}
