package worldgen

// stage_water_shade.go — вид воды: полосы глубины у берега и рябь на поверхности.
//
// Вода — единственное, что генератор красит сам (в наборах нет сплошного водного
// тайла, см. autotiling.md §5). Раньше это была одна ровная заливка на всю карту,
// и берег читался только по кромке травы. Здесь добавляются две вещи:
//
//  1. ГЛУБИНА. Полоса воды у берега светлее открытой воды, вся внутренняя вода
//     (пруды и реки) — светлая целиком: мелко по определению. Полоса считается по
//     расстоянию до суши, её граница размыта шумом, иначе вокруг острова идёт
//     механический контур-обводка.
//  2. РЯБЬ. Штампы `water_detail` из decor.json (лист water_detilazation) —
//     светлые блики на воде, снятые с карты художника. Кладутся только на
//     полностью водяные тайлы, чтобы не лезть под береговые тайлы слоя coast.

import "math/rand"

// Полосы глубины в тайлах от берега: до ~2.4 клеток — самая светлая вода, до
// ~5.2 — промежуточная, дальше открытая вода последним цветом water_colors.
// waterShadeNoise — амплитуда сдвига границы в тайлах (держать заметно меньше
// ширины полосы), waterDistMax обрезает расчёт расстояния — глубже уже ничего
// не меняется.
const (
	waterShadeShallow = 2.4
	waterShadeMid     = 5.2
	waterShadeNoise   = 1.1
	waterDistMax      = 12
)

// Индексы полос глубины: 1 — мель/внутренняя вода, дальше глубже. 0 = тайл не
// водяной (на карте не встречается: полоса считается для ВСЕХ тайлов, суша
// просто получает мель — под травой её всё равно не видно, зато на кромке из-под
// берегового тайла светит мелководье, а не открытая вода).
const (
	shadeShallow uint8 = 1
	shadeMid     uint8 = 2
	shadeDeep    uint8 = 3
)

// stageWaterShade заполняет g.WaterShade — полосу глубины на КАЖДЫЙ тайл.
//
// Считается в два приёма: сперва полоса по клеткам сетки уровней, затем перенос
// на тайлы по dual-grid (тайл (x,y) описывает четвёрку клеток, сходящихся в его
// верхне-левом углу, autotiling.md §2). Берётся минимум — то есть самая мелкая
// из четырёх: тайл на кромке рисуется как мель, и цвет под береговым тайлом
// совпадает с водой рядом с ним.
func (g *Generator) stageWaterShade() {
	W, H := g.P.Width, g.P.Height
	dist := g.waterDistance()
	inner := g.innerWater()

	// непрерывная «глубина» в тайлах: расстояние до суши, сдвинутое шумом
	depth := NewGrid[float64](W, H)
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			// две октавы шума в сумме дают [-1,1]: крупная гуляет полосой вдоль
			// берега, мелкая рвёт её кромку по клеткам — без второй граница
			// читается как ступенчатый контур-обводка острова. Амплитуду держать
			// меньше ширины полосы, иначе в мелком заливе вылезают отдельные
			// пятна открытой воды.
			n := 1.3*cnoise(float64(x)*0.14, float64(y)*0.14, g.Seed^0x9A7EC0DE) +
				0.7*cnoise(float64(x)*0.33, float64(y)*0.33, g.Seed^0x5177E)
			depth.Set(x, y, distTiles(dist, x, y)+waterShadeNoise*n)
		}
	}
	// Размытие поля ДО порога, а не чистка полос после. Шум значений кусочно
	// плоский, и порог по нему режет уровни прямоугольниками по решётке шума;
	// после размытия граница полосы идёт кривой, а одиночные пятна пропадают.
	blurField(depth, 2)

	cell := NewGrid[uint8](W, H)
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			switch {
			case !g.Level.At(x, y).isLiquid() || inner.At(x, y):
				cell.Set(x, y, shadeShallow)
			case depth.At(x, y) <= waterShadeShallow:
				cell.Set(x, y, shadeShallow)
			case depth.At(x, y) <= waterShadeMid:
				cell.Set(x, y, shadeMid)
			default:
				cell.Set(x, y, shadeDeep)
			}
		}
	}

	despeckleShade(cell)

	g.WaterShade = NewGrid[uint8](W, H)
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			s := shadeDeep
			for _, c := range [4][2]int{{x - 1, y - 1}, {x, y - 1}, {x - 1, y}, {x, y}} {
				if cell.In(c[0], c[1]) {
					if v := cell.At(c[0], c[1]); v < s {
						s = v
					}
				}
			}
			g.WaterShade.Set(x, y, s)
		}
	}
}

