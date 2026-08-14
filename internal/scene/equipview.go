package scene

// Окно героя: слева фигура с гнёздами снаряжения, справа сумка. Открывается
// отдельно (клавиша «сумка») и оно же служит правой половиной окна сундука —
// поэтому надеть найденное можно прямо у сундука, не закрывая его.

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/settings"
	"github.com/vladislav/game/internal/ui"
)

// bagPane — панель героя в фиксированном месте экрана. Знает только про
// попадание курсора и отрисовку; что делать с кликом, решает экран.
type bagPane struct {
	w    invPanel
	x, y float64

	slot int    // ячейка сумки под курсором (-1 нет)
	worn string // гнездо снаряжения под курсором ("" нет)
}

func newBagPane(g *Game) (*bagPane, bool) {
	w, ok := newInvPanel(g, "equipment")
	if !ok {
		return nil, false
	}
	return &bagPane{w: w, slot: -1}, true
}

func (b *bagPane) size() (int, int) { return b.w.p.Size() }

// aim запоминает, что под курсором.
func (b *bagPane) aim(mx, my float64) {
	b.slot = b.w.p.SlotAt(mx, my, b.x, b.y)
	b.worn = ""
	if b.slot < 0 {
		b.worn = b.w.p.EquipAt(mx, my, b.x, b.y)
	}
}

// hovered — есть ли вообще что-то под курсором.
func (b *bagPane) hovered() bool { return b.slot >= 0 || b.worn != "" }

// note — подпись про то, на что наведён курсор.
func (b *bagPane) note(g *Game) string {
	switch {
	case b.slot >= 0:
		return b.w.nameAt(g.bag, b.slot)
	case b.worn != "":
		if n := g.wornName(b.worn); n != "" {
			return n
		}
		return equipTitle(b.worn)
	}
	return ""
}

func (b *bagPane) draw(dst *ebiten.Image, g *Game, title string) {
	b.w.draw(dst, b.x, b.y, title, g.bag, b.slot)
	b.w.drawWorn(dst, b.x, b.y, g.eq, b.worn)
}

// closeHit — клик по крестику окна.
func (b *bagPane) closeHit(mx, my float64) bool {
	return b.w.p.InRect(b.w.p.Close, mx, my, b.x, b.y)
}

// equipTitle — как называется пустое гнездо.
func equipTitle(slot string) string {
	switch slot {
	case "weapon":
		return "ГНЕЗДО ОРУЖИЯ"
	case "shield":
		return "ГНЕЗДО ЩИТА"
	case "ring":
		return "ГНЕЗДО КОЛЬЦА"
	case "amulet":
		return "ГНЕЗДО АМУЛЕТА"
	}
	return slot
}

// equipView — окно героя само по себе. Мир под ним замирает, как в паузе:
// разбирать сумку посреди драки всё равно нельзя.
type equipView struct {
	g    *Game
	pane *bagPane
	note string
}

// newEquipView открывает окно героя. nil — разметки окон нет, показывать нечем.
func newEquipView(g *Game) *equipView {
	pane, ok := newBagPane(g)
	if !ok {
		return nil
	}
	w, h := pane.size()
	pane.x = float64(int((config.ScreenW - float64(w)) / 2))
	pane.y = float64(int((config.ScreenH - float64(h)) / 2))
	return &equipView{g: g, pane: pane}
}

func (v *equipView) Update() (Scene, error) {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) ||
		inpututil.IsKeyJustPressed(settings.Key(settings.ActBag)) {
		return v.g, nil
	}
	mx, my := ebiten.CursorPosition()
	fx, fy := float64(mx), float64(my)
	v.pane.aim(fx, fy)
	if n := v.pane.note(v.g); n != "" {
		v.note = n
	}

	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return v, nil
	}
	if v.pane.closeHit(fx, fy) {
		return v.g, nil
	}
	// Клик надевает и снимает: отдельной кнопки не нужно, сторона окна и так
	// говорит, что произойдёт.
	switch {
	case v.pane.slot >= 0:
		if s := v.g.equipFromBag(v.pane.slot); s != "" {
			v.note = s
		}
	case v.pane.worn != "":
		if s := v.g.unequip(v.pane.worn); s != "" {
			v.note = s
		}
	}
	return v, nil
}

func (v *equipView) Draw(screen *ebiten.Image) {
	v.g.Draw(screen)
	vector.FillRect(screen, 0, 0, config.ScreenW, config.ScreenH, ovDimPause, false)
	ui.Cursor = ui.CursorArrow

	v.pane.draw(screen, v.g, "ГЕРОЙ")

	_, h := v.pane.size()
	note := v.note
	if note == "" && v.g.bag.Empty() {
		note = "СУМКА ПУСТА"
	}
	if note != "" {
		ui.PixelTextCentered(screen, note, config.ScreenW/2, v.pane.y+float64(h)+6, 1, chestHint)
	}
	ui.PixelTextCentered(screen, "ЛКМ - НАДЕТЬ ИЛИ СНЯТЬ,  ESC - ЗАКРЫТЬ",
		config.ScreenW/2, v.pane.y+float64(h)+16, 1, chestHint)
}
