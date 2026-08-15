package worldgen

// pipeline_run.go — оркестрация шагов и сборка результата в map_format v1.
// По мере готовности стадий (M3–M6) сюда добавляются вызовы shape/tiles/props.

import "github.com/vladislav/game/internal/physics"

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
	// Броды ставятся ДО автотайлинга: чтобы лечь ровно, переправа имеет право
	// подсыпать берег на клетку, а такую правку суши обязан увидеть автотайл.
	g.stageBridges()

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

	// пятна на грунте троп — по готовой подложке mud, но до плато: в стопке слоёв
	// ground_spots лежит под плато и его тенью, и они закрывают пятна сами.
	g.stageGroundSpots()

	// плато: травяной верх grass_cliff + скальный обрыв на юг (spots_rock),
	// вокруг — тень возвышенности на нижней земле (grass_shadow/mud_shadow)
	g.stagePlateau()
	g.stagePlateauShadow()

	// кувшинки и камыш — по готовой сетке уровней и полосам глубины
	g.stageSurface()

	// кустики травы — после плато: слой ground_decor лежит над ним, иначе
	// макушка закрыла бы собой свои же кустики
	g.stageGroundDecor()
	g.stageHangers() // лианы — поверх готовой стены обрыва и врезанных в неё лестниц

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
		Rev:         MapRev,
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
			GroundDecor:   g.Sparse["ground_decor"],
			Coast:         g.Sparse["coast"],
			Bridges:       g.Sparse["bridges"],
			SurfaceLiquid: g.Sparse["surface_liquid"],
			PlateauShadow: g.Sparse["plateau_shadow"],
			Cliff:         g.Sparse["cliff"],
			Stairs:        g.Sparse["stairs"],
			Hangers:       g.Sparse["hangers"],
		},
		Props:   g.sortedProps(),
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

// navScale — под-клеток физики на тайл.
//
// Четыре, то есть под-клетка равна четверти тайла (4 px). Для рельефа хватало и
// двух: тайл dual-grid собран из четырёх четвертей, и мельче половины тайла
// данных о поверхности всё равно нет. Но объекты задают барьер не по клеткам, а
// по замеру спрайта, и на сетке в 8 px он получался блочным: любая задетая
// под-клетка становится стеной целиком, и вокруг ствола нарастал квадрат заметно
// шире самого ствола — игрок упирался там, где на вид проходил. На 4 px ошибка
// вчетверо меньше. Сетка рельефа от этого не врёт: четверти тайла просто
// дублируются по две.
const navScale = 4

// buildNav — сетка физики в под-клетках (шаг 19).
//
// Здесь же снимается главный перекос старой версии: сетка уровней и картинка
// сдвинуты друг относительно друга на полтайла. Тайл в позиции (x,y) описывает
// не клетку (x,y), а область вокруг своего верхне-левого угла — четыре клетки
// (x-1,y-1)…(x,y). Поэтому содержимое клетки (x,y) НАРИСОВАНО в квадрате,
// сдвинутом на полтайла вправо и вниз, а физика, читавшая клетку как p/tile,
// проверяла соседнюю: у воды это давало полтайла «хождения по воде» на каждом
// берегу.
//
// Сдвиг запекается сюда, в данные: под-клетка (sx,sy) берёт содержимое клетки
// ((sx-1)/2, (sy-1)/2) — то есть той, которая в этом месте нарисована. Дальше
// движок читает поле напрямую, без поправок.
func (g *Generator) buildNav() NavData {
	sw, sh := g.P.Width*navScale, g.P.Height*navScale
	cells := make([]uint8, sw*sh)
	for sy := 0; sy < sh; sy++ {
		y := (sy - navScale/2) / navScale
		if sy < navScale/2 {
			y = -1 // полоса перед первой клеткой: за картой, там вода
		}
		for sx := 0; sx < sw; sx++ {
			x := (sx - navScale/2) / navScale
			if sx < navScale/2 {
				x = -1
			}
			cells[sy*sw+sx] = uint8(g.navCell(x, y))
		}
	}
	g.markPropCells(cells, sw, sh)
	return NavData{Width: sw, Height: sh, Scale: navScale, Cells: cells}
}

// navCell — что физика видит в клетке уровней (x,y).
func (g *Generator) navCell(x, y int) physics.Cell {
	if !g.Level.In(x, y) {
		return physics.Deep // за краем карты — вода, как и в водяном кольце
	}
	switch {
	// Лестница проверяется ПЕРВОЙ: её клетки лежат внутри тела обрыва и иначе
	// остались бы стеной. Это единственный проход между этажами.
	case g.Stair[[2]int{x, y}]:
		return physics.Ramp
	case g.Cliff[[2]int{x, y}]:
		return physics.Solid // тело обрыва: скала, а не земля
	// Брод — камни, положенные НА воду: уровень под ним так и остаётся водой и
	// водой же рисуется, поэтому проверять его надо до веток воды. Этаж нижний
	// у обоих берегов, связка этажей тут ни при чём — это Ground, не Ramp.
	case g.Bridge[[2]int{x, y}]:
		return physics.Ground
	}
	switch g.Level.At(x, y) {
	case LiquidDeep:
		return physics.Deep
	case LiquidShallow:
		return physics.Shallow
	case Plateau:
		return physics.Plateau
	}
	return physics.Ground
}