// blurField — n проходов усреднения 3×3 по полю (края берут ближайшую клетку).
func blurField(f *Grid[float64], passes int) {
	tmp := make([]float64, len(f.Data))
	for p := 0; p < passes; p++ {
		for y := 0; y < f.H; y++ {
			for x := 0; x < f.W; x++ {
				sum := 0.0
				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						sum += f.At(clampInt(x+dx, 0, f.W-1), clampInt(y+dy, 0, f.H-1))
					}
				}
				tmp[y*f.W+x] = sum / 9
			}
		}
		copy(f.Data, tmp)
	}
}

// despeckleShade убирает одиночные клетки чужой полосы: клетка, у которой все
// четыре ортогональных соседа одной ДРУГОЙ полосы, забирает их значение. Такие
// «пиксели» вылезают там, где шум переваливает порог ровно на одной клетке, и
// читаются как грязь на воде, а не как глубина.
func despeckleShade(cell *Grid[uint8]) {
	src := make([]uint8, len(cell.Data))
	copy(src, cell.Data)
	at := func(x, y int) (uint8, bool) {
		if !cell.In(x, y) {
			return 0, false
		}
		return src[y*cell.W+x], true
	}
	for y := 0; y < cell.H; y++ {
		for x := 0; x < cell.W; x++ {
			v := src[y*cell.W+x]
			var same uint8
			n := 0
			for _, o := range nb4 {
				nv, ok := at(x+o[0], y+o[1])
				if !ok || nv == v {
					n = -1
					break
				}
				if n == 0 {
					same = nv
				} else if nv != same {
					n = -1
					break
				}
				n++
			}
			if n > 0 {
				cell.Set(x, y, same)
			}
		}
	}
}

// Веса чамфера 3-4: шаг по стороне стоит 3, по диагонали 4 (≈√2·3). Расстояние
// Чебышёва (1 и 1) давало вокруг острова квадратно-диагональную кайму — контур
// полосы заметно спрямлялся по осям и под 45°. Хранится в третях тайла.
const (
	chamferOrtho = 3
	chamferDiag  = 4
)

// distTiles — значение поля waterDistance в тайлах.
func distTiles(d *Grid[int], x, y int) float64 {
	return float64(d.At(x, y)) / chamferOrtho
}

// waterDistance — приближённое евклидово расстояние от клетки воды до ближайшей
// суши (у суши 0) в третях тайла, обрезанное waterDistMax. Два прохода чамфера,
// как в trailClearance; край массива сушей НЕ считается — за рамкой открытое
// море, и светлая кайма по периметру карты была бы враньём.
func (g *Generator) waterDistance() *Grid[int] {
	W, H := g.P.Width, g.P.Height
	max := waterDistMax * chamferOrtho
	d := NewGrid[int](W, H)
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			if g.Level.At(x, y).isLiquid() {
				d.Set(x, y, max)
			}
		}
	}
	relax := func(x, y int, offs [][3]int) {
		if d.At(x, y) == 0 {
			return
		}
		best := d.At(x, y)
		for _, o := range offs {
			nx, ny := x+o[0], y+o[1]
			if !d.In(nx, ny) {
				continue // за краем карты — та же вода, а не берег
			}
			if v := d.At(nx, ny) + o[2]; v < best {
				best = v
			}
		}
		d.Set(x, y, best)
	}
	fwd := [][3]int{
		{-1, -1, chamferDiag}, {0, -1, chamferOrtho}, {1, -1, chamferDiag}, {-1, 0, chamferOrtho},
	}
	bwd := [][3]int{
		{1, 1, chamferDiag}, {0, 1, chamferOrtho}, {-1, 1, chamferDiag}, {1, 0, chamferOrtho},
	}
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			relax(x, y, fwd)
		}
	}
	for y := H - 1; y >= 0; y-- {
		for x := W - 1; x >= 0; x-- {
			relax(x, y, bwd)
		}
	}
	return d
}

