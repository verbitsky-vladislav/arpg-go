package main

// pipeline.go — Generator держит все промежуточные поля генерации одной карты
// и прогоняет шаги пайплайна (worldgen.spec §5) по порядку. Один Generator =
// одна карта = один seed.

import (
	"math"
	"sort"
)

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
	Cliff map[[2]int]bool // клетки тела обрыва (юбка под южной кромкой плато)
	Decor *DecorLib       // библиотека штампов декора (decor.json)

	// cornerMiss — сколько раз угловой набор не имел точного ключа и пришлось
	// брать ближайший по Хеммингу. Ненулевое значение = дыра в разметке, из-за
	// которой на стыке встаёт чужой тайл (проверка E13).
	cornerMiss map[string]int

	// разрежённые слои и сущности
	Sparse   map[string][]SparseTile
	Props    []PropInst
	Marks    []Marker
	spawn    [2]int // точка появления игрока (в тайлах)
	hasSpawn bool
}

func NewGenerator(p Params, seed uint64, m *Manifest) *Generator {
	return &Generator{
		P:          p,
		Seed:       seed,
		Manifest:   m,
		Decor:      loadDecor(m.dir),
		Sparse:     map[string][]SparseTile{},
		Cliff:      map[[2]int]bool{},
		cornerMiss: map[string]int{},
	}
}

// Пороги уровней (worldgen.spec §5, таблица). Рабочие пороги считаются под сид
// (landThreshold/plateauThreshold), эти константы остались как запасные значения
// для вырожденных случаев и как исходный зазор «мель ↔ глубина».
const (
	thDeep    = 0.22
	thShallow = 0.32
	thPlateau = 0.72
)

// usableArea — площадь, по которой имеет смысл считать долю суши. Островная
// маска высоты (applyEdge) держит сушу внутри круга R=min(W,H)/2, поэтому доля
// от ВСЕГО массива физически не может превысить π/4≈78.5%, и порог 45..70 от
// массива был недостижим по построению. Считаем от площади круга.
func (g *Generator) usableArea() float64 {
	if g.P.EdgeMode == "wall" {
		return float64(g.P.Width * g.P.Height)
	}
	R := math.Min(float64(g.P.Width), float64(g.P.Height)) / 2
	return math.Pi * R * R
}

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

// applyEdge — радиальная островная маска: суша концентрируется в круге от
// центра карты, а высота плавно спадает к кромке острова, поэтому силуэт выходит
// округлым, а не квадратным. edge = (R - dist_to_center) / EDGE_FALLOFF —
// сколько тайлов «вглубь» от берега; h -= (1-clamp(edge))^2 * 1.5.
func (g *Generator) applyEdge(x, y int, h float64) float64 {
	cx := float64(g.P.Width-1) / 2
	cy := float64(g.P.Height-1) / 2
	R := math.Min(float64(g.P.Width), float64(g.P.Height)) / 2
	dc := math.Hypot(float64(x)-cx, float64(y)-cy)
	edge := (R - dc) / g.P.EdgeFalloff
	if edge > 1 {
		edge = 1
	} else if edge < 0 {
		edge = 0
	}
	k := 1 - edge
	return h - k*k*1.5
}

// landTargetShare — целевая доля суши от площади острова (середина коридора E6
// 45..70%). Как и с плато, фиксированный порог высоты давал лотерею: у одного
// сида остров занимал 45% круга, у другого 70%. Квантиль держит долю на месте,
// а форму берега по-прежнему целиком определяет шум.
const landTargetShare = 0.68

// landThreshold — порог «вода/суша» под конкретный сид: квантиль поля высоты по
// клеткам внутри острова (круг R=min(W,H)/2 минус рамка). Возвращает порог мели;
// глубина идёт ниже с тем же зазором, что был между константами thShallow/thDeep.
func (g *Generator) landThreshold() (shallow, deep float64) {
	R := math.Min(float64(g.P.Width), float64(g.P.Height)) / 2
	cx, cy := float64(g.P.Width-1)/2, float64(g.P.Height-1)/2
	b := g.P.EdgeBorder
	vals := make([]float64, 0, g.P.Width*g.P.Height)
	for y := b; y < g.P.Height-b; y++ {
		for x := b; x < g.P.Width-b; x++ {
			if math.Hypot(float64(x)-cx, float64(y)-cy) <= R {
				vals = append(vals, g.Height.At(x, y))
			}
		}
	}
	if len(vals) < 16 {
		return thShallow, thDeep
	}
	sort.Float64s(vals)
	idx := int(float64(len(vals)) * (1 - landTargetShare))
	if idx >= len(vals) {
		idx = len(vals) - 1
	}
	shallow = vals[idx]
	return shallow, shallow - (thShallow - thDeep)
}

// plateauTargetShare — какую долю СУШИ отдать под возвышенности до чистки формы.
// Взято с запасом к целевому коридору E7 (10..30%): морфология и резервирование
// юбки под обрыв съедают часть клеток.
const plateauTargetShare = 0.22

// plateauThreshold — порог высоты для плато, подобранный ПОД КОНКРЕТНЫЙ СИД как
// квантиль поля высоты по клеткам суши. Фиксированная константа давала разброс
// от 5% до 36% плато между сидами: у одного шума хвост распределения толще, у
// другого тоньше. Квантиль убирает эту лотерею — доля возвышенностей стабильна,
// а форма всё так же полностью определяется шумом.
func (g *Generator) plateauThreshold(shallow float64) float64 {
	land := make([]float64, 0, g.P.Width*g.P.Height/2)
	b := g.P.EdgeBorder
	for y := b; y < g.P.Height-b; y++ {
		for x := b; x < g.P.Width-b; x++ {
			if h := g.Height.At(x, y); h >= shallow {
				land = append(land, h)
			}
		}
	}
	if len(land) < 16 {
		return thPlateau
	}
	sort.Float64s(land)
	idx := int(float64(len(land)) * (1 - plateauTargetShare))
	if idx >= len(land) {
		idx = len(land) - 1
	}
	return math.Max(land[idx], shallow+0.05)
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
	shallow, deep := g.landThreshold()
	th := g.plateauThreshold(shallow)
	for y := 0; y < g.P.Height; y++ {
		for x := 0; x < g.P.Width; x++ {
			h := g.Height.At(x, y)
			var lv Level
			switch {
			case h < deep:
				lv = LiquidDeep
			case h < shallow:
				lv = LiquidShallow
			case h < th:
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
