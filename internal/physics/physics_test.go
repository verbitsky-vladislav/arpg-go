package physics

import (
	"testing"

	"github.com/vladislav/game/internal/engine"
)

// sub — сторона под-клетки в тестах: круглое число, чтобы координаты в
// проверках читались как «клетка × 8».
const sub = 8.0

// parse собирает поле из картинки: строка на ряд под-клеток.
//
//	~ глубокая вода   - мелководье   . земля   ^ плато   / лестница   # стена
func parse(t *testing.T, rows ...string) *Field {
	t.Helper()
	w := len(rows[0])
	cells := make([]Cell, 0, w*len(rows))
	for _, r := range rows {
		if len(r) != w {
			t.Fatalf("ряды разной длины: %q", r)
		}
		for _, ch := range r {
			c, ok := map[rune]Cell{
				'~': Deep, '-': Shallow, '.': Ground, '^': Plateau, '/': Ramp, '#': Solid,
			}[ch]
			if !ok {
				t.Fatalf("неизвестный символ %q", ch)
			}
			cells = append(cells, c)
		}
	}
	f := NewField(w, len(rows), sub, cells)
	if f == nil {
		t.Fatal("поле не собралось")
	}
	return f
}

// at — центр под-клетки (sx,sy) в мировых координатах.
func at(sx, sy int) engine.Vec2 {
	return engine.Vec2{X: (float64(sx) + 0.5) * sub, Y: (float64(sy) + 0.5) * sub}
}

var (
	walker = Body{Radius: 3, Floor: FloorLow, Caps: Caps{Wade: true}}
	beast  = Body{Radius: 3, Floor: FloorLow} // в воду не суётся
	duck   = Body{Radius: 3, Floor: FloorLow, Caps: Caps{Wade: true, Swim: true}}
)

func TestPassableByCaps(t *testing.T) {
	f := parse(t, "~-.")
	cases := []struct {
		name string
		b    Body
		want [3]bool // Deep, Shallow, Ground
	}{
		{"герой бродит по мели", walker, [3]bool{false, true, true}},
		{"зверь воду обходит", beast, [3]bool{false, false, true}},
		{"утка плывёт везде", duck, [3]bool{true, true, true}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for i, cell := range []Cell{Deep, Shallow, Ground} {
				if got := f.Passable(cell, c.b); got != c.want[i] {
					t.Errorf("клетка %d: %v, ждали %v", cell, got, c.want[i])
				}
			}
		})
	}
}

// Тело — круг, а не точка: у самой воды стоять нельзя, даже если точка опоры на
// суше. Именно из-за точки звери и висели наполовину в воде.
func TestFitsIsCircleNotPoint(t *testing.T) {
	f := parse(t,
		"...~",
		"...~",
		"...~")
	edge := at(2, 1) // земля вплотную к воде: центр клетки в 4 px от кромки
	if !f.Fits(edge, Body{Radius: 3, Floor: FloorLow}) {
		t.Error("тело радиуса 3 должно вставать в 4 px от воды")
	}
	if f.Fits(edge, Body{Radius: 6, Floor: FloorLow}) {
		t.Error("тело радиуса 6 задевает воду и стоять там не может")
	}
}

func TestMoveSlidesAlongWall(t *testing.T) {
	f := parse(t,
		"..#",
		"..#",
		"..#")
	start := at(1, 1)
	// Ход по диагонали в стену: X упирается, Y должен доехать.
	got, _ := f.Move(start, engine.Vec2{X: 4, Y: 4}, walker)
	if got.X != start.X {
		t.Errorf("X должен упереться в стену: %v", got)
	}
	if got.Y != start.Y+4 {
		t.Errorf("Y должен проехать: %v", got)
	}
}

// Главное правило: на возвышенность нельзя зайти сбоку. Обрыв нарисован только
// с юга, но граница этажей — стена со всех сторон.
func TestPlateauNeedsStairs(t *testing.T) {
	f := parse(t,
		"^^^^",
		"^^^^",
		"....",
		"....")
	pos := at(1, 2)
	got, floor := f.Move(pos, engine.Vec2{X: 0, Y: -4}, walker)
	if got != pos {
		t.Errorf("шаг на плато без лестницы: %v → %v", pos, got)
	}
	if floor != FloorLow {
		t.Errorf("этаж не должен меняться: %d", floor)
	}
}

func TestRampConnectsFloors(t *testing.T) {
	f := parse(t,
		"^^^^^^",
		"^^^^^^",
		"##//##",
		"..//..",
		"......")
	pos := at(2, 4) // низ, под лестницей
	b := walker
	b.Radius = 3
	floor := FloorLow
	for range 40 {
		pos, floor = f.Move(pos, engine.Vec2{X: 0, Y: -1}, Body{Radius: b.Radius, Floor: floor, Caps: b.Caps})
	}
	if floor != FloorHigh {
		t.Fatalf("по лестнице не поднялись: этаж %d, позиция %v", floor, pos)
	}
	if c := f.CellAt(pos); c != Plateau {
		t.Fatalf("оказались не на макушке: клетка %d, позиция %v", c, pos)
	}
	// Спускаться обратно — тоже только по лестнице: шаг с макушки на юг мимо
	// лестницы запрещён (спрыгивать нельзя).
	off := at(0, 1)
	if got, _ := f.Move(off, engine.Vec2{X: 0, Y: 4}, Body{Radius: 3, Floor: FloorHigh}); got != off {
		t.Errorf("прыжок с обрыва: %v → %v", off, got)
	}
}

