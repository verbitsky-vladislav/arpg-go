package worldgen

// bodygraph.go — связность карты ТЕЛОМ, а не по клеткам.
//
// Клеточная связность врёт: перешеек шириной в одну клетку (16 px) её
// обеспечивает, а тело героя (radius 7, спавн ищется под 8) через него не
// проходит. Замер по сидам 1..40: на восьми картах есть область, куда клеточный
// обход доходит, а игрок — нет.
//
// Поэтому и переправы, и точка появления считаются по одной и той же мере: сетка
// nav (та самая, что уезжает в игру) заворачивается в physics.Field, и области
// размечаются обходом по physics.Fits. «Проходимо» в генераторе и в движке
// обязано означать одно и то же.

import (
	"github.com/vladislav/game/internal/engine"
	"github.com/vladislav/game/internal/physics"
)

// bodyGraph — разметка карты по областям, проходимым телом.
type bodyGraph struct {
	labels *Grid[int]  // клетка → id области (0 = тело сюда не помещается)
	sizes  map[int]int // id → размер области в клетках
}

// bodyComponents размечает области, по которым способно ходить тело радиуса r.
//
// Считается по под-клеткам, а результат сводится к клеткам: клетка (x,y)
// нарисована под-клеткой (x*scale + scale/2, ...) — тот же сдвиг dual-grid,
// который запечён в buildNav.
func (g *Generator) bodyComponents(r float64) *bodyGraph {
	n := g.buildNav()
	if n.Width == 0 || n.Scale == 0 {
		return nil
	}
	cells := make([]physics.Cell, len(n.Cells))
	for i, c := range n.Cells {
		cells[i] = physics.Cell(c)
	}
	sub := float64(g.Manifest.TileSize) / float64(n.Scale)
	f := physics.NewField(n.Width, n.Height, sub, cells)
	body := physics.Body{Radius: r}
	fits := func(sx, sy int) bool {
		return f.Fits(engine.Vec2{X: (float64(sx) + 0.5) * sub, Y: (float64(sy) + 0.5) * sub}, body)
	}

	lab := make([]int, n.Width*n.Height)
	next := 0
	for i := range lab {
		if lab[i] != 0 || !fits(i%n.Width, i/n.Width) {
			continue
		}
		next++
		lab[i] = next
		stack := []int{i}
		for len(stack) > 0 {
			p := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			px, py := p%n.Width, p/n.Width
			for _, d := range nb4 {
				nx, ny := px+d[0], py+d[1]
				if nx < 0 || ny < 0 || nx >= n.Width || ny >= n.Height {
					continue
				}
				if q := ny*n.Width + nx; lab[q] == 0 && fits(nx, ny) {
					lab[q] = next
					stack = append(stack, q)
				}
			}
		}
	}
	if next == 0 {
		return nil
	}

	bg := &bodyGraph{labels: NewGrid[int](g.P.Width, g.P.Height), sizes: map[int]int{}}
	half := n.Scale / 2
	for y := 0; y < g.P.Height; y++ {
		for x := 0; x < g.P.Width; x++ {
			sx, sy := x*n.Scale+half, y*n.Scale+half
			if sx >= n.Width || sy >= n.Height {
				continue
			}
			if v := lab[sy*n.Width+sx]; v != 0 {
				bg.labels.Set(x, y, v)
				bg.sizes[v]++
			}
		}
	}
	return bg
}

// bodyLabelNear — id области у клетки (x,y) или у ближайшей клетки в радиусе
// reach. Проба в упор не годится: у самой кромки воды и скалы тело не
// помещается, и клетка берега, к которой пристаёт переправа, почти всегда даёт
// 0. На этом ошиблись обе сессии, меряя проходимость.
//
// (sx,sy) — полуплоскость поиска: берутся только клетки, лежащие по эту сторону
// от (x,y). Без ограничения проба перепрыгивает узкий пролив и возвращает метку
// ПРОТИВОПОЛОЖНОГО берега: пролив бывает у́же самого reach, и тогда оба конца
// переправы выглядят как одна область, а место отбраковывается. Нулевой вектор —
// искать во все стороны.
func (bg *bodyGraph) bodyLabelNear(x, y, reach, sx, sy int) int {
	if bg == nil {
		return 0
	}
	for r := 0; r <= reach; r++ {
		for dy := -r; dy <= r; dy++ {
			for dx := -r; dx <= r; dx++ {
				if absInt(dx) != r && absInt(dy) != r {
					continue // кольцо радиуса r, внутренность уже просмотрена
				}
				if dx*sx+dy*sy < 0 {
					continue // не та сторона от пролива
				}
				if bg.labels.In(x+dx, y+dy) {
					if v := bg.labels.At(x+dx, y+dy); v != 0 {
						return v
					}
				}
			}
		}
	}
	return 0
}

// biggest — id самой крупной области (0, если областей нет).
func (bg *bodyGraph) biggest() int {
	if bg == nil {
		return 0
	}
	best, bestN := 0, -1
	for id, sz := range bg.sizes {
		if sz > bestN || (sz == bestN && id < best) {
			best, bestN = id, sz
		}
	}
	return best
}

// significant — области от доли frac общей проходимой площади. Мелкие пятачки
// связывать незачем: игрок их не заметит, а переправа к каждому превратит реку
// в решето.
func (bg *bodyGraph) significant(frac float64) map[int]bool {
	out := map[int]bool{}
	if bg == nil {
		return out
	}
	total := 0
	for _, sz := range bg.sizes {
		total += sz
	}
	for id, sz := range bg.sizes {
		if float64(sz) >= float64(total)*frac {
			out[id] = true
		}
	}
	return out
}
