package main

// pipeline_run.go — оркестрация шагов и сборка результата в map_format v1.
// По мере готовности стадий (M3–M6) сюда добавляются вызовы shape/tiles/props.

// Run прогоняет пайплайн генерации одной карты (worldgen.spec §5).
func (g *Generator) Run() {
	// шаги 1-4: рельеф
	g.stageHeight()
	g.stageMoisture()
	g.stageLevels()

	// шаги 5-9: форма (CA-сглаживание, острова, чистка силуэта возвышенностей,
	// срез узких плато, лестницы, связность). Реализуются в stage_shape.go.
	g.stageSmooth()
	g.stageIslands()
	g.stagePlateauShape()
	g.stagePlateauFix()
	g.stageStairs()
	g.stageConnectivity()
	g.forceEdgeRing() // §4: гарантировать водяную рамку после сглаживания (E1)

	// Пруды/реки вырезаются в уровнях до автотайла; тропы копятся набором.
	// Вода режет только нижнюю землю, плато обходит: раньше река проходила
	// сквозь возвышенность и резала её на ленты в 1-2 клетки уже ПОСЛЕ
	// stagePlateauFix — отсюда и брались узкие плато в отчёте (E4).
	g.stageWater()
	// вода могла отрезать от плато куски — чистим силуэт ещё раз. Срез узких
	// клеток и резервирование юбки влияют друг на друга (юбка срезает южные
	// ряды, от этого плато может стать у́же 4), поэтому крутим до устойчивости.
	for pass := 0; pass < 4; pass++ {
		g.stagePlateauShape()
		g.stagePlateauFix()
		before := len(g.Cliff)
		g.stagePlateauApron()
		if pass > 0 && len(g.Cliff) == before {
			break
		}
	}
	g.stageTrails()

	// Автотайлинг ТОЛЬКО из Ground_grass + Water_coasts, по ручным наборам.
	// Слои: вода (цвет-фон) → grass_underlay → grass_ground → mud_underlay → mud;
	// берега (grass_water/mud_water, лист Water_coasts) — в разрежённый слой coast.
	g.GroundUnderLayer = NewGrid[uint16](g.P.Width, g.P.Height)
	g.GroundLayer = NewGrid[uint16](g.P.Width, g.P.Height)
	g.MudUnderLayer = NewGrid[uint16](g.P.Width, g.P.Height)
	g.MudLayer = NewGrid[uint16](g.P.Width, g.P.Height)
	isLandCell := func(x, y int) bool { return g.Level.In(x, y) && g.Level.At(x, y).isLand() }
	inTrail := func(x, y int) bool { return g.Trail[[2]int{x, y}] }
	// трава: интерьер grass_ground, берег grass_water; подложка grass_underlay
	g.paintLand(isLandCell, "ground_under", "ground", "ground", "grass_water", "coast",
		g.GroundUnderLayer, g.GroundLayer)
	// тропы: интерьер/кромка mud (кромка ложится над травой), берег mud_water; подложка mud_underlay
	g.paintLand(inTrail, "mud_under", "mud", "mud", "mud_water", "coast",
		g.MudUnderLayer, g.MudLayer)

	// плато: травяной верх grass_cliff + скальный обрыв на юг (spots_rock)
	g.stagePlateau()

	// точка появления, объекты, маркеры. Спавн — отдельная стадия: stageProps
	// выходит сразу, если у биома нет пропсов, и раньше уносила спавн с собой (E5).
	g.stageSpawn()
	g.stageProps()
	g.stageMarkers()
}

// ToMapV1 собирает результат в выходной формат.
func (g *Generator) ToMapV1(seed int64) *MapV1 {
	mp := &MapV1{
		Format:   "map_format v1",
		Biome:    g.Manifest.ID,
		Seed:     seed,
		Width:    g.P.Width,
		Height:   g.P.Height,
		TileSize: g.Manifest.TileSize,
		Sheets:   g.sheetRefs(),
		Layers: Layers{
			Liquid:        denseData(g.LiquidLayer),
			GroundUnder:   denseData(g.GroundUnderLayer),
			Ground:        denseData(g.GroundLayer),
			MudUnder:      denseData(g.MudUnderLayer),
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

// addSparse добавляет клетку в разрежённый слой layer.
func (g *Generator) addSparse(layer, sheet string, x, y, tile int, anim *AnimRef) {
	g.Sparse[layer] = append(g.Sparse[layer], SparseTile{
		X: x, Y: y, Sheet: g.sheetIndexInManifest(sheet), Tile: uint16(tile), Anim: anim,
	})
}
