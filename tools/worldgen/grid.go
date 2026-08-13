package main

// grid.go — сетки и общие операции над ними: уровни высоты, плотные слои
// тайлов, соседи, битовые маски, flood fill. Всё в тайловых координатах.

// Level — классификация клетки по высоте (worldgen.spec §5, таблица порогов).
type Level uint8

const (
	LiquidDeep    Level = iota // < 0.22, непроходимо
	LiquidShallow              // 0.22..0.32, мелководье (камыш)
	Ground                     // 0.32..0.68, нижняя земля
	Plateau                    // > 0.68, верхний уровень, только по лестнице
)

func (l Level) isLiquid() bool { return l == LiquidDeep || l == LiquidShallow }
func (l Level) isLand() bool   { return l == Ground || l == Plateau }

// Grid[T] — прямоугольная сетка row-major. Дженерик, чтобы одинаково хранить
// уровни, float-поля высоты/влажности и id тайлов.
type Grid[T any] struct {
	W, H int
	Data []T
}

func NewGrid[T any](w, h int) *Grid[T] {
	return &Grid[T]{W: w, H: h, Data: make([]T, w*h)}
}

func (g *Grid[T]) In(x, y int) bool { return x >= 0 && y >= 0 && x < g.W && y < g.H }

func (g *Grid[T]) At(x, y int) T { return g.Data[y*g.W+x] }

func (g *Grid[T]) Set(x, y int, v T) { g.Data[y*g.W+x] = v }

// AtOr возвращает значение или def за границей — удобно для соседей у края.
func (g *Grid[T]) AtOr(x, y int, def T) T {
	if !g.In(x, y) {
		return def
	}
	return g.Data[y*g.W+x]
}

// nb8 — смещения 8 соседей по часовой стрелке, начиная с севера.
// Порядок фиксирован и используется как порядок битов в масках.
var nb8 = [8][2]int{
	{0, -1},  // 0 N
	{1, -1},  // 1 NE
	{1, 0},   // 2 E
	{1, 1},   // 3 SE
	{0, 1},   // 4 S
	{-1, 1},  // 5 SW
	{-1, 0},  // 6 W
	{-1, -1}, // 7 NW
}

// nb4 — 4 ортогональных соседа (N,E,S,W) — порядок битов 4-битной Wang-маски.
var nb4 = [4][2]int{
	{0, -1}, // bit0 N
	{1, 0},  // bit1 E
	{0, 1},  // bit2 S
	{-1, 0}, // bit3 W
}

// mask8 — 8-битная маска: бит i выставлен, если сосед i удовлетворяет pred.
// За границей сетки считаем, что предикат истинен (край = «такой же»),
// чтобы автотайл не рисовал обрыв по рамке массива.
func mask8(g *Grid[Level], x, y int, pred func(Level) bool) uint8 {
	var m uint8
	for i, d := range nb8 {
		nx, ny := x+d[0], y+d[1]
		if !g.In(nx, ny) {
			m |= 1 << uint(i)
			continue
		}
		if pred(g.At(nx, ny)) {
			m |= 1 << uint(i)
		}
	}
	return m
}

// --- угловая (corner) схема автотайлинга ---------------------------------
//
// Набор построен по corner-схеме на 16 комбинаций, а не по рёберной blob-47.
// Граница террейна идёт не по рёбрам клеток, а через их центры: тайл клетки
// задаётся четырьмя её углами в порядке NW NE SE SW. Угол «внутри террейна»,
// если из четырёх примыкающих клеток хотя бы ДВЕ ему принадлежат. Так берег
// получается мягким, органическим, без ступенек — рёберная схема так не умеет.

// cornerKey — dual-grid: тайл (x,y) определяется четырьмя клетками, сходящимися
// в его ВЕРХНЕ-ЛЕВОМ углу. Так граница двух террейнов (трава/вода) даёт
// переходный тайл и на ПРЯМОМ ребре, а не только на выступах. Порядок NW NE SE SW,
// угол помечен, если соответствующая клетка принадлежит террейну.
//   NW=(x-1,y-1)  NE=(x,y-1)
//   SW=(x-1,y)    SE=(x,y)
func cornerKey(inSet func(x, y int) bool, x, y int) [4]bool {
	return [4]bool{
		inSet(x-1, y-1), // NW
		inSet(x, y-1),   // NE
		inSet(x, y),     // SE
		inSet(x-1, y),   // SW
	}
}

// cornerKeyStr — строковый ключ угловой комбинации "NW,NE,SE,SW" (1/0),
// совпадает с ключами в манифесте и в разметке .tsx.
func cornerKeyStr(k [4]bool) string {
	b := []byte("0,0,0,0")
	if k[0] {
		b[0] = '1'
	}
	if k[1] {
		b[2] = '1'
	}
	if k[2] {
		b[4] = '1'
	}
	if k[3] {
		b[6] = '1'
	}
	return string(b)
}

// components — метки связных компонент клеток, где sel истинен (4-связность).
// Возвращает grid меток (0 = не входит) и число компонент.
func components(lv *Grid[Level], sel func(Level) bool) (*Grid[int], int) {
	labels := NewGrid[int](lv.W, lv.H)
	next := 0
	stack := make([][2]int, 0, 256)
	for y := 0; y < lv.H; y++ {
		for x := 0; x < lv.W; x++ {
			if !sel(lv.At(x, y)) || labels.At(x, y) != 0 {
				continue
			}
			next++
			labels.Set(x, y, next)
			stack = stack[:0]
			stack = append(stack, [2]int{x, y})
			for len(stack) > 0 {
				c := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				for _, d := range nb4 {
					nx, ny := c[0]+d[0], c[1]+d[1]
					if lv.In(nx, ny) && labels.At(nx, ny) == 0 && sel(lv.At(nx, ny)) {
						labels.Set(nx, ny, next)
						stack = append(stack, [2]int{nx, ny})
					}
				}
			}
		}
	}
	return labels, next
}

// componentSizes — размеры компонент по меткам (index 0 не используется).
func componentSizes(labels *Grid[int], n int) []int {
	sizes := make([]int, n+1)
	for _, v := range labels.Data {
		if v > 0 {
			sizes[v]++
		}
	}
	return sizes
}
