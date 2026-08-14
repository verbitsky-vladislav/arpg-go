package ui

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// CursorKind — какой курсор рисовать. Системный курсор скрыт (см. main), рисуем
// свой: в интерфейсе — стрелка, в бою — прицел. Сцена выставляет вид в Draw,
// движок перед каждым кадром сбрасывает его на стрелку (см. core.Game.Draw).
type CursorKind int

const (
	CursorArrow CursorKind = iota // меню, книга, настройки
	CursorAim                     // игра: целимся мышью
)

// Cursor — вид курсора текущего кадра.
var Cursor CursorKind

// Как прячется системный курсор — и почему одного вызова при старте мало.
//
// SetCursorMode(Hidden) до RunGame запоминается и применяется при создании
// окна, но применяется наполовину: в macOS-части GLFW «спрятать» доходит до
// системы только если указатель в этот момент над окном:
//
//	void _glfwPlatformSetCursorMode(...) {   // cocoa_window_darwin.m
//	    if (cursorInContentArea(window))
//	        updateCursorImage(window);       // вот здесь [NSCursor hide]
//	}
//
// Игру же запускают, когда мышь над терминалом или иконкой, а не над окном,
// которого ещё нет. Свой режим GLFW при этом уже записал — и все последующие
// попытки спрятать курсор молча выходят на первой же проверке:
//
//	if (window->cursorMode == value) return;  // input_unix.c
//
// В итоге системный курсор остаётся на экране рядом с нарисованным, и видно их
// оба. Тот же разрыв случается при переходе в полный экран (на macOS это новое
// окно) и при возврате фокуса.
//
// Отсюда два правила ниже: режим приходится сбрасывать на видимый и ставить
// заново (иначе проверка равенства съест вызов), и делать это в те моменты,
// когда состояние могло разъехаться, — а не каждый кадр, чтобы не гонять
// показать/спрятать по шестьдесят раз в секунду.
var (
	curKnown bool // хоть раз прятали
	curFull  bool // полный экран в прошлый раз
	curFocus bool // окно было в фокусе
	curIn    bool // указатель был над окном
	curWarm  int  // тиков с начала игры (пока идёт разогрев — перепроверяем)
)

// Разогрев: первые две секунды состояние перепроверяется по расписанию, потому
// что окно в это время ещё едет — settings.Init уводит игру в полный экран
// через несколько кадров после старта. Шаг в треть секунды, а не каждый тик:
// сброс режима на мгновение показывает системный курсор, и делать это шестьдесят
// раз в секунду значит моргать им.
const (
	curWarmTicks = 2 * 60
	curWarmEvery = 20
)

// KeepCursorHidden держит системный курсор спрятанным. Зовётся каждый тик из
// игрового цикла; настоящая работа делается, только когда состояние могло
// разъехаться (см. рассуждение выше).
func KeepCursorHidden(screenW, screenH int) {
	mx, my := ebiten.CursorPosition()
	in := mx >= 0 && my >= 0 && mx < screenW && my < screenH
	full, focus := ebiten.IsFullscreen(), ebiten.IsFocused()

	changed := !curKnown || full != curFull || focus != curFocus || in != curIn
	warm := curWarm < curWarmTicks && curWarm%curWarmEvery == 0
	curWarm++
	curKnown, curFull, curFocus, curIn = true, full, focus, in
	if !changed && !warm {
		return
	}
	if !in || !focus {
		// Указатель не над нашим окном (или окно не в фокусе) — курсор сейчас
		// не наш: прятать чужой курсор и нельзя, и незачем. Вернётся фокус —
		// вернёмся сюда с changed == true.
		return
	}
	// Сброс на видимый — не украшательство: без него «спрятать» не дойдёт до
	// окна, потому что GLFW сравнит режим с текущим и выйдет.
	ebiten.SetCursorMode(ebiten.CursorModeVisible)
	ebiten.SetCursorMode(ebiten.CursorModeHidden)
}

// arrowArt — рисунок стрелки: '#' — обводка, 'X' — заливка.
var arrowArt = []string{
	"#.........",
	"##........",
	"#X#.......",
	"#XX#......",
	"#XXX#.....",
	"#XXXX#....",
	"#XXXXX#...",
	"#XXXXXX#..",
	"#XXX####..",
	"#X#XX#....",
	"##.#XX#...",
	"...#XX#...",
	"....###...",
}

var arrowImg *ebiten.Image

// arrow — картинка стрелки (строится один раз, как глифы шрифта).
func arrow() *ebiten.Image {
	if arrowImg != nil {
		return arrowImg
	}
	w, h := len(arrowArt[0]), len(arrowArt)
	px := make([]byte, w*h*4)
	for y, row := range arrowArt {
		for x := 0; x < w && x < len(row); x++ {
			var c color.RGBA
			switch row[x] {
			case '#':
				c = color.RGBA{0x08, 0x0a, 0x10, 0xff}
			case 'X':
				c = color.RGBA{0xf2, 0xf6, 0xff, 0xff}
			default:
				continue
			}
			i := (y*w + x) * 4
			px[i], px[i+1], px[i+2], px[i+3] = c.R, c.G, c.B, c.A
		}
	}
	arrowImg = ebiten.NewImage(w, h)
	arrowImg.WritePixels(px)
	return arrowImg
}

// DrawCursor рисует курсор в позиции мыши.
//
// Не рисует вовсе, когда указателя над окном нет или окно не в фокусе: там
// курсор принадлежит системе, она его показывает — и наш, замерший в последней
// известной точке, оказался бы вторым. Именно это видно при запуске, пока фокус
// ещё у терминала: две мыши, одна бежит, другая стоит.
func DrawCursor(dst *ebiten.Image) {
	mx, my := ebiten.CursorPosition()
	b := dst.Bounds()
	if !ebiten.IsFocused() || mx < b.Min.X || my < b.Min.Y || mx >= b.Max.X || my >= b.Max.Y {
		return
	}
	if Cursor == CursorAim {
		drawAim(dst, float32(mx), float32(my))
		return
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(mx), float64(my)) // остриё стрелки — сама точка
	dst.DrawImage(arrow(), op)
}

// drawAim — прицел с двойной обводкой: читается на любом фоне.
func drawAim(dst *ebiten.Image, x, y float32) {
	line := color.RGBA{0xf2, 0xf6, 0xff, 0xff}
	shadow := color.RGBA{0x08, 0x0a, 0x10, 0xd0}

	vector.StrokeCircle(dst, x, y, 6, 2, shadow, true)
	vector.StrokeCircle(dst, x, y, 6, 1, line, true)
	for _, t := range [][4]float32{{-9, 0, -4, 0}, {9, 0, 4, 0}, {0, -9, 0, -4}, {0, 9, 0, 4}} {
		vector.StrokeLine(dst, x+t[0], y+t[1], x+t[2], y+t[3], 2, shadow, true)
		vector.StrokeLine(dst, x+t[0], y+t[1], x+t[2], y+t[3], 1, line, true)
	}
	vector.FillCircle(dst, x, y, 1, line, true)
}
