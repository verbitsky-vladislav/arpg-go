package scene

// Экран открытого сундука: слева его содержимое, справа то же окно героя, что
// открывается клавишей сумки. Мир под ним замирает и продолжает рисоваться —
// как в паузе.

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/settings"
	"github.com/vladislav/game/internal/ui"
)

// chestPanelGap — зазор между окном сундука и окном героя.
const chestPanelGap = 10

var (
	chestCap    = color.RGBA{0x1d, 0x2b, 0x1f, 0xff} // надпись на зелёной шапке
	chestCount  = color.RGBA{0xfa, 0xf2, 0xd6, 0xff}
	chestShadow = color.RGBA{0x2a, 0x1c, 0x10, 0xff}
	chestHover  = color.RGBA{0xff, 0xf0, 0xb0, 0x50}
	chestHint   = color.RGBA{0xd8, 0xc8, 0xa0, 0xff}
)

// chestView — окно сундука. Ничего не хранит про предметы сам: и сундук, и
// сумка героя — обычные item.Inventory, экран только показывает и переносит.
type chestView struct {
	g    *Game
	c    *chest
	w    invPanel // окно сундука
	pane *bagPane // окно героя справа

	px, py float64 // левый верхний угол окна сундука
	hover  int     // ячейка сундука под курсором (-1 нет)
	note   string
}

// newChestView открывает сундук. Возвращает nil, если показывать нечем —
// сцена тогда просто не меняется, а не падает.
func newChestView(g *Game, c *chest) *chestView {
	w, ok := newInvPanel(g, "grid")
	if !ok || c == nil {
		return nil
	}
	pane, ok := newBagPane(g)
	if !ok {
		return nil
	}
	cw, ch := w.p.Size()
	bw, bh := pane.size()
	total := float64(cw + chestPanelGap + bw)

	v := &chestView{g: g, c: c, w: w, pane: pane, hover: -1}
	v.px = float64(int((config.ScreenW - total) / 2))
	v.py = float64(int((config.ScreenH - float64(ch)) / 2))
	pane.x = v.px + float64(cw+chestPanelGap)
	// Окна разной высоты — выравниваем по центру, иначе правое висит углом.
	pane.y = float64(int(v.py + float64(ch-bh)/2))
	return v
}

func (v *chestView) Update() (Scene, error) {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) ||
		inpututil.IsKeyJustPressed(settings.Key(settings.ActUse)) {
		uiCancel()
		return v.g, nil
	}

	mx, my := ebiten.CursorPosition()
	fx, fy := float64(mx), float64(my)
	v.hover = v.w.p.SlotAt(fx, fy, v.px, v.py)
	v.pane.aim(fx, fy)
	if v.hover >= 0 {
		if n := v.g.itemNote(v.c.inv.At(v.hover).ID); n != "" {
			v.note = n
		}
	} else if n := v.pane.note(v.g); n != "" {
		v.note = n
	}

	// Забрать всё — привычнее, чем щёлкать по каждой ячейке.
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		if v.c.inv.MoveAllTo(v.g.bag) == 0 && !v.c.inv.Empty() {
			v.note = "СУМКА ПОЛНА"
		}
	}

	right := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight)
	left := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
	// Правой кнопкой вещь надевается прямо у сундука: иначе одно и то же
	// движение означало бы то «убрать в сундук», то «надеть».
	if right && v.pane.slot >= 0 {
		if s := v.g.equipFromBag(v.pane.slot); s != "" {
			v.note = s
		}
		return v, nil
	}
	if !left {
		return v, nil
	}
	if v.w.p.InRect(v.w.p.Close, fx, fy, v.px, v.py) || v.pane.closeHit(fx, fy) {
		return v.g, nil
	}
	switch {
	case v.hover >= 0: // из сундука в сумку
		if moved := v.c.inv.MoveTo(v.g.bag, v.hover); moved == 0 && !v.c.inv.At(v.hover).Empty() {
			v.note = "СУМКА ПОЛНА"
		}
	case v.pane.slot >= 0: // из сумки в сундук
		v.g.bag.MoveTo(v.c.inv, v.pane.slot)
	case v.pane.worn != "": // снять надетое
		if s := v.g.unequip(v.pane.worn); s != "" {
			v.note = s
		}
	}
	return v, nil
}

func (v *chestView) Draw(screen *ebiten.Image) {
	v.g.Draw(screen)
	vector.FillRect(screen, 0, 0, config.ScreenW, config.ScreenH, ovDimPause, false)
	ui.Cursor = ui.CursorArrow

	v.w.draw(screen, v.px, v.py, v.c.kind.Title, v.c.inv, v.hover)
	v.pane.draw(screen, v.g, "ГЕРОЙ")

	_, h := v.w.p.Size()
	if v.note != "" {
		ui.PixelTextCentered(screen, v.note, config.ScreenW/2, v.py+float64(h)+6, 1, chestHint)
	}
	ui.PixelTextCentered(screen, "ЛКМ - ПЕРЕЛОЖИТЬ,  ПКМ - НАДЕТЬ,  ПРОБЕЛ - ЗАБРАТЬ ВСЁ,  ESC - ЗАКРЫТЬ",
		config.ScreenW/2, v.py+float64(h)+16, 1, chestHint)
}
