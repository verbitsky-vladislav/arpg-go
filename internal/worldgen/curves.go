package worldgen

// curves.go — конвейер извилистых кривых для рек и троп (порт из generate_map.py):
//   A* по 8 соседям  →  меандр  →  сглаживание Чайкина  →  круглая кисть.
// Меандр обязателен: A* даёт почти прямую, а Чайкин умеет только скруглить углы,
// которых нет. Кисть даёт непрерывность линии по построению. Всё в тайловых
// координатах; zone — предикат допустимой клетки, cost — стоимость шага.

import (
	"container/heap"
	"math"
)

type fpt struct{ x, y float64 }

// curveFBM — 3-октавный шум для меандра/стоимости, центрированный в [-0.5,0.5].
var curveFBM = fbmParams{Octaves: 3, Freq: 1, Lacunarity: 2, Gain: 0.5}

func cnoise(x, y float64, seed uint64) float64 {
	return fbm(x, y, seed, curveFBM) - 0.5
}

// --- A* -------------------------------------------------------------------

type astarNode struct {
	x, y int
	f    float64
	idx  int
}
type astarHeap []*astarNode

func (h astarHeap) Len() int           { return len(h) }
func (h astarHeap) Less(i, j int) bool { return h[i].f < h[j].f }
func (h astarHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i]; h[i].idx = i; h[j].idx = j }
func (h *astarHeap) Push(x any)        { n := x.(*astarNode); n.idx = len(*h); *h = append(*h, n) }
func (h *astarHeap) Pop() any {
	old := *h
	n := len(old)
	it := old[n-1]
	old[n-1] = nil
	*h = old[:n-1]
	return it
}

var astar8 = [8][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {1, -1}, {-1, 1}, {-1, -1}}

// astar ищет путь от a к b по 8 соседям в пределах zone. cost(n) — цена входа
// в клетку; диагональ дороже в 1.41 раза. Возвращает путь как список клеток
// (включая концы) или nil, если пути нет.
func astar(ax, ay, bx, by int, zone func(x, y int) bool, cost func(x, y int) float64) [][2]int {
	type key = [2]int
	g := map[key]float64{{ax, ay}: 0}
	par := map[key]key{}
	closed := map[key]bool{}
	h := &astarHeap{{x: ax, y: ay, f: 0}}
	heap.Init(h)
	for h.Len() > 0 {
		cur := heap.Pop(h).(*astarNode)
		ck := key{cur.x, cur.y}
		if cur.x == bx && cur.y == by {
			break
		}
		if closed[ck] {
			continue
		}
		closed[ck] = true
		for _, d := range astar8 {
			nx, ny := cur.x+d[0], cur.y+d[1]
			if !zone(nx, ny) {
				continue
			}
			step := cost(nx, ny)
			if d[0] != 0 && d[1] != 0 {
				step *= 1.41
			}
			ng := g[ck] + step
			nk := key{nx, ny}
			if old, ok := g[nk]; !ok || ng < old {
				g[nk] = ng
				par[nk] = ck
				fscore := ng + math.Hypot(float64(nx-bx), float64(ny-by))
				heap.Push(h, &astarNode{x: nx, y: ny, f: fscore})
			}
		}
	}
	if _, ok := par[[2]int{bx, by}]; !ok && !(ax == bx && ay == by) {
		return nil
	}
	// восстановить путь
	path := [][2]int{{bx, by}}
	cur := key{bx, by}
	for cur != (key{ax, ay}) {
		p, ok := par[cur]
		if !ok {
			return nil
		}
		cur = p
		path = append(path, cur)
	}
	// развернуть
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}

// --- преобразования кривой -------------------------------------------------

// simplify прореживает путь, оставляя каждую s-ю точку (плюс концы).
func simplify(p [][2]int, s int) []fpt {
	out := make([]fpt, 0, len(p)/s+2)
	if len(p) <= 2 {
		for _, q := range p {
			out = append(out, fpt{float64(q[0]), float64(q[1])})
		}
		return out
	}
	out = append(out, fpt{float64(p[0][0]), float64(p[0][1])})
	for i := s; i < len(p)-1; i += s {
		out = append(out, fpt{float64(p[i][0]), float64(p[i][1])})
	}
	out = append(out, fpt{float64(p[len(p)-1][0]), float64(p[len(p)-1][1])})
	return out
}

// meander сдвигает внутренние точки перпендикулярно ходу на величину из шума —
// иначе A* даёт почти прямую, которую Чайкину нечем изгибать.
func meander(pts []fpt, amp float64, seed uint64) []fpt {
	if len(pts) < 3 {
		return pts
	}
	out := make([]fpt, 0, len(pts))
	out = append(out, pts[0])
	for i := 1; i < len(pts)-1; i++ {
		a, b := pts[i-1], pts[i+1]
		dx, dy := b.x-a.x, b.y-a.y
		L := math.Hypot(dx, dy)
		if L == 0 {
			L = 1
		}
		d := amp * cnoise(float64(i)*0.38, 0, seed) * 2.4
		out = append(out, fpt{pts[i].x - dy/L*d, pts[i].y + dx/L*d})
	}
	out = append(out, pts[len(pts)-1])
	return out
}

