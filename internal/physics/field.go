// Package physics — проходимость мира и тела в нём: единственное место, где
// живёт ответ на вопрос «можно ли сюда шагнуть».
//
// Мир плоский только на вид. Клетка поля несёт не «проходимо/нет», а ЧТО там:
// вода, земля, макушка возвышенности, лестница, стена. Из этого и из
// возможностей тела (Caps) выводится проходимость — и она разная для героя,
// утки и кабана.
//
// Этажей два: низ (вода, земля) и макушка плато. Между ними нельзя шагнуть
// напрямую — только через Ramp (лестницу), которая принадлежит обоим. Поэтому
// на возвышенность нельзя забраться сбоку, даже там, где обрыв не нарисован:
// граница этажей сама по себе стена.
//
// Сетка поля мельче тайла (см. worldgen: под-клетка = полтайла) и сдвинута
// вместе с артом: тайл dual-grid описывает область вокруг своего верхне-левого
// угла, поэтому клетка уровней рисуется на полтайла правее и ниже своего
// индекса. Сдвиг запечён в поле генератором — здесь координаты уже прямые:
// под-клетка (sx,sy) занимает пиксели [sx*Sub, sx*Sub+Sub).
package physics

import (
	"math"

	"github.com/vladislav/game/internal/engine"
)

// Cell — что в под-клетке поля.
type Cell uint8

const (
	Deep    Cell = iota // глубокая вода: только вплавь
	Shallow             // мелководье: вброд, с замедлением
	Ground              // нижняя земля
	Plateau             // макушка возвышенности — верхний этаж
	Ramp                // лестница: единственная связка этажей
	Solid               // тело обрыва, пропс — стена для всех
)

// Этажи. FloorAny — «тело сейчас на связке»: на лестнице проходимы клетки обоих
// этажей, иначе с неё некуда сойти (круг тела всегда задевает и низ, и верх).
const (
	FloorLow  uint8 = 0
	FloorHigh uint8 = 1
	FloorAny  uint8 = 255
)

// ShallowSpeed — во сколько раз медленнее ход по мелководью.
const ShallowSpeed = 0.6

// Floor — этаж клетки.
func (c Cell) Floor() uint8 {
	if c == Plateau {
		return FloorHigh
	}
	return FloorLow
}

// onFloor — лежит ли клетка на этаже f. Лестница лежит на обоих, и тело на
// лестнице (FloorAny) принимает любую клетку.
func (c Cell) onFloor(f uint8) bool {
	return c == Ramp || f == FloorAny || c.Floor() == f
}

// Liquid — вода ли это (для выбора походки: плыть или идти).
func (c Cell) Liquid() bool { return c == Deep || c == Shallow }

// Caps — что тело умеет. Всё, чем герой отличается от утки в смысле физики.
type Caps struct {
	Wade bool // заходит в мелководье (герой — да, олень — нет)
	Swim bool // плавает по глубокой воде
}

// Body — тело в поле: круг радиуса Radius на этаже Floor.
type Body struct {
	Radius float64
	Floor  uint8
	Caps   Caps
}

// Field — сетка проходимости мира.
type Field struct {
	w, h  int     // размер в под-клетках
	sub   float64 // сторона под-клетки в пикселях
	cells []Cell
}

// NewField собирает поле из плотной сетки под-клеток (row-major, w*h).
// Возвращает nil при несогласованных размерах: поле nil означает «физики нет»,
// и все методы это переживают — просмотрщикам и тестам не нужна карта.
func NewField(w, h int, sub float64, cells []Cell) *Field {
	if w <= 0 || h <= 0 || sub <= 0 || len(cells) != w*h {
		return nil
	}
	return &Field{w: w, h: h, sub: sub, cells: cells}
}

// Sub — сторона под-клетки в пикселях.
func (f *Field) Sub() float64 {
	if f == nil {
		return 1
	}
	return f.sub
}

// Size — размеры поля в под-клетках.
func (f *Field) Size() (int, int) {
	if f == nil {
		return 0, 0
	}
	return f.w, f.h
}

// At — содержимое под-клетки. За границей поля — глубокая вода: карту
// обрамляет водяное кольцо (worldgen E1), и край мира ведёт себя как её берег.
func (f *Field) At(sx, sy int) Cell {
	if f == nil {
		return Ground
	}
	if sx < 0 || sy < 0 || sx >= f.w || sy >= f.h {
		return Deep
	}
	return f.cells[sy*f.w+sx]
}

// CellAt — содержимое под-клетки под мировой точкой.
func (f *Field) CellAt(p engine.Vec2) Cell {
	if f == nil {
		return Ground
	}
	return f.At(f.sx(p.X), f.sx(p.Y))
}

// sx — индекс под-клетки по координате. Отрицательные координаты должны уходить
// в -1, а не в 0, иначе полоса слева от карты считается первой клеткой.
func (f *Field) sx(v float64) int { return int(math.Floor(v / f.sub)) }

// Passable — проходима ли клетка для тела b.
func (f *Field) Passable(c Cell, b Body) bool {
	switch c {
	case Solid:
		return false
	case Deep:
		return b.Caps.Swim && c.onFloor(b.Floor)
	case Shallow:
		return (b.Caps.Wade || b.Caps.Swim) && c.onFloor(b.Floor)
	}
	return c.onFloor(b.Floor)
}

// Fits — помещается ли тело в точке p целиком: круг не задевает ни одной
// непроходимой под-клетки. Проверяется именно круг, а не точка опоры, — иначе
// зверь наполовину висит в воде или в скале.
func (f *Field) Fits(p engine.Vec2, b Body) bool {
	if f == nil {
		return true
	}
	r := math.Max(b.Radius, 0.5)
	x0, x1 := f.sx(p.X-r), f.sx(p.X+r)
	y0, y1 := f.sx(p.Y-r), f.sx(p.Y+r)
	for sy := y0; sy <= y1; sy++ {
		for sx := x0; sx <= x1; sx++ {
			if f.Passable(f.At(sx, sy), b) {
				continue
			}
			if circleHitsCell(p, r, float64(sx)*f.sub, float64(sy)*f.sub, f.sub) {
				return false
			}
		}
	}
	return true
}

// circleHitsCell — пересекает ли круг (c,r) квадрат со стороной s и левым
// верхним углом (x,y). Касание не считается пересечением: тело должно иметь
// право встать вплотную к стене.
func circleHitsCell(c engine.Vec2, r, x, y, s float64) bool {
	nx := math.Min(math.Max(c.X, x), x+s)
	ny := math.Min(math.Max(c.Y, y), y+s)
	dx, dy := c.X-nx, c.Y-ny
	return dx*dx+dy*dy < r*r
}

// SpeedScale — множитель скорости в точке: мелководье вязкое, остальное нет.
func (f *Field) SpeedScale(p engine.Vec2) float64 {
	if f.CellAt(p) == Shallow {
		return ShallowSpeed
	}
	return 1
}
