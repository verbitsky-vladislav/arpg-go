package main

// pipeline.go — Generator держит все промежуточные поля генерации одной карты
// и прогоняет шаги пайплайна (worldgen.spec §5) по порядку. Один Generator =
// одна карта = один seed.

import "math"

// Params — настройки генерации, общие для всех сидов одного прогона.
type Params struct {
	Width, Height int
	EdgeMode      string  // "water" | "wall"
	EdgeFalloff   float64 // тайлов, §4 (EDGE_FALLOFF)
	EdgeBorder    int     // ширина принудительной рамки, §4
	FBM           fbmParams
	Warp          float64 // сила domain warp
}

func defaultParams(w, h int) Params {
	return Params{
		Width:       w,
		Height:      h,
		EdgeMode:    "water",
		EdgeFalloff: 20,
		EdgeBorder:  12,
		FBM:         defaultFBM(),
		Warp:        18,
	}
}

// Generator — состояние генерации одной карты.
type Generator struct {
	P        Params
	Seed     uint64
	Manifest *Manifest

	Height   *Grid[float64] // h_final после edge falloff
	Moisture *Grid[float64]
	Level    *Grid[Level]

	// плотные слои тайлов (локальные id листа, 0 = пусто)
	LiquidLayer      *Grid[uint16]
	GroundUnderLayer *Grid[uint16] // grass_overlay, рисуется под GroundLayer
	GroundLayer      *Grid[uint16] // grass-блок
	MudUnderLayer    *Grid[uint16] // mud_overlay, рисуется под MudLayer поверх травы
	MudLayer         *Grid[uint16] // mud-блок (грунтовые тропы)
	PlateauLayer     *Grid[uint16]

	Trail map[[2]int]bool // клетки грунтовых троп (роль mud)
	Decor *DecorLib      // библиотека штампов декора (decor.json)

	// разрежённые слои и сущности
	Sparse map[string][]SparseTile
	Props  []PropInst
	Marks  []Marker
	spawn  [2]int // точка появления игрока (в тайлах)
}

func NewGenerator(p Params, seed uint64, m *Manifest) *Generator {
	return &Generator{
		P:        p,
		Seed:     seed,
		Manifest: m,
		Decor:    loadDecor(m.dir),
		Sparse:   map[string][]SparseTile{},
	}
}

// Пороги уровней (worldgen.spec §5, таблица).
const (
	thDeep    = 0.22
	thShallow = 0.32
	thPlateau = 0.68
)

// stageHeight — шаги 1-2: fBm + domain warp, затем принудительный спад высоты
// у края (§4), чтобы карта закрывалась рамкой воды/стены.
func (g *Generator) stageHeight() {
	g.Height = NewGrid[float64](g.P.Width, g.P.Height)
	for y := 0; y < g.P.Height; y++ {
		for x := 0; x < g.P.Width; x++ {
			h := fbmWarped(float64(x), float64(y), g.Seed, g.P.FBM, g.P.Warp)
			h = g.applyEdge(x, y, h)
			g.Height.Set(x, y, h)
		}
	}
}

// applyEdge — формула спада высоты у границы (§4):
// edge = distance_to_border / EDGE_FALLOFF; h -= (1-clamp(edge))^2 * 1.5
func (g *Generator) applyEdge(x, y int, h float64) float64 {
	dx := math.Min(float64(x), float64(g.P.Width-1-x))
	dy := math.Min(float64(y), float64(g.P.Height-1-y))
	d := math.Min(dx, dy)
	edge := d / g.P.EdgeFalloff
	if edge > 1 {
		edge = 1
	}
	k := 1 - edge
	return h - k*k*1.5
}

// stageMoisture — шаг 3: независимое поле влажности (для выбора ground_a/b).
func (g *Generator) stageMoisture() {
	g.Moisture = NewGrid[float64](g.P.Width, g.P.Height)
	mp := g.P.FBM
	mp.Freq = g.P.FBM.Freq * 0.75
	for y := 0; y < g.P.Height; y++ {
		for x := 0; x < g.P.Width; x++ {
			m := fbmWarped(float64(x), float64(y), g.Seed^0x9E3779B9, mp, g.P.Warp*0.5)
			g.Moisture.Set(x, y, m)
		}
	}
}

// stageLevels — шаг 4: классификация по порогам высоты + жёсткое закрытие края
// (§4): во внешней рамке шириной EdgeBorder суша принудительно становится водой,
// чтобы игрок не видел обрыв массива (E1). Внутренняя граница рамки остаётся
// органичной за счёт спада высоты из applyEdge.
func (g *Generator) stageLevels() {
	g.Level = NewGrid[Level](g.P.Width, g.P.Height)
	b := g.P.EdgeBorder
	for y := 0; y < g.P.Height; y++ {
		for x := 0; x < g.P.Width; x++ {
			h := g.Height.At(x, y)
			var lv Level
			switch {
			case h < thDeep:
				lv = LiquidDeep
			case h < thShallow:
				lv = LiquidShallow
			case h < thPlateau:
				lv = Ground
			default:
				lv = Plateau
			}
			if x < b || y < b || x >= g.P.Width-b || y >= g.P.Height-b {
				lv = LiquidDeep
			}
			g.Level.Set(x, y, lv)
		}
	}
}

// forceEdgeRing повторно закрывает внешнюю рамку водой (сглаживание/острова
// могли занести туда сушу). Вызывается после шагов формы (E1).
func (g *Generator) forceEdgeRing() {
	b := g.P.EdgeBorder
	for y := 0; y < g.P.Height; y++ {
		for x := 0; x < g.P.Width; x++ {
			if x < b || y < b || x >= g.P.Width-b || y >= g.P.Height-b {
				g.Level.Set(x, y, LiquidDeep)
			}
		}
	}
}