// chaikin — сглаживание срезанием углов, it итераций (обычно 4).
func chaikin(p []fpt, it int) []fpt {
	for ; it > 0; it-- {
		if len(p) < 3 {
			break
		}
		out := make([]fpt, 0, len(p)*2)
		out = append(out, p[0])
		for i := 0; i+1 < len(p); i++ {
			a, b := p[i], p[i+1]
			out = append(out, fpt{a.x*0.75 + b.x*0.25, a.y*0.75 + b.y*0.25})
			out = append(out, fpt{a.x*0.25 + b.x*0.75, a.y*0.25 + b.y*0.75})
		}
		out = append(out, p[len(p)-1])
		p = out
	}
	return p
}

// brushBand растеризует кривую полосой РОВНО в 2 клетки: каждая точка кривой
// кладёт блок 2×2 — четыре клетки, сходящиеся в ближайшем к ней узле сетки.
// Круглая кисть так не умеет: brush центрирует диск на клетке, поэтому её след
// всегда нечётной ширины (1, 3, 5...), а «диск радиуса ~0.6» даёт то 1 клетку,
// то 2 — в зависимости от того, куда легла точка.
// Блоки соседних точек всегда перекрываются хотя бы одной клеткой, так что
// полоса непрерывна и 4-связна: на dual-grid она рисуется сплошной лентой без
// «бус» из ключа 1,0,1,0, который получается на голой диагонали.
// Клетки вне zone отбрасываются — у самой воды полоса может стать у́же.
func brushBand(pts []fpt, zone func(x, y int) bool) map[[2]int]bool {
	cells := map[[2]int]bool{}
	if len(pts) < 2 {
		return cells
	}
	for i := 0; i+1 < len(pts); i++ {
		a, b := pts[i], pts[i+1]
		n := int(math.Hypot(b.x-a.x, b.y-a.y) * 3)
		if n < 1 {
			n = 1
		}
		for k := 0; k <= n; k++ {
			t := float64(k) / float64(n)
			cx := a.x + (b.x-a.x)*t
			cy := a.y + (b.y-a.y)*t
			// узел сетки между клетками gx,gx+1 и gy,gy+1 — ближайший к точке
			gx := int(math.Round(cx - 0.5))
			gy := int(math.Round(cy - 0.5))
			for dy := 0; dy <= 1; dy++ {
				for dx := 0; dx <= 1; dx++ {
					if zone(gx+dx, gy+dy) {
						cells[[2]int{gx + dx, gy + dy}] = true
					}
				}
			}
		}
	}
	fill4(cells, zone)
	return cells
}

// fill4 достраивает след до 4-связного: чисто диагональный стык на dual-grid
// даёт ключ «1,0,1,0» (два противоположных угла), и лента рассыпается в бусы.
// Целая полоса brushBand такого стыка не даёт — страховка нужна там, где zone
// срезал часть блока у самой воды.
// Решение по каждой паре принимается по ИСХОДНОМУ набору, поэтому от порядка
// обхода map результат не зависит.
func fill4(cells map[[2]int]bool, zone func(x, y int) bool) {
	var add [][2]int
	for c := range cells {
		// двух диагоналей достаточно: каждая пара разбирается один раз, слева
		for _, d := range [2][2]int{{1, 1}, {1, -1}} {
			if !cells[[2]int{c[0] + d[0], c[1] + d[1]}] {
				continue
			}
			a := [2]int{c[0] + d[0], c[1]}
			b := [2]int{c[0], c[1] + d[1]}
			if cells[a] || cells[b] {
				continue // ступенька уже есть
			}
			switch {
			case zone(a[0], a[1]):
				add = append(add, a)
			case zone(b[0], b[1]):
				add = append(add, b)
			}
		}
	}
	for _, c := range add {
		cells[c] = true
	}
}

// brush растеризует кривую круглой кистью: для каждого сегмента шагаем с шагом
// в треть клетки и заливаем диск радиуса rad(t). Линия непрерывна по построению.
// В результат попадают только клетки zone.
func brush(pts []fpt, rad func(t float64) float64, zone func(x, y int) bool) map[[2]int]bool {
	cells := map[[2]int]bool{}
	if len(pts) < 2 {
		return cells
	}
	for i := 0; i+1 < len(pts); i++ {
		a, b := pts[i], pts[i+1]
		n := int(math.Hypot(b.x-a.x, b.y-a.y) * 3)
		if n < 1 {
			n = 1
		}
		for k := 0; k <= n; k++ {
			t := float64(k) / float64(n)
			cx := a.x + (b.x-a.x)*t
			cy := a.y + (b.y-a.y)*t
			r := rad(float64(i) / math.Max(1, float64(len(pts)-1)))
			ri := int(math.Ceil(r))
			for dx := -ri; dx <= ri; dx++ {
				for dy := -ri; dy <= ri; dy++ {
					if float64(dx*dx+dy*dy) <= r*r+0.25 {
						q := [2]int{int(math.Round(cx)) + dx, int(math.Round(cy)) + dy}
						if zone(q[0], q[1]) {
							cells[q] = true
						}
					}
				}
			}
		}
	}
	return cells
}
