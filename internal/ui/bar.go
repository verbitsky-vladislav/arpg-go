package ui

// Полосы состояния из пака ui/bars: рамка плюс заливка, обрезаемая по доле.
//
// Разметка (что здесь рамка, где внутри неё лежит заливка) задана руками в
// bars.json и не выводится из пикселей: на листе рамка и её заливка нарисованы
// вплотную, поэтому автоматическая нарезка iconnorm склеивает их в один кусок.
//
// Стиль выбирается игроком, поэтому реестр здесь пакетный, как ShowFPS и
// Cursor: экран настроек и HUD берут его из одного места, не таская загрузчик
// через все сцены.

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"io/fs"
	"path"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/vladislav/game/internal/assets"
)

// BarStyle — один вид полосы.
type BarStyle struct {
	ID    string  `json:"id"`
	Title string  `json:"title"`
	Frame [4]int  `json:"frame"`          // рамка на листе: x,y,w,h
	Fill  *[4]int `json:"fill,omitempty"` // спрайт заливки на листе
	At    [2]int  `json:"at"`             // куда заливка ложится внутри рамки
	Slot  *[4]int `json:"slot,omitempty"` // прямоугольник под сплошную заливку
	Color string  `json:"color,omitempty"`

	col color.RGBA
}

// Bars — реестр полос: лист и стили к нему.
type Bars struct {
	Version int         `json:"version"`
	Sheet   string      `json:"sheet"`
	Default string      `json:"default"`
	Styles  []*BarStyle `json:"styles"`

	img  *ebiten.Image
	byID map[string]*BarStyle
}

var bars *Bars

// InitBars читает разметку полос из каталога dir. Ошибка не смертельна:
// без полос HUD рисует запасную прямоугольную, поэтому вызывающий может
// ограничиться предупреждением.
func InitBars(l *assets.Loader, dir string) error {
	b, err := fs.ReadFile(l.FS(), path.Join(dir, "bars.json"))
	if err != nil {
		return fmt.Errorf("ui: чтение разметки полос: %w", err)
	}
	var r Bars
	if err := json.Unmarshal(b, &r); err != nil {
		return fmt.Errorf("ui: разбор разметки полос: %w", err)
	}
	img, err := l.Image(path.Join(dir, r.Sheet))
	if err != nil {
		return fmt.Errorf("ui: лист полос: %w", err)
	}
	r.img, r.byID = img, map[string]*BarStyle{}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	for _, s := range r.Styles {
		if !inSheet(s.Frame, w, h) {
			return fmt.Errorf("ui: полоса %q: рамка вылезает за лист", s.ID)
		}
		if s.Fill != nil && !inSheet(*s.Fill, w, h) {
			return fmt.Errorf("ui: полоса %q: заливка вылезает за лист", s.ID)
		}
		if s.Fill == nil && s.Slot == nil {
			return fmt.Errorf("ui: полоса %q: нет ни заливки, ни слота", s.ID)
		}
		s.col = parseHex(s.Color, color.RGBA{0xc8, 0x38, 0x2e, 0xff})
		r.byID[s.ID] = s
	}
	if len(r.Styles) == 0 {
		return fmt.Errorf("ui: в разметке полос нет ни одного стиля")
	}
	if _, ok := r.byID[r.Default]; !ok {
		r.Default = r.Styles[0].ID
	}
	bars = &r
	return nil
}

func inSheet(r [4]int, w, h int) bool {
	return r[2] > 0 && r[3] > 0 && r[0] >= 0 && r[1] >= 0 && r[0]+r[2] <= w && r[1]+r[3] <= h
}

// parseHex разбирает "#rrggbb"; при любой неудаче отдаёт запасной цвет.
func parseHex(s string, def color.RGBA) color.RGBA {
	if len(s) != 7 || s[0] != '#' {
		return def
	}
	v, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil {
		return def
	}
	return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 0xff}
}

// BarIDs — стили в порядке объявления (пусто, если полосы не загрузились).
func BarIDs() []string {
	if bars == nil {
		return nil
	}
	out := make([]string, 0, len(bars.Styles))
	for _, s := range bars.Styles {
		out = append(out, s.ID)
	}
	return out
}

// BarStyleOf — стиль по id; неизвестный id (или пустой, как у свежих настроек)
// разрешается в стиль по умолчанию.
func BarStyleOf(id string) *BarStyle {
	if bars == nil {
		return nil
	}
	if s, ok := bars.byID[id]; ok {
		return s
	}
	return bars.byID[bars.Default]
}

// NextBarID — следующий стиль по кругу. Перебор живёт здесь, потому что
// «текущий» — это не то, что записано в настройках: пустое и неизвестное
// значение показывается стилем по умолчанию, и шагать надо от него, иначе
// первое переключение выглядит как ничего не сделавшее.
func NextBarID(cur string) string {
	s := BarStyleOf(cur)
	if s == nil {
		return cur
	}
	ids := BarIDs()
	for i, id := range ids {
		if id == s.ID {
			return ids[(i+1)%len(ids)]
		}
	}
	return ids[0]
}

// BarTitle — подпись стиля для экрана настроек.
func BarTitle(id string) string {
	if s := BarStyleOf(id); s != nil {
		return s.Title
	}
	return "—"
}

// BarSize — размер полосы стиля id (0,0 — полос нет).
func BarSize(id string) (w, h int) {
	s := BarStyleOf(id)
	if s == nil {
		return 0, 0
	}
	return s.Frame[2], s.Frame[3]
}

// BarSlot — где внутри рамки лежит заливка: смещение и размер относительно
// левого верхнего угла полосы. Нужен тем, кто пишет поверх полосы (числа
// здоровья должны попадать в жёлоб, а не в узор рамки).
func BarSlot(id string) (dx, dy, w, h int, ok bool) {
	s := BarStyleOf(id)
	if s == nil {
		return 0, 0, 0, 0, false
	}
	if s.Fill != nil {
		return s.At[0], s.At[1], s.Fill[2], s.Fill[3], true
	}
	return s.Slot[0], s.Slot[1], s.Slot[2], s.Slot[3], true
}

// DrawBar рисует полосу стиля id левым верхним углом в (x,y), заполненную на
// долю frac. Возвращает false, если рисовать нечем — вызывающий покажет своё.
func DrawBar(dst *ebiten.Image, id string, x, y, frac float64) bool {
	s := BarStyleOf(id)
	if s == nil || bars.img == nil {
		return false
	}
	frac = min(max(frac, 0), 1)
	sub := func(r [4]int) *ebiten.Image {
		return bars.img.SubImage(image.Rect(r[0], r[1], r[0]+r[2], r[1]+r[3])).(*ebiten.Image)
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(x, y)
	dst.DrawImage(sub(s.Frame), op)

	switch {
	case s.Fill != nil:
		// Заливка режется слева: её правый край и есть уровень полосы.
		f := *s.Fill
		w := int(float64(f[2])*frac + 0.5)
		if w <= 0 {
			return true
		}
		f[2] = w
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(x+float64(s.At[0]), y+float64(s.At[1]))
		dst.DrawImage(sub(f), op)
	case s.Slot != nil:
		sl := *s.Slot
		w := float32(float64(sl[2]) * frac)
		if w <= 0 {
			return true
		}
		vector.FillRect(dst, float32(x)+float32(sl[0]), float32(y)+float32(sl[1]),
			w, float32(sl[3]), s.col, false)
	}
	return true
}
