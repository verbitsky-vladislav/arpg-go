package scene

import (
	"image/color"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/ui"
)

// Палитра меню: тёмный сине-стальной фон, золотой акцент на выборе.
var (
	menuBG       = color.RGBA{0x0b, 0x0e, 0x18, 0xff}
	menuScan     = color.RGBA{0x00, 0x00, 0x00, 0x38}
	menuFrame    = color.RGBA{0x27, 0x30, 0x4d, 0xff}
	menuPlate    = color.RGBA{0x16, 0x1b, 0x2b, 0xff}
	menuPlateSel = color.RGBA{0x2b, 0x35, 0x55, 0xff}
	menuEdge     = color.RGBA{0x4a, 0x58, 0x86, 0xff}
	menuEdgeSel  = color.RGBA{0xf2, 0xc9, 0x6b, 0xff}
	menuText     = color.RGBA{0xb8, 0xc4, 0xe6, 0xff}
	menuTextSel  = color.RGBA{0xff, 0xf3, 0xd0, 0xff}
	menuTitle    = color.RGBA{0xf2, 0xc9, 0x6b, 0xff}
	menuShadow   = color.RGBA{0x05, 0x07, 0x0e, 0xff}
)

// Геометрия пунктов (логическое разрешение 640×360).
const (
	itemW         = 180
	itemH         = 28
	itemGap       = 10
	itemTop       = 140
	itemTextScale = 2
)

// menuItem — пункт меню: подпись и действие. Действие nil (экран ещё не
// написан) — пункт рисуется как обычный, но выбор ничего не делает.
type menuItem struct {
	label string
	act   func() (Scene, error)
}

// Menu — стартовый экран: заголовок и три пункта. Управление мышью
// (наведение + клик) и клавиатурой (стрелки/WS + Enter/Space).
//
// Экраны за пунктами подключаются снаружи — присвоением OnStart/OnSettings/
// OnBestiary до первого Update; пока они nil, меню остаётся на месте.
type Menu struct {
	OnStart    func() Scene
	OnSettings func() Scene
	OnBestiary func() Scene

	// Resume — забег, поставленный на паузу: меню открыто из игры, и тот же ESC
	// возвращает обратно. nil — возвращаться некуда (игра ещё не начиналась).
	Resume Scene

	sel int // индекс выбранного пункта
}

// NewMenu создаёт стартовое меню.
func NewMenu() *Menu { return &Menu{} }

func (m *Menu) items() []menuItem {
	// Переходы приходят снаружи как func() Scene — здесь они оборачиваются в
	// действие с ошибкой, потому что выход из игры возвращает Termination.
	open := func(f func() Scene) func() (Scene, error) {
		if f == nil {
			return nil
		}
		return func() (Scene, error) { return f(), nil }
	}
	return []menuItem{
		{"СТАРТ", open(m.OnStart)},
		{"НАСТРОЙКИ", open(m.OnSettings)},
		{"БЕСТИАРИЙ", open(m.OnBestiary)},
		{"ВЫХОД", quit},
	}
}

// itemRect — прямоугольник i-го пункта.
func itemRect(i int) (x, y, w, h float32) {
	return (config.ScreenW - itemW) / 2, itemTop + float32(i*(itemH+itemGap)), itemW, itemH
}

func (m *Menu) Update() (Scene, error) {
	if m.Resume != nil && inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		return m.Resume, nil
	}
	items := m.items()

	// Мышь: наведение перекладывает выбор на пункт под курсором.
	mx, my := ebiten.CursorPosition()
	hovered := -1
	for i := range items {
		x, y, w, h := itemRect(i)
		if float32(mx) >= x && float32(mx) < x+w && float32(my) >= y && float32(my) < y+h {
			hovered = i
			m.sel = i
		}
	}

	// Клавиатура: перебор пунктов по кругу.
	if keyPressed(ebiten.KeyDown, ebiten.KeyS) {
		m.sel = (m.sel + 1) % len(items)
	}
	if keyPressed(ebiten.KeyUp, ebiten.KeyW) {
		m.sel = (m.sel - 1 + len(items)) % len(items)
	}

	activate := keyPressed(ebiten.KeyEnter, ebiten.KeyNumpadEnter, ebiten.KeySpace)
	if hovered >= 0 && inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		activate = true
	}
	if activate {
		if act := items[m.sel].act; act != nil {
			return act()
		}
	}
	return m, nil
}