// innerWater — вода, НЕ связанная с открытым морем: пруды и реки внутри острова.
// Море — компонента, куда входит рамка карты (её вода гарантирована
// forceEdgeRing). Всё остальное мелкое по смыслу, поэтому светлое целиком, без
// оглядки на расстояние до берега: центр крупного пруда иначе уходил бы в цвет
// открытого моря.
func (g *Generator) innerWater() *Grid[bool] {
	W, H := g.P.Width, g.P.Height
	sea := NewGrid[bool](W, H)
	stack := make([][2]int, 0, 256)
	push := func(x, y int) {
		if g.Level.In(x, y) && !sea.At(x, y) && g.Level.At(x, y).isLiquid() {
			sea.Set(x, y, true)
			stack = append(stack, [2]int{x, y})
		}
	}
	for x := 0; x < W; x++ {
		push(x, 0)
		push(x, H-1)
	}
	for y := 0; y < H; y++ {
		push(0, y)
		push(W-1, y)
	}
	for len(stack) > 0 {
		c := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, o := range nb4 {
			push(c[0]+o[0], c[1]+o[1])
		}
	}
	inner := NewGrid[bool](W, H)
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			inner.Set(x, y, g.Level.At(x, y).isLiquid() && !sea.At(x, y))
		}
	}
	return inner
}

// waterDetailCover — доля водяной поверхности под рябью (замерено по карте
// художника: блики покрывают около четверти воды).
const waterDetailCover = 0.24

// waterDetailFill — сколько водяной поверхности добирается второй проходкой,
// мелкими штампами. Идёт поверх waterDetailCover и попадает в основном в узкие
// места, куда крупные полосы не влезли: свободных клеток в открытом море после
// первой проходки почти не остаётся.
const waterDetailFill = 0.10

// stageWaterDetail раскладывает штампы ряби по воде. Штамп кладётся только туда,
// где ВСЕ четыре клетки его dual-grid четвёрки — вода: на кромке тайл занят
// берегом (слой coast рисуется выше), и блик под ним просто пропал бы.
func (g *Generator) stageWaterDetail() {
	if g.Decor == nil || len(g.Decor.Stamps["water_detail"]) == 0 {
		return
	}
	// За краем массива — то же открытое море, а не берег (та же конвенция, что в
	// waterDistance). Иначе весь периметр карты читается как узкая вода.
	liquid := func(x, y int) bool { return !g.Level.In(x, y) || g.Level.At(x, y).isLiquid() }
	openWater := func(x, y int) bool {
		for _, c := range [4][2]int{{x - 1, y - 1}, {x, y - 1}, {x - 1, y}, {x, y}} {
			if !liquid(c[0], c[1]) {
				return false
			}
		}
		return true
	}
	rng := rand.New(rand.NewSource(int64(g.Seed) ^ 0x5EA0F0A))
	occ := map[[2]int]bool{}
	g.scatterStamps(rng, occ, "water_detail", "liquid_detail", openWater, waterDetailCover)

	// Вторая проходка — та же зона, но МЕЛКИМИ штампами и по списку клеток.
	// Первая берёт штамп из всей библиотеки, а там полосы до 17 тайлов: в протоку
	// или залив шириной в пару тайлов не влезает ни одна, и узкая вода оставалась
	// неподвижной. Зона не расширяется ни на клетку: рябь имеет право лежать
	// только на тайле, где ВСЯ четвёрка — вода. На кромочный тайл она не заходит,
	// поэтому на берег не наезжает и поверх берегового тайла в игре не рисуется.
	cells := make([][2]int, 0, 1024)
	for y := 0; y < g.P.Height; y++ {
		for x := 0; x < g.P.Width; x++ {
			if openWater(x, y) {
				cells = append(cells, [2]int{x, y})
			}
		}
	}
	g.scatterStampsOn(rng, occ, stampsUpTo(g.Decor.Stamps["water_detail"], 3, 2),
		"liquid_detail", openWater, cells, waterDetailFill)

	// Третья проходка — по водоёмам, которым не досталось ничего. В библиотеке
	// все штампы горизонтальные и длиной от двух тайлов, а у реки шириной в пару
	// тайлов полоса подходящих клеток вертикальная и шириной в одну: туда не
	// встаёт ни один штамп, и внутренняя вода стояла неподвижной, пока море
	// рябило. Море под условие не попадает — там покрытие и так выше порога.
	g.fillQuietWater(rng, occ, openWater, cells)
}