// markPropCells закрывает клетку под якорем пропса, объявленного непроходимым.
//
// Габарит футпринта для этого не годится: у дерева w×h — это размер картинки в
// тайлах (крона в 8 тайлов), а мешает пройти только ствол. Якорь стоит ровно
// под ним — его клетку и закрываем. Если у объектов появится собственный
// габарит основания, читать надо будет его.
//
// Лестницы и вода не перекрываются никогда: пропс, севший на единственный
// подъём, отрезал бы макушку плато от мира, а «стена посреди пруда» смысла не
// имеет — по воде и так ходят только вплавь.
func (g *Generator) markPropCells(cells []uint8, sw, sh int) {
	for _, p := range g.Props {
		if !p.Collides {
			continue
		}
		x, y := p.X+p.Anchor[0], p.Y+p.Anchor[1]-1
		// Брод — проход, а не земля: пропс на его камнях закрыл бы переправу,
		// как закрыл бы лестницу.
		if g.Bridge[[2]int{x, y}] {
			continue
		}
		// Тело кладётся ПО ЗАМЕРУ спрайта, а не одной клеткой. Клетка — это 16 px
		// на любой объект, и сквозь пень в 28 px игрок проходил краем, а мимо
		// поваленного ствола в 72 px — и вовсе насквозь.
		//
		// Форма — приплюснутый овал: смотрим на мир сверху под углом, и то, чем
		// объект занимает ЗЕМЛЮ, по вертикали короче, чем по горизонтали. Центр —
		// середина нарисованной клетки якоря, то есть основание спрайта.
		// Точка опоры объекта в мире. Рендер ставит спрайт так, что опора его
		// РИСУНКА (p.Base) попадает сюда же, поэтому горизонтальной поправки не
		// нужно: смещение рисунка внутри холста уже учтено при отрисовке.
		ts := g.Manifest.TileSize
		sub := float64(ts) / navScale
		baseX := float64(x*ts + ts)        // центр нарисованной клетки
		baseY := float64(y*ts + ts + ts/2) // её низ — линия, по которой объект стоит
		rx := float64(p.Body) / 2
		if rx < sub {
			rx = sub // минимум — прежняя клетка под якорем
		}
		ry := rx / 2
		if ry < sub {
			ry = sub
		}
		// Растеризуем с запасом ВНУТРЬ: под-клетка становится стеной, только если
		// её центр попал в овал, ужатый на полклетки. Иначе барьер systematically
		// шире объекта — задетая краем клетка перекрывается целиком, и игрок
		// упирается в воздух рядом со стволом. Лучше отдать пару пикселей внутрь
		// ствола, чем забрать их снаружи.
		irx, iry := rx-sub/2, ry-sub/2
		if irx < sub/2 {
			irx = sub / 2
		}
		if iry < sub/2 {
			iry = sub / 2
		}
		// Овал прижат НИЗОМ к линии опоры: ниже нарисованного объект землю не
		// занимает. Раньше он центрировался по клетке и половиной свисал ниже
		// рисунка — у лежащего бревна это было заметнее всего.
		cxp, cyp := baseX, baseY-ry
		s0, s1 := int((cxp-rx)/sub), int((cxp+rx)/sub)
		t0, t1 := int((cyp-ry)/sub), int((cyp+ry)/sub)
		for s := s0; s <= s1; s++ {
			for t := t0; t <= t1; t++ {
				if s < 0 || t < 0 || s >= sw || t >= sh {
					continue
				}
				// центр под-клетки внутри ужатого овала?
				dx := (float64(s)+0.5)*sub - cxp
				dy := (float64(t)+0.5)*sub - cyp
				if dx*dx/(irx*irx)+dy*dy/(iry*iry) > 1 {
					continue
				}
				switch physics.Cell(cells[t*sw+s]) {
				case physics.Ground, physics.Plateau:
					cells[t*sw+s] = uint8(physics.Solid)
				}
			}
		}
	}
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
	g.addSparseRot(layer, sheet, x, y, tile, 0, anim)
}

// addSparseRot — то же, но с поворотом тайла на rot четвертей по часовой.
func (g *Generator) addSparseRot(layer, sheet string, x, y, tile int, rot uint8, anim *AnimRef) {
	g.Sparse[layer] = append(g.Sparse[layer], SparseTile{
		X: x, Y: y, Sheet: g.sheetIndexInManifest(sheet), Tile: uint16(tile),
		Rot: rot % 4, Anim: anim,
	})
}