// keyPressed — нажата ли в этом кадре любая из клавиш.
func keyPressed(keys ...ebiten.Key) bool {
	return slices.ContainsFunc(keys, inpututil.IsKeyJustPressed)
}

func (m *Menu) Draw(screen *ebiten.Image) {
	m.drawBackground(screen)

	// Заголовок с тенью в один «пиксель» шрифта (scale 4 → 4 экранных px).
	const titleScale = 4
	ui.PixelTextCentered(screen, "PIXEL ARPG", config.ScreenW/2+titleScale, 56+titleScale, titleScale, menuShadow)
	ui.PixelTextCentered(screen, "PIXEL ARPG", config.ScreenW/2, 56, titleScale, menuTitle)
	vector.FillRect(screen, config.ScreenW/2-110, 104, 220, 1, menuFrame, false)

	for i, it := range m.items() {
		m.drawItem(screen, i, it.label, i == m.sel)
	}

	hint := "МЫШЬ ИЛИ СТРЕЛКИ - ВЫБОР, ENTER - ПУСК"
	if m.Resume != nil {
		hint = "ESC - ВЕРНУТЬСЯ В ИГРУ,  ENTER - ВЫБОР"
	}
	ui.PixelTextCentered(screen, hint, config.ScreenW/2, config.ScreenH-24, 1, menuEdge)
}

// drawBackground — фон: заливка, скан-линии и рамка экрана со «срезанными»
// пиксельными углами.
func (m *Menu) drawBackground(screen *ebiten.Image) {
	screen.Fill(menuBG)
	for y := 0; y < config.ScreenH; y += 3 {
		vector.FillRect(screen, 0, float32(y), config.ScreenW, 1, menuScan, false)
	}
	vector.StrokeRect(screen, 5.5, 5.5, config.ScreenW-11, config.ScreenH-11, 1, menuFrame, false)
	for _, c := range [][2]float32{{5, 5}, {config.ScreenW - 6, 5}, {5, config.ScreenH - 6}, {config.ScreenW - 6, config.ScreenH - 6}} {
		vector.FillRect(screen, c[0], c[1], 1, 1, menuBG, false)
	}
}

// drawItem рисует пункт: плашка, рамка со скошенными углами, подпись и
// «стрелки» по бокам у выбранного.
func (m *Menu) drawItem(screen *ebiten.Image, i int, label string, sel bool) {
	x, y, w, h := itemRect(i)
	plate, edge, text := menuPlate, menuEdge, menuText
	if sel {
		plate, edge, text = menuPlateSel, menuEdgeSel, menuTextSel
	}

	vector.FillRect(screen, x, y, w, h, plate, false)
	vector.StrokeRect(screen, x+0.5, y+0.5, w-1, h-1, 1, edge, false)
	// Углы срезаем фоном — характерная «фаска» пиксельной кнопки.
	for _, c := range [][2]float32{{x, y}, {x + w - 1, y}, {x, y + h - 1}, {x + w - 1, y + h - 1}} {
		vector.FillRect(screen, c[0], c[1], 1, 1, menuBG, false)
	}

	ty := float64(y) + (itemH-ui.PixelTextHeight(itemTextScale))/2
	ui.PixelTextCentered(screen, label, config.ScreenW/2, ty, itemTextScale, text)
	if sel {
		ui.PixelText(screen, ">", float64(x)+9, ty, itemTextScale, edge)
		ui.PixelText(screen, "<", float64(x+w)-9-ui.PixelTextWidth("<", itemTextScale), ty, itemTextScale, edge)
	}
}
