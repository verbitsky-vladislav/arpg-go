package scene

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/settings"
	"github.com/vladislav/game/internal/ui"
)

// Геометрия двух колонок настроек (логическое разрешение 640×360). Экран и
// управление стоят рядом: список клавиш в один столбец с остальным уже не
// помещается по высоте.
const (
	setColW, setColGap = 280, 32
	setRowH            = 22
	setRowGap          = 5
	setColTop          = 84
	setMsgTicks        = 4 * config.TPS // сколько держится сообщение об обмене клавиш
)

// setRow — одна строка настройки: подпись, текущее значение и действие по
// Enter или клику. Значение читается из settings каждый кадр — источник правды
// один, копий состояния тут нет.
type setRow struct {
	label string
	// value — что показать справа и считать ли это «включённым» (для цвета).
	value func() (text string, on bool)
	do    func()
	off   func() string // "" — строка доступна
}

// Settings — экран настроек: слева экран, справа привязки клавиш. Сам ничего
// не применяет и не сохраняет: и то и другое делает пакет settings при каждом
// изменении.
type Settings struct {
	back Scene
	cols [2][]setRow
	col  int
	sel  [2]int

	bind     settings.Action // какое действие ждёт клавишу
	binding  bool
	msg      string
	msgLeft  int
	lastKeys []ebiten.Key // буфер для опроса нажатых клавиш, без аллокаций
}

// NewSettings собирает экран настроек.
func NewSettings(back Scene) *Settings {
	s := &Settings{back: back}
	onOff := func(v bool) (string, bool) {
		if v {
			return "ВКЛ", true
		}
		return "ВЫКЛ", false
	}

	s.cols[0] = []setRow{
		{
			label: "ПОЛНЫЙ ЭКРАН",
			value: func() (string, bool) { return onOff(settings.Get().Fullscreen) },
			do:    settings.ToggleFullscreen,
		},
		{
			label: "МАСШТАБ ОКНА",
			value: func() (string, bool) { return fmt.Sprintf("%dX", settings.Get().WindowScale), false },
			do:    settings.CycleWindowScale,
			off: func() string {
				if settings.Get().Fullscreen {
					return "ПОЛНЫЙ ЭКРАН"
				}
				return ""
			},
		},
		{
			label: "СИНХРОНИЗАЦИЯ",
			value: func() (string, bool) { return onOff(settings.Get().VSync) },
			do:    settings.ToggleVSync,
		},
		{
			label: "СЧЁТЧИК КАДРОВ",
			value: func() (string, bool) { return onOff(settings.Get().ShowFPS) },
			do:    settings.ToggleFPS,
		},
	}

	for _, a := range settings.Actions() {
		s.cols[1] = append(s.cols[1], setRow{
			label: a.Title(),
			value: func() (string, bool) { return settings.KeyLabel(settings.Key(a)), false },
			do:    func() { s.startBind(a) },
		})
	}
	s.cols[1] = append(s.cols[1], setRow{
		label: "СБРОСИТЬ",
		value: func() (string, bool) { return "КЛАВИШИ", false },
		do: func() {
			settings.ResetKeys()
			s.say("КЛАВИШИ ВЕРНУЛИСЬ К ЗАВОДСКИМ")
		},
	})
	return s
}

// startBind переводит строку в ожидание клавиши.
func (s *Settings) startBind(a settings.Action) {
	s.bind, s.binding = a, true
	s.msg, s.msgLeft = "", 0
}

// say показывает сообщение внизу экрана на несколько секунд.
func (s *Settings) say(m string) { s.msg, s.msgLeft = m, setMsgTicks }

func (s *Settings) Update() (Scene, error) {
	if s.msgLeft > 0 {
		s.msgLeft--
	}
	if s.binding {
		return s.captureKey()
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		if s.back != nil {
			return s.back, nil
		}
		return NewMenu(), nil
	}
	// Полный экран удобно дёргать и напрямую — F11 привычнее, чем идти в список.
	if inpututil.IsKeyJustPressed(ebiten.KeyF11) {
		settings.ToggleFullscreen()
		return s, nil
	}

	mx, my := ebiten.CursorPosition()
	hovered := false
	for c := range s.cols {
		for i := range s.cols[c] {
			if x, y, w, h := s.rowRect(c, i); inRect(mx, my, x, y, w, h) {
				s.col, s.sel[c], hovered = c, i, true
			}
		}
	}

	if keyPressed(ebiten.KeyDown, ebiten.KeyS) {
		s.sel[s.col] = (s.sel[s.col] + 1) % len(s.cols[s.col])
	}
	if keyPressed(ebiten.KeyUp, ebiten.KeyW) {
		s.sel[s.col] = (s.sel[s.col] - 1 + len(s.cols[s.col])) % len(s.cols[s.col])
	}
	if keyPressed(ebiten.KeyLeft, ebiten.KeyRight, ebiten.KeyA, ebiten.KeyD) {
		s.col = 1 - s.col
		s.sel[s.col] = min(s.sel[s.col], len(s.cols[s.col])-1)
	}

	act := keyPressed(ebiten.KeyEnter, ebiten.KeyNumpadEnter, ebiten.KeySpace)
	if hovered && inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		act = true
	}
	if !act {
		return s, nil
	}
	if r := s.cols[s.col][s.sel[s.col]]; r.off == nil || r.off() == "" {
		r.do()
	}
	return s, nil
}

