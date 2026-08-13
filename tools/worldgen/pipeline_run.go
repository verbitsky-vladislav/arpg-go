package main

// pipeline_run.go — оркестрация шагов и сборка результата в map_format v1.
// По мере готовности стадий (M3–M6) сюда добавляются вызовы shape/tiles/props.

// Run прогоняет пайплайн генерации одной карты (worldgen.spec §5).
func (g *Generator) Run() {
	// шаги 1-4: рельеф
	g.stageHeight()
	g.stageMoisture()
	g.stageLevels()

	// шаги 5-9: форма суши (CA-сглаживание, мелкие острова, связность).
	g.stageSmooth()
	g.stageIslands()
	g.stageConnectivity()
	g.forceEdgeRing() // §4: гарантировать водяную рамку после сглаживания (E1)

	// Пруды/реки вырезаются в уровнях до автотайла; тропы копятся набором.
	g.stageWater()

	// Возвышенности ставятся ПОСЛЕ воды: место под кусок и под его юбку ищется
	// уже по окончательной суше, поэтому река не может разрезать плато на ленты.
	// Порог по высоте, который насыпал stageLevels, здесь снимается — форма,
	// размер и количество задаются типом куска, а не изолинией шума.
	g.stagePlateauPlace()
	// Срез узких клеток и резервирование юбки влияют друг на друга (юбка режет
	// южные ряды, от этого макушка может стать у́же 4), поэтому крутим до
	// устойчивости.
	for pass := 0; pass < 4; pass++ {
		g.stagePlateauFix()
		g.stagePlateauTerrace()
		before := len(g.Cliff)
		g.stagePlateauApron()
		if pass > 0 && len(g.Cliff) == before {
			break
		}
	}
	// Лестницы врезаются в готовую юбку обрыва: до этой точки g.Cliff ещё
	// переставляется, а лестница обязана лечь на окончательное тело скалы.
	g.stageStairs()
	g.stageTrails()

	// Вид воды считается по окончательной сетке уровней, до автотайлинга суши:
	// полосы глубины у берега + рябь на открытой воде.
	g.stageWaterShade()
	g.stageWaterDetail()

	// Автотайлинг ТОЛЬКО из Ground_grass + Water_coasts, по ручным наборам.
	// Слои: вода (цвет-фон) → mud → grass_ground; берега (grass_water/mud_water,
	// лист Water_coasts) — в разрежённый слой coast.
	//
	// Земля собрана как у автора в forest.tmx: снизу СПЛОШНОЙ mud (в его карте в
	// слое ground у грунта всегда ключ 1,1,1,1), сверху трава, которая вырезает по
	// нему свои переходные тайлы. Очертания тропе задаёт трава, а не грунт —
	// отсюда и травяная кайма вокруг тропы, ровно как берег вокруг воды.
	//
	// Подложка — свойство СУШИ, а не тропы: любая будущая проплешина в траве
	// (поляна, стоянка) сразу получит грунт под собой и кайму по краю. Под
	// сплошной травой подложка не кладётся — её там не видно.
	g.GroundLayer = NewGrid[uint16](g.P.Width, g.P.Height)
	g.MudLayer = NewGrid[uint16](g.P.Width, g.P.Height)
	isLandCell := func(x, y int) bool { return g.Level.In(x, y) && g.Level.At(x, y).isLand() }
	inTrail := func(x, y int) bool { return g.Trail[[2]int{x, y}] }
	// трава обрывается на тропе — иначе кромке неоткуда взяться
	grassCell := func(x, y int) bool { return isLandCell(x, y) && !inTrail(x, y) }
	// грунт: сплошная подложка под травой, берег mud_water
	g.paintUnder(isLandCell, grassCell, "mud", "mud_water", "coast", g.MudLayer)
	// трава: интерьер grass_ground, кромка по краю тропы, берег grass_water
	g.paintLand(grassCell, "ground", "ground", "grass_water", "coast", g.GroundLayer)

	// плато: травяной верх grass_cliff + скальный обрыв на юг (spots_rock),
	// вокруг — тень возвышенности на нижней земле (grass_shadow/mud_shadow)
	g.stagePlateau()
	g.stagePlateauShadow()

	// точка появления, объекты, маркеры. Спавн — отдельная стадия: stageProps
	// выходит сразу, если у биома нет пропсов, и раньше уносила спавн с собой (E5).
	g.stageSpawn()
	g.stageProps()
	g.stageMarkers()
}

