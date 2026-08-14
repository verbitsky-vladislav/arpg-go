package scene

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/ui"
)

// Геометрия панели поверх игры.
const (
	ovItemW, ovItemH = 200, 26
	ovItemGap        = 8
	ovTitleScale     = 3
	ovTitleGap       = 16
)

var (
	ovDimPause = color.RGBA{0x08, 0x0a, 0x12, 0xc0} // затемнение паузы
	ovDimDeath = color.RGBA{0x2a, 0x08, 0x0c, 0xd0} // затемнение смерти — краснее
	ovPanel    = color.RGBA{0x0d, 0x11, 0x1c, 0xf0}
	ovDeadText = color.RGBA{0xe8, 0x6a, 0x58, 0xff}
)

// ovItem — пункт панели. Действие возвращает следующую сцену; ошибка нужна
// выходу из игры (ebiten.Termination останавливает цикл штатно).
type ovItem struct {
	label string
	act   func() (Scene, error)
}

// overlay — экран поверх замороженной игры: мир под затемнением продолжает
// рисоваться, но не обновляется. Из него сделаны и пауза, и смерть — разница
// только в заголовке, цвете и наборе пунктов.
type overlay struct {
	under  Scene // что видно под затемнением
	title  string
	tcol   color.Color
	dim    color.Color
	items  []ovItem
	cancel func() (Scene, error) // ESC; nil — ESC ничего не делает
	hint   string
	sel    int
}

// newPause — пауза забега: мир остаётся на экране, игрок видит, что никуда не
// делся.
func newPause(g *Game) *overlay {
	o := &overlay{
		under: g,
		title: "ПАУЗА",
		tcol:  menuTitle,
		dim:   ovDimPause,
		hint:  "ESC - ПРОДОЛЖИТЬ",
	}
	o.items = []ovItem{
		{"ПРОДОЛЖИТЬ", func() (Scene, error) { return g, nil }},
		{"НАСТРОЙКИ", func() (Scene, error) { return NewSettings(o), nil }},
		{"В МЕНЮ", func() (Scene, error) { return g.toMenu(), nil }},
		{"ВЫХОД", quit},
	}
	o.cancel = o.items[0].act
	return o
}

// newDeath — экран смерти. Возрождение поднимает того же героя на точке старта:
// забег не теряется, теряется только пройденный путь.
func newDeath(g *Game) *overlay {
	o := &overlay{
		under: g,
		title: "ВЫ ПОГИБЛИ",
		tcol:  ovDeadText,
		dim:   ovDimDeath,
		hint:  "ENTER - ВОЗРОДИТЬСЯ",
	}
	o.items = []ovItem{
		{"ВОЗРОДИТЬСЯ", func() (Scene, error) { return g.revive(), nil }},
		{"В МЕНЮ", func() (Scene, error) { return g.toMenu(), nil }},
		{"ВЫХОД", quit},
	}
	return o
}

// quit — выход из игры: Termination останавливает цикл Ebiten штатно.
func quit() (Scene, error) { return nil, ebiten.Termination }

func (o *overlay) Update() (Scene, error) {
	if o.cancel != nil && inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		return o.cancel()
	}

	mx, my := ebiten.CursorPosition()
	hovered := -1
	for i := range o.items {
		if x, y, w, h := o.itemRect(i); inRect(mx, my, x, y, w, h) {
			hovered, o.sel = i, i
		}
	}
	if keyPressed(ebiten.KeyDown, ebiten.KeyS) {
		o.sel = (o.sel + 1) % len(o.items)
	}
	if keyPressed(ebiten.KeyUp, ebiten.KeyW) {
		o.sel = (o.sel - 1 + len(o.items)) % len(o.items)
	}

	act := keyPressed(ebiten.KeyEnter, ebiten.KeyNumpadEnter, ebiten.KeySpace)
	if hovered >= 0 && inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		act = true
	}
	if act {
		return o.items[o.sel].act()
	}
	return o, nil
}

func (o *overlay) Draw(screen *ebiten.Image) {
	o.under.Draw(screen) // кадр игры замер: рисуем, но не обновляем
	vector.FillRect(screen, 0, 0, config.ScreenW, config.ScreenH, o.dim, false)

	top := o.top()
	px, pw := float32(config.ScreenW-ovItemW)/2-20, float32(ovItemW+40)
	py, ph := top-24, o.height()+48
	vector.FillRect(screen, px, py, pw, ph, ovPanel, false)
	vector.StrokeRect(screen, px+0.5, py+0.5, pw-1, ph-1, 1, menuFrame, false)

	ui.PixelTextCentered(screen, o.title, config.ScreenW/2, float64(top), ovTitleScale, o.tcol)
	for i, it := range o.items {
		o.drawItem(screen, i, it.label)
	}
	if o.hint != "" {
		ui.PixelTextCentered(screen, o.hint, config.ScreenW/2, float64(py+ph)+10, 1, menuEdge)
	}
	ui.Cursor = ui.CursorArrow // под затемнением игра, но выбираем мы в интерфейсе
}

// height — высота содержимого панели (заголовок и пункты).
func (o *overlay) height() float32 {
	items := float32(len(o.items)*ovItemH + (len(o.items)-1)*ovItemGap)
	return float32(ui.PixelTextHeight(ovTitleScale)) + ovTitleGap + items
}

// top — верх содержимого: панель стоит по центру экрана.
func (o *overlay) top() float32 { return (config.ScreenH - o.height()) / 2 }

// itemRect — рамка i-го пункта.
func (o *overlay) itemRect(i int) (x, y, w, h float32) {
	y = o.top() + float32(ui.PixelTextHeight(ovTitleScale)) + ovTitleGap + float32(i*(ovItemH+ovItemGap))
	return (config.ScreenW - ovItemW) / 2, y, ovItemW, ovItemH
}

func (o *overlay) drawItem(dst *ebiten.Image, i int, label string) {
	x, y, w, h := o.itemRect(i)
	plate, edge, text := menuPlate, menuEdge, menuText
	if i == o.sel {
		plate, edge, text = menuPlateSel, menuEdgeSel, menuTextSel
	}
	vector.FillRect(dst, x, y, w, h, plate, false)
	vector.StrokeRect(dst, x+0.5, y+0.5, w-1, h-1, 1, edge, false)
	for _, c := range [][2]float32{{x, y}, {x + w - 1, y}, {x, y + h - 1}, {x + w - 1, y + h - 1}} {
		vector.FillRect(dst, c[0], c[1], 1, 1, ovPanel, false)
	}
	ui.PixelTextCentered(dst, label, config.ScreenW/2,
		float64(y)+(ovItemH-ui.PixelTextHeight(2))/2, 2, text)
}