func TestEscapeFromWall(t *testing.T) {
	f := parse(t,
		"....",
		".##.",
		".##.",
		"....")
	stuck := at(1, 1) // внутри стены
	got, _ := f.Move(stuck, engine.Vec2{}, walker)
	if got == stuck {
		t.Fatal("тело в стене должно выдавливаться наружу")
	}
	// и за несколько тиков выбирается совсем
	pos := got
	for range 30 {
		pos, _ = f.Move(pos, engine.Vec2{}, walker)
	}
	if !f.Fits(pos, walker) {
		t.Errorf("не выбралось за 30 тиков: %v", pos)
	}
}

func TestPlaceFindsNearestSpot(t *testing.T) {
	f := parse(t,
		"~~~~",
		"~~~~",
		"~~..",
		"~~..")
	got, ok := f.Place(at(0, 0), beast, 64)
	if !ok {
		t.Fatal("место на суше есть, но не нашлось")
	}
	if !f.Fits(got, beast) {
		t.Errorf("выбранное место не подходит: %v", got)
	}
	if _, ok := f.Place(at(0, 0), beast, 8); ok {
		t.Error("в радиусе 8 px суши нет, а место нашлось")
	}
}

func TestSeparatePushesApart(t *testing.T) {
	a := engine.Vec2{X: 0, Y: 0}
	b := engine.Vec2{X: 4, Y: 0}
	d, ok := Separate(a, 3, b, 3)
	if !ok {
		t.Fatal("тела пересеклись, а расталкивания нет")
	}
	if got := a.Add(d).Sub(b.Sub(d)).Len(); got < 6-1e-9 {
		t.Errorf("после расталкивания зазор %.3f, ждали 6", got)
	}
	if _, ok := Separate(a, 1, b, 1); ok {
		t.Error("непересекающиеся тела расталкивать нечего")
	}
	if _, ok := Separate(a, 3, a, 3); !ok {
		t.Error("совпавшие тела должны расцепляться")
	}
}

func TestNilFieldLetsEverythingThrough(t *testing.T) {
	var f *Field
	pos, floor := f.Move(engine.Vec2{}, engine.Vec2{X: 5}, walker)
	if pos.X != 5 || floor != FloorLow {
		t.Errorf("без поля тело должно ходить свободно: %v", pos)
	}
	if !f.Fits(engine.Vec2{}, walker) || f.SpeedScale(engine.Vec2{}) != 1 {
		t.Error("без поля не должно быть ни стен, ни замедления")
	}
}

func TestShallowSlowsDown(t *testing.T) {
	f := parse(t, ".-~")
	if s := f.SpeedScale(at(0, 0)); s != 1 {
		t.Errorf("по земле полная скорость, а не %.2f", s)
	}
	if s := f.SpeedScale(at(1, 0)); s != ShallowSpeed {
		t.Errorf("по мели %.2f, ждали %.2f", s, ShallowSpeed)
	}
}

// Поле собирается только из согласованных данных: битую карту лучше поймать на
// загрузке, чем ловить призраков в физике.
func TestNewFieldRejectsBadSize(t *testing.T) {
	if NewField(2, 2, 8, make([]Cell, 3)) != nil {
		t.Error("размер не сошёлся, а поле собралось")
	}
	if NewField(0, 2, 8, nil) != nil {
		t.Error("пустое поле должно быть nil")
	}
}

func TestOutsideIsWater(t *testing.T) {
	f := parse(t, "..", "..")
	if c := f.At(-1, 0); c != Deep {
		t.Errorf("за краем карты ждали воду, получили %d", c)
	}
	if c := f.CellAt(engine.Vec2{X: -1, Y: 1}); c != Deep {
		t.Errorf("слева от карты ждали воду, получили %d", c)
	}
}

// Расталкивание применяется через Move, а не прибавлением к позиции: тело,
// которому некуда отойти, должно остаться на месте, а не уехать в стену.
func TestSeparateThroughMoveKeepsWalls(t *testing.T) {
	f := parse(t,
		"#....",
		"#....",
		"#....")
	pos := at(1, 1) // два тела точно друг в друге у самой стены
	d, ok := Separate(pos, 3, pos, 3)
	if !ok {
		t.Fatal("совпавшие тела должны расталкиваться")
	}
	free, _ := f.Move(pos, d, walker)
	stuck, _ := f.Move(pos, d.Scale(-1), walker)
	if free.X <= pos.X {
		t.Errorf("тело, которому есть куда отойти, не сдвинулось: %v", free)
	}
	if !f.Fits(stuck, walker) {
		t.Errorf("тело вдавили в стену: %v", stuck)
	}
}
