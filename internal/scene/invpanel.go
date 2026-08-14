package scene

// Окно с ячейками: одинаково выглядят и сумка героя, и содержимое сундука —
// разница только в заголовке и в том, что внутри. Поэтому рисование одно на
// оба экрана.

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/vladislav/game/internal/assets"
	"github.com/vladislav/game/internal/item"
	"github.com/vladislav/game/internal/ui"
)

// invPanel — окно вместе с тем, что нужно, чтобы нарисовать в нём предметы.
type invPanel struct {
	p   *ui.Panel
	cat *item.Catalog
	l   *assets.Loader
}

// newInvPanel собирает окно id для забега. Второе значение — false, если
// разметки окон нет: экран тогда просто не открывается.
func newInvPanel(g *Game, id string) (invPanel, bool) {
	p := ui.Window(id)
	if p == nil || g == nil {
		return invPanel{}, false
	}
	return invPanel{p: p, cat: g.items, l: g.l}, true
}

// draw рисует окно левым верхним углом в (x,y) с заголовком и содержимым.
// hover — подсвеченная ячейка (-1 нет).
func (w invPanel) draw(dst *ebiten.Image, x, y float64, title string, inv *item.Inventory, hover int) {
	w.p.Draw(dst, x, y)

	head := w.p.Caption
	ui.PixelText(dst, title, x+float64(head[0])+2,
		y+float64(head[1])+(float64(head[3])-ui.PixelTextHeight(1))/2, 1, chestCap)

	for i := range w.p.Slots() {
		sx, sy, sw, sh := w.p.SlotRect(i)
		w.cell(dst, x+float64(sx), y+float64(sy), sw, sh, inv.At(i), i == hover)
	}
}

// drawWorn рисует надетое в гнёздах снаряжения. Гнёзда нарисованы на арте
// призраками (меч, щит, кольцо, амулет) — вещь ложится поверх своего призрака.
func (w invPanel) drawWorn(dst *ebiten.Image, x, y float64, eq *item.Equipment, hover string) {
	for _, name := range w.p.EquipSlots() {
		r, ok := w.p.EquipRect(name)
		if !ok {
			continue
		}
		w.cell(dst, x+float64(r[0]), y+float64(r[1]), r[2], r[3], eq.At(name), name == hover)
	}
}

// cell — одна ячейка: подсветка, иконка, число в стопке.
func (w invPanel) cell(dst *ebiten.Image, ox, oy float64, sw, sh int, s item.Slot, hover bool) {
	if hover {
		vector.FillRect(dst, float32(ox), float32(oy), float32(sw), float32(sh), chestHover, false)
	}
	if s.Empty() {
		return
	}
	img, err := w.cat.Icon(w.l, s.ID)
	if err != nil {
		// Предмет без картинки не должен исчезать: показываем ячейку занятой,
		// иначе добыча просто пропадёт из окна.
		vector.FillRect(dst, float32(ox)+3, float32(oy)+3, float32(sw)-6, float32(sh)-6, chestCount, false)
		return
	}
	b := img.Bounds()
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(ox+float64(sw-b.Dx())/2, oy+float64(sh-b.Dy())/2)
	dst.DrawImage(img, op)

	if s.N > 1 {
		n := fmt.Sprint(s.N)
		tx := ox + float64(sw) - ui.PixelTextWidth(n, 1)
		ty := oy + float64(sh) - ui.PixelTextHeight(1)
		ui.PixelText(dst, n, tx+1, ty+1, 1, chestShadow)
		ui.PixelText(dst, n, tx, ty, 1, chestCount)
	}
}

// nameAt — как называется предмет в ячейке i (пусто, если ячейка пуста).
func (w invPanel) nameAt(inv *item.Inventory, i int) string {
	if s := inv.At(i); !s.Empty() {
		return w.cat.Name(s.ID)
	}
	return ""
}