// ToMapV1 собирает результат в выходной формат.
func (g *Generator) ToMapV1(seed int64) *MapV1 {
	mp := &MapV1{
		Format:      "map_format v1",
		Biome:       g.Manifest.ID,
		Seed:        seed,
		Width:       g.P.Width,
		Height:      g.P.Height,
		TileSize:    g.Manifest.TileSize,
		Sheets:      g.sheetRefs(),
		WaterColors: g.Manifest.waterPaletteRGB(),
		Layers: Layers{
			Liquid:        denseData(g.LiquidLayer),
			LiquidShade:   shadeData(g.WaterShade),
			Ground:        denseData(g.GroundLayer),
			Mud:           denseData(g.MudLayer),
			Plateau:       denseData(g.PlateauLayer),
			LiquidDetail:  g.Sparse["liquid_detail"],
			GroundSpots:   g.Sparse["ground_spots"],
			Coast:         g.Sparse["coast"],
			SurfaceLiquid: g.Sparse["surface_liquid"],
			PlateauShadow: g.Sparse["plateau_shadow"],
			Cliff:         g.Sparse["cliff"],
			Stairs:        g.Sparse["stairs"],
			Hangers:       g.Sparse["hangers"],
		},
		Props:   g.Props,
		Markers: g.Marks,
		Nav:     g.buildNav(),
	}
	return mp
}

func denseData(g *Grid[uint16]) []uint16 {
	if g == nil {
		return nil
	}
	return g.Data
}

func shadeData(g *Grid[uint8]) []uint8 {
	if g == nil {
		return nil
	}
	return g.Data
}

// sheetRefs — список листов в порядке автора с рассчитанным firstgid.
func (g *Generator) sheetRefs() []SheetRef {
	order := sheetOrder(g.Manifest)
	refs := make([]SheetRef, 0, len(order))
	gid := 1
	for _, name := range order {
		sh := g.Manifest.Sheets[name]
		refs = append(refs, SheetRef{
			Name: name, File: sh.File, Columns: sh.Columns,
			Count: sh.Count, Firstgid: gid,
		})
		gid += sh.Count
	}
	return refs
}

// buildNav — плотная сетка стоимости прохода по уровням (шаг 19).
func (g *Generator) buildNav() NavData {
	cost := make([]uint8, g.P.Width*g.P.Height)
	for i, lv := range g.Level.Data {
		switch lv {
		case LiquidDeep:
			cost[i] = 0 // непроходимо
		case LiquidShallow:
			cost[i] = 160 // ×0.6
		default:
			cost[i] = 255
		}
	}
	// тело обрыва — стена, а не земля: клетки под южной кромкой плато закрыты
	// скалой, ходить по ним нельзя.
	for c := range g.Cliff {
		if g.Level.In(c[0], c[1]) {
			cost[c[1]*g.P.Width+c[0]] = 0
		}
	}
	// лестницы — единственный проход сквозь обрыв, поэтому открываются ПОСЛЕ
	// него: их клетки лежат внутри g.Cliff и иначе остались бы стеной.
	for c := range g.Stair {
		if g.Level.In(c[0], c[1]) {
			cost[c[1]*g.P.Width+c[0]] = 255
		}
	}
	return NavData{Width: g.P.Width, Height: g.P.Height, Cost: cost}
}

// sheetIndexInManifest — индекс листа по имени в порядке автора (для SparseTile.Sheet).
func (g *Generator) sheetIndexInManifest(name string) uint8 {
	for i, n := range sheetOrder(g.Manifest) {
		if n == name {
			return uint8(i)
		}
	}
	return 0
}

// sheetAnim — ссылка на анимацию листа целиком (вода), если она объявлена в
// манифесте. Тайлы такого листа всегда уезжают в вывод анимированными.
func (g *Generator) sheetAnim(sheet string) *AnimRef {
	sh, ok := g.Manifest.Sheets[sheet]
	if !ok || sh.Anim == nil {
		return nil
	}
	return &AnimRef{Frames: sh.Anim.Frames, Stride: sh.Anim.Stride, MS: sh.Anim.MS}
}

// addSparse добавляет клетку в разрежённый слой layer.
func (g *Generator) addSparse(layer, sheet string, x, y, tile int, anim *AnimRef) {
	g.Sparse[layer] = append(g.Sparse[layer], SparseTile{
		X: x, Y: y, Sheet: g.sheetIndexInManifest(sheet), Tile: uint16(tile), Anim: anim,
	})
}
