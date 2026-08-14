package ui

// Окна интерфейса из пака ui/rpg_basic: рамка со свитком, зелёная шапка,
// сетка ячеек под предметы и гнёзда снаряжения.
//
// Разметка задана руками в panels.json по той же причине, что и у полос
// здоровья: нарезка по пустым промежуткам видит окно единым куском и про
// сетку внутри него ничего не знает.

import (
	"encoding/json"
	"fmt"
	"image"
	"io/fs"
	"path"
	"sort"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/vladislav/game/internal/assets"
)

// Panel — одно окно. Caption и Close — прямоугольники внутри окна: первый под
// собственную надпись, второй под клик по крестику. Grid задаёт сетку ячеек:
// [dx, dy, сторона, шаг, столбцов, строк]. Equip — гнёзда снаряжения.
type Panel struct {
	Title   string            `json:"title"`
	Sheet   string            `json:"sheet,omitempty"` // свой лист, если не общий
	Frame   [4]int            `json:"frame"`
	Caption [4]int            `json:"caption"`
	Close   [4]int            `json:"close"`
	Grid    *[6]int           `json:"grid,omitempty"`
	Equip   map[string][4]int `json:"equip,omitempty"`

	img   *ebiten.Image
	slots []string // гнёзда снаряжения в устойчивом порядке
}

type panels struct {
	Sheet  string            `json:"sheet"`
	Panels map[string]*Panel `json:"panels"`
}

var winds *panels

// InitPanels читает разметку окон из каталога dir. Как и полосы, окна не
// смертельны: без них экраны, которые их рисуют, просто не откроются.
func InitPanels(l *assets.Loader, dir string) error {
	b, err := fs.ReadFile(l.FS(), path.Join(dir, "panels.json"))
	if err != nil {
		return fmt.Errorf("ui: чтение разметки окон: %w", err)
	}
	var p panels
	if err := json.Unmarshal(b, &p); err != nil {
		return fmt.Errorf("ui: разбор разметки окон: %w", err)
	}
	if len(p.Panels) == 0 {
		return fmt.Errorf("ui: в разметке окон нет ни одного окна")
	}
	sheets := map[string]*ebiten.Image{}
	for id, w := range p.Panels {
		name := w.Sheet
		if name == "" {
			name = p.Sheet
		}
		img, ok := sheets[name]
		if !ok {
			if img, err = l.Image(path.Join(dir, name)); err != nil {
				return fmt.Errorf("ui: окно %q: лист %q: %w", id, name, err)
			}
			sheets[name] = img
		}
		w.img = img
		if err := w.check(id, img.Bounds().Dx(), img.Bounds().Dy()); err != nil {
			return err
		}
		w.slots = make([]string, 0, len(w.Equip))
		for k := range w.Equip {
			w.slots = append(w.slots, k)
		}
		sort.Strings(w.slots)
	}
	winds = &p
	return nil
}

// check — разметка обязана лежать внутри своего листа и своей рамки: окно,
// у которого сетка съехала за пергамент, рисует предметы по краю бумаги.
func (p *Panel) check(id string, sw, sh int) error {
	if !inSheet(p.Frame, sw, sh) {
		return fmt.Errorf("ui: окно %q: рамка вылезает за лист", id)
	}
	if g := p.Grid; g != nil {
		if g[2] <= 0 || g[3] <= 0 || g[4] <= 0 || g[5] <= 0 {
			return fmt.Errorf("ui: окно %q: пустая сетка ячеек", id)
		}
		if g[0]+(g[4]-1)*g[3]+g[2] > p.Frame[2] || g[1]+(g[5]-1)*g[3]+g[2] > p.Frame[3] {
			return fmt.Errorf("ui: окно %q: сетка не помещается в рамку", id)
		}
	}
	for name, r := range p.Equip {
		if r[2] <= 0 || r[3] <= 0 || r[0] < 0 || r[1] < 0 ||
			r[0]+r[2] > p.Frame[2] || r[1]+r[3] > p.Frame[3] {
			return fmt.Errorf("ui: окно %q: гнездо %q не помещается в рамку", id, name)
		}
	}
	return nil
}

// Window — окно по имени (nil, если разметки нет).
func Window(id string) *Panel {
	if winds == nil {
		return nil
	}
	return winds.Panels[id]
}

// Size — размер окна в пикселях.
func (p *Panel) Size() (w, h int) {
	if p == nil {
		return 0, 0
	}
	return p.Frame[2], p.Frame[3]
}

// Slots — сколько ячеек у окна.
func (p *Panel) Slots() int {
	if p == nil || p.Grid == nil {
		return 0
	}
	return p.Grid[4] * p.Grid[5]
}

// SlotRect — рамка i-й ячейки относительно левого верхнего угла окна.
func (p *Panel) SlotRect(i int) (x, y, w, h int) {
	if p == nil || p.Grid == nil || i < 0 || i >= p.Slots() {
		return 0, 0, 0, 0
	}
	g := p.Grid
	col, row := i%g[4], i/g[4]
	return g[0] + col*g[3], g[1] + row*g[3], g[2], g[2]
}

// SlotAt — номер ячейки под точкой (mx,my), если окно нарисовано в (px,py).
// -1 — мимо ячеек.
func (p *Panel) SlotAt(mx, my, px, py float64) int {
	for i := range p.Slots() {
		x, y, w, h := p.SlotRect(i)
		if inBox(mx, my, px+float64(x), py+float64(y), float64(w), float64(h)) {
			return i
		}
	}
	return -1
}

// EquipSlots — имена гнёзд снаряжения в устойчивом порядке.
func (p *Panel) EquipSlots() []string {
	if p == nil {
		return nil
	}
	return p.slots
}

// EquipRect — рамка гнезда относительно угла окна.
func (p *Panel) EquipRect(name string) ([4]int, bool) {
	if p == nil {
		return [4]int{}, false
	}
	r, ok := p.Equip[name]
	return r, ok
}

// EquipAt — гнездо под точкой ("" — мимо).
func (p *Panel) EquipAt(mx, my, px, py float64) string {
	for _, name := range p.EquipSlots() {
		r := p.Equip[name]
		if inBox(mx, my, px+float64(r[0]), py+float64(r[1]), float64(r[2]), float64(r[3])) {
			return name
		}
	}
	return ""
}

// InRect — попадает ли точка в прямоугольник окна r (в координатах окна).
func (p *Panel) InRect(r [4]int, mx, my, px, py float64) bool {
	return inBox(mx, my, px+float64(r[0]), py+float64(r[1]), float64(r[2]), float64(r[3]))
}

func inBox(mx, my, x, y, w, h float64) bool {
	return mx >= x && mx < x+w && my >= y && my < y+h
}

// Draw рисует окно левым верхним углом в (x,y).
func (p *Panel) Draw(dst *ebiten.Image, x, y float64) {
	if p == nil || p.img == nil {
		return
	}
	f := p.Frame
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(x, y)
	dst.DrawImage(p.img.SubImage(image.Rect(f[0], f[1], f[0]+f[2], f[1]+f[3])).(*ebiten.Image), op)
}