// quietWaterCover — до какой доли клеток доводится рябь в водоёме, где её почти
// нет. Сопоставимо с тем, что общие проходки дают морю.
const quietWaterCover = 0.28

// fillQuietWater добавляет одиночные капли ряби в водоёмы, обойдённые общими
// проходками: каждый замкнутый водоём считается отдельно, и добирается только
// тот, что остался ниже порога.
func (g *Generator) fillQuietWater(rng *rand.Rand, occ map[[2]int]bool,
	pred func(x, y int) bool, cells [][2]int) {
	singles := singleStamps(stampsUpTo(g.Decor.Stamps["water_detail"], 3, 1))
	if len(singles) == 0 {
		return
	}
	in := make(map[[2]int]bool, len(cells))
	for _, c := range cells {
		in[c] = true
	}
	seen := make(map[[2]int]bool, len(cells))
	for _, start := range cells {
		if seen[start] {
			continue
		}
		body := [][2]int{start}
		seen[start] = true
		for i := 0; i < len(body); i++ {
			c := body[i]
			for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				n := [2]int{c[0] + d[0], c[1] + d[1]}
				if in[n] && !seen[n] {
					seen[n] = true
					body = append(body, n)
				}
			}
		}
		covered := 0
		for _, c := range body {
			if occ[c] {
				covered++
			}
		}
		have := float64(covered) / float64(len(body))
		if have >= quietWaterCover {
			continue
		}
		need := quietWaterCover - have
		// У лужи в пару клеток доля округлилась бы в ноль, и она осталась бы
		// мёртвой: просим ровно одну каплю.
		if int(float64(len(body))*need) == 0 {
			need = 1 / float64(len(body))
		}
		g.scatterStampsOn(rng, occ, singles, "liquid_detail", pred, body, need)
	}
}

// singleStamps режет штампы на одиночные тайлы: в узкую воду встаёт только одна
// клетка, а каждый такой тайл и сам по себе читается как рябь (проверено на
// арте: это мягкие мазки без обрубленных краёв). Новый арт не нужен.
func singleStamps(lib []Stamp) []Stamp {
	type key struct {
		sheet string
		tile  int
	}
	seen := map[key]bool{}
	var out []Stamp
	for _, s := range lib {
		for _, c := range s.Cells {
			k := key{c.Sheet, c.Tile}
			if seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, Stamp{W: 1, H: 1, Cells: []StampCell{{Dx: 0, Dy: 0, Sheet: c.Sheet, Tile: c.Tile}}})
		}
	}
	return out
}

// stampsUpTo — штампы не крупнее w×h тайлов.
func stampsUpTo(lib []Stamp, w, h int) []Stamp {
	out := make([]Stamp, 0, len(lib))
	for _, s := range lib {
		if s.W <= w && s.H <= h {
			out = append(out, s)
		}
	}
	return out
}
