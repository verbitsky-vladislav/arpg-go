package scene

// Ввод имени персонажа.
//
// Принимается только то, что шрифт умеет нарисовать (ui.PixelHasGlyph):
// пиксельный шрифт знает кириллицу, латиницу, цифры и немного знаков, а
// принятая, но не рисуемая буква превратилась бы в «?» уже после ввода — когда
// исправить её игрок сможет только стиранием.

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/save"
	"github.com/vladislav/game/internal/ui"
)

// Геометрия поля ввода (логическое разрешение 640×360).
const (
	nameFieldW, nameFieldH = 260, 40
	nameFieldY             = 150
	nameScale              = 2
	// Стирание с удержания: первое повторение через holdDelay, дальше каждые
	// holdRate тиков. Без этого длинное имя стирается по одной букве за нажатие.
	nameHoldDelay = 30
	nameHoldRate  = 4
)

// nameEntry — экран «как звать героя».
type nameEntry struct {
	back Scene
	sub  string // какое тело выбрано — чтобы не гадать, кого называешь
	done func(string) (Scene, error)

	name  []rune
	buf   []rune // буфер ввода Ebiten, переиспользуется без аллокаций
	blink int
	err   string
}

// newName собирает экран ввода имени. done получает уже вычищенное имя.
func newName(back Scene, bodyID, bodyTitle string, done func(string) (Scene, error)) *nameEntry {
	sub := bodyTitle
	if sub == "" {
		sub = bodyID
	}
	return &nameEntry{back: back, sub: sub, done: done}
}

func (n *nameEntry) Update() (Scene, error) {
	n.blink++
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		return n.backScene(), nil
	}

	n.buf = ebiten.AppendInputChars(n.buf[:0])
	for _, r := range n.buf {
		if len(n.name) >= save.NameLimit || !ui.PixelHasGlyph(r) {
			continue
		}
		n.name = append(n.name, r)
		n.err = ""
	}
	if n.erasing() && len(n.name) > 0 {
		n.name = n.name[:len(n.name)-1]
	}

	if keyPressed(ebiten.KeyEnter, ebiten.KeyNumpadEnter) {
		clean := save.CleanName(string(n.name))
		if clean == "" {
			n.err = "ВВЕДИТЕ ИМЯ"
			return n, nil
		}
		return n.done(clean)
	}
	return n, nil
}

// erasing — надо ли стереть букву в этом тике: нажатие или повтор с удержания.
func (n *nameEntry) erasing() bool {
	d := inpututil.KeyPressDuration(ebiten.KeyBackspace)
	return d == 1 || (d > nameHoldDelay && d%nameHoldRate == 0)
}

func (n *nameEntry) backScene() Scene {
	if n.back != nil {
		return n.back
	}
	return NewMenu()
}

func (n *nameEntry) Draw(screen *ebiten.Image) {
	drawMenuBack(screen)
	ui.PixelTextCentered(screen, "ИМЯ ГЕРОЯ", config.ScreenW/2, 60, 3, menuTitle)
	ui.PixelTextCentered(screen, n.sub, config.ScreenW/2, 100, 1, menuEdge)

	x := float32(config.ScreenW-nameFieldW) / 2
	vector.FillRect(screen, x, nameFieldY, nameFieldW, nameFieldH, menuPlate, false)
	vector.StrokeRect(screen, x+0.5, nameFieldY+0.5, nameFieldW-1, nameFieldH-1, 1, menuEdgeSel, false)

	txt := string(n.name)
	ty := float64(nameFieldY) + (nameFieldH-ui.PixelTextHeight(nameScale))/2
	w := ui.PixelTextWidth(txt, nameScale)
	tx := float64(config.ScreenW)/2 - w/2
	ui.PixelText(screen, txt, tx, ty, nameScale, menuTextSel)
	// Курсор мигает полсекунды через полсекунды — так поле видно как активное,
	// даже когда оно пустое.
	if n.blink%(config.TPS) < config.TPS/2 {
		vector.FillRect(screen, float32(tx+w)+2, float32(ty), nameScale, float32(ui.PixelTextHeight(nameScale)), menuTextSel, false)
	}

	ui.PixelTextCentered(screen, "ДО 12 ЗНАКОВ", config.ScreenW/2, nameFieldY+nameFieldH+10, 1, menuEdge)
	if n.err != "" {
		ui.PixelTextCentered(screen, n.err, config.ScreenW/2, nameFieldY+nameFieldH+26, 1, ovDeadText)
	}
	ui.PixelTextCentered(screen, "ENTER - В ИГРУ,  ESC - НАЗАД", config.ScreenW/2, config.ScreenH-24, 1, menuEdge)
	ui.Cursor = ui.CursorArrow
}