// captureKey ждёт клавишу для выбранного действия. ESC отменяет: иначе выйти из
// ожидания можно было бы только назначив что-нибудь.
func (s *Settings) captureKey() (Scene, error) {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		s.binding = false
		return s, nil
	}
	s.lastKeys = inpututil.AppendJustPressedKeys(s.lastKeys[:0])
	for _, k := range s.lastKeys {
		if k == ebiten.KeyEscape {
			continue
		}
		// Клавиша занята другим действием — Bind поменяет их местами, но игрок
		// должен узнать об этом сразу, а не обнаружить потом в бою.
		if other, taken := settings.Owner(k); taken && other != s.bind {
			s.say(fmt.Sprintf("%s БЫЛА У ДЕЙСТВИЯ %s - ПОМЕНЯЛИСЬ МЕСТАМИ",
				settings.KeyLabel(k), other.Title()))
		}
		settings.Bind(s.bind, k)
		s.binding = false
		break
	}
	return s, nil
}

func (s *Settings) Draw(screen *ebiten.Image) {
	screen.Fill(menuBG)
	for y := 0; y < config.ScreenH; y += 3 {
		vector.FillRect(screen, 0, float32(y), config.ScreenW, 1, menuScan, false)
	}
	ui.PixelTextCentered(screen, "НАСТРОЙКИ", config.ScreenW/2, 26, 3, menuTitle)
	vector.FillRect(screen, config.ScreenW/2-110, 60, 220, 1, menuFrame, false)

	for c, title := range [2]string{"ЭКРАН", "УПРАВЛЕНИЕ"} {
		x, _, w, _ := s.rowRect(c, 0)
		ui.PixelTextCentered(screen, title, float64(x+w/2), setColTop-18, 2, menuEdgeSel)
		for i := range s.cols[c] {
			s.drawRow(screen, c, i)
		}
	}

	ui.PixelTextCentered(screen, s.hint(), config.ScreenW/2, config.ScreenH-22, 1, menuEdge)
}

// hint — строка внизу: что делать сейчас. В ожидании клавиши она объясняет
// ожидание, после обмена клавиш — что произошло.
func (s *Settings) hint() string {
	cur := s.cols[s.col][s.sel[s.col]]
	switch {
	case s.binding:
		return "НАЖМИТЕ КЛАВИШУ ДЛЯ ДЕЙСТВИЯ " + s.bind.Title() + ",  ESC - ОТМЕНА"
	case s.msgLeft > 0:
		return s.msg
	case cur.off != nil && cur.off() != "":
		return cur.label + " НЕДОСТУПЕН: " + cur.off()
	}
	return "ESC - НАЗАД,  СТРЕЛКИ - ВЫБОР,  ENTER ИЛИ ЛКМ - ИЗМЕНИТЬ,  F11 - ПОЛНЫЙ ЭКРАН"
}

func (s *Settings) drawRow(dst *ebiten.Image, c, i int) {
	r := s.cols[c][i]
	x, y, w, h := s.rowRect(c, i)
	sel := c == s.col && i == s.sel[c]
	disabled := ""
	if r.off != nil {
		disabled = r.off()
	}

	plate, edge, label := menuPlate, menuEdge, menuText
	if sel {
		plate, edge, label = menuPlateSel, menuEdgeSel, menuTextSel
	}
	if disabled != "" {
		label = menuEdge
	}
	vector.FillRect(dst, x, y, w, h, plate, false)
	vector.StrokeRect(dst, x+0.5, y+0.5, w-1, h-1, 1, edge, false)
	for _, p := range [][2]float32{{x, y}, {x + w - 1, y}, {x, y + h - 1}, {x + w - 1, y + h - 1}} {
		vector.FillRect(dst, p[0], p[1], 1, 1, menuBG, false)
	}

	ty := float64(y) + (setRowH-ui.PixelTextHeight(2))/2
	ui.PixelText(dst, r.label, float64(x)+8, ty, 2, label)

	text, on := r.value()
	col := menuText
	switch {
	case disabled != "":
		// Причина недоступности не влезает рядом с подписью — ставим прочерк,
		// а объяснение уходит в подсказку внизу, когда строка выбрана.
		text, col = "—", menuEdge
	case s.binding && c == 1 && i == int(s.bind):
		text, col = "ЖДУ...", menuEdgeSel
	case on:
		col = menuEdgeSel
	}
	ui.PixelText(dst, text, float64(x+w)-8-ui.PixelTextWidth(text, 2), ty, 2, col)
}

// rowRect — рамка i-й строки колонки c.
func (s *Settings) rowRect(c, i int) (x, y, w, h float32) {
	x = float32(config.ScreenW-2*setColW-setColGap)/2 + float32(c)*(setColW+setColGap)
	return x, setColTop + float32(i*(setRowH+setRowGap)), setColW, setRowH
}
