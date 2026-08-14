package scene

import (
	"image/color"
	"math"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/engine"
	"github.com/vladislav/game/internal/ui"
)

// Обратная связь боя: сколько живёт число урона, как долго трясёт экран и
// светится край после удара по игроку.
const (
	popTicks = 45
	popScale = 2 // число урона мельче просто не заметить
	// Взятый уровень висит дольше и крупнее урона: это событие забега, а не
	// одна из десятка цифр, мелькающих в драке.
	levelTicks = 100
	levelScale = 3
	shakeTicks = 12
	flashTicks = 20
	shakeAmp   = 3 // размах тряски в пикселях
	slashTicks = 12
)

var (
	fxDamage = color.RGBA{0xff, 0xe9, 0xa8, 0xff} // урон, нанесённый игроком
	fxHurt   = color.RGBA{0xff, 0x6a, 0x5a, 0xff} // урон, полученный игроком
	fxEdge   = color.RGBA{0xc8, 0x28, 0x28, 0xff} // свечение края экрана
	fxSlash  = color.RGBA{0xff, 0xf4, 0xd0, 0xff} // след замаха
	fxXP     = color.RGBA{0x8c, 0xd8, 0xff, 0xff} // полученный опыт
	fxNoXP   = color.RGBA{0x7a, 0x84, 0x98, 0xff} // добыча ниже героя — опыта нет
	fxLevel  = color.RGBA{0xff, 0xe4, 0x6a, 0xff} // взят уровень
)

// slash — след удара: тот самый сектор, которым игровой слой ищет цели.
// Нужен потому, что клип удара есть не у всех лоадаутов (безоружный бьёт из
// стойки), и без следа непонятно, случился ли замах и куда он пришёлся.
type slash struct {
	center engine.Vec2
	face   engine.Vec2
	reach  float64
	arc    float64 // раствор в градусах
	col    color.RGBA
	ttl    int
}

// popup — всплывающая надпись: всплывает над целью и тает. Числом урона она
// быть не обязана — тем же способом сообщается о подобранной добыче.
type popup struct {
	pos   engine.Vec2
	txt   string
	col   color.RGBA
	scale float64
	ttl   int
	max   int // сколько тиков жила изначально: по нему считается угасание
}

// effects — короткая обратная связь боя. Ничего не решает про игру: только
// показывает то, что уже случилось.
type effects struct {
	pops    []popup
	slashes []slash
	shake   int
	flash   int
	tick    int
}

// swing запоминает замах: сектор живёт несколько кадров и тает. Цвет говорит,
// чей это был удар: свой светлый, украденная сила — цвета своей стихии.
func (e *effects) swing(center, face engine.Vec2, reach, arc float64, col color.RGBA) {
	e.slashes = append(e.slashes, slash{
		center: center, face: face, reach: reach, arc: arc, col: col, ttl: slashTicks,
	})
}

// pop добавляет число урона над точкой pos.
func (e *effects) pop(pos engine.Vec2, dmg int, col color.RGBA) {
	if dmg <= 0 {
		return
	}
	e.pops = append(e.pops, popup{pos: pos, txt: strconv.Itoa(dmg), col: col, scale: popScale, ttl: popTicks, max: popTicks})
}

// xp показывает опыт за добитую тварь. Ноль тоже показывается, только серым:
// «эта добыча тебя переросла» — ответ на вопрос игрока, а молчание ответом не
// будет.
func (e *effects) xp(pos engine.Vec2, got int) {
	txt, col := "+"+strconv.Itoa(got), fxXP
	if got <= 0 {
		txt, col = "0", fxNoXP
	}
	e.pops = append(e.pops, popup{pos: pos, txt: txt, col: col, scale: popScale, ttl: popTicks, max: popTicks})
}

// level — взятый уровень над героем.
func (e *effects) level(pos engine.Vec2, lvl int) {
	e.pops = append(e.pops, popup{
		pos: pos, txt: "УРОВЕНЬ " + strconv.Itoa(lvl), col: fxLevel,
		scale: levelScale, ttl: levelTicks, max: levelTicks,
	})
}

// text добавляет надпись над точкой pos — так сообщается о подобранной добыче.
// Мельче числа урона: имя предмета в две клетки шрифта заняло бы полэкрана, да
// и спорить за внимание с боем ему незачем.
func (e *effects) text(pos engine.Vec2, txt string, col color.RGBA) {
	if txt == "" {
		return
	}
	e.pops = append(e.pops, popup{pos: pos, txt: txt, col: col, scale: 1, ttl: popTicks, max: popTicks})
}

// hurt — по игроку попали: тряска экрана и свечение по краю.
func (e *effects) hurt() {
	e.shake, e.flash = shakeTicks, flashTicks
}

func (e *effects) update() {
	e.tick++
	live := e.pops[:0]
	for _, p := range e.pops {
		p.ttl--
		p.pos.Y -= 0.4 // всплывает
		if p.ttl > 0 {
			live = append(live, p)
		}
	}
	e.pops = live
	liveS := e.slashes[:0]
	for _, s := range e.slashes {
		if s.ttl--; s.ttl > 0 {
			liveS = append(liveS, s)
		}
	}
	e.slashes = liveS
	if e.shake > 0 {
		e.shake--
	}
	if e.flash > 0 {
		e.flash--
	}
}

// offset — смещение камеры на этот кадр. Дребезг детерминированный (по тику), а
// не случайный: одинаковый удар выглядит одинаково.
func (e *effects) offset() engine.Vec2 {
	if e.shake <= 0 {
		return engine.Vec2{}
	}
	amp := float64(e.shake) / shakeTicks * shakeAmp
	sx, sy := 1.0, 1.0
	if e.tick%2 == 0 {
		sx = -1
	}
	if e.tick%4 < 2 {
		sy = -1
	}
	return engine.Vec2{X: amp * sx, Y: amp * sy * 0.6}
}

// drawPopups рисует числа урона в мировых координатах через камеру.
func (e *effects) drawPopups(dst *ebiten.Image, cam *engine.Camera) {
	for _, p := range e.pops {
		s := cam.WorldToScreen(p.pos)
		col := p.col
		// Последнюю треть жизни надпись тает — своей жизни, а не общей: взятый
		// уровень висит вдвое дольше числа урона.
		if fade := max(p.max, 3) / 3; p.ttl < fade {
			col.A = uint8(255 * p.ttl / fade)
		}
		// Тень в пиксель: надпись читается и на светлой траве, и на тёмной воде.
		ui.PixelTextCentered(dst, p.txt, s.X+1, s.Y+1, p.scale, color.RGBA{0, 0, 0, col.A})
		ui.PixelTextCentered(dst, p.txt, s.X, s.Y, p.scale, col)
	}
}

// drawSlashes рисует следы замахов — дугу по краю того самого сектора, в
// котором игровой слой ищет цели. Поэтому промах видно так же ясно, как
// попадание, а у безоружного это вообще единственный признак удара.
func (e *effects) drawSlashes(dst *ebiten.Image, cam *engine.Camera) {
	for _, s := range e.slashes {
		c := cam.WorldToScreen(s.center)
		base := s.face.Angle()
		half := s.arc / 2 * math.Pi / 180
		// Дуга подтягивается к концу сектора и тает: замах уходит вперёд.
		k := 1 - float64(s.ttl)/slashTicks
		r := s.reach * (0.55 + 0.45*k)

		var path vector.Path
		const steps = 16
		for i := 0; i <= steps; i++ {
			a := base - half + 2*half*float64(i)/steps
			x, y := float32(c.X+math.Cos(a)*r), float32(c.Y+math.Sin(a)*r)
			if i == 0 {
				path.MoveTo(x, y)
				continue
			}
			path.LineTo(x, y)
		}

		op := &vector.DrawPathOptions{AntiAlias: true}
		col := s.col
		if col == (color.RGBA{}) {
			col = fxSlash
		}
		op.ColorScale.ScaleWithColor(col)
		// Прозрачность задаётся только через ScaleAlpha: цвет с «сырой» альфой
		// Ebiten считает premultiplied и рисует почти непрозрачным.
		op.ColorScale.ScaleAlpha(float32(s.ttl) / slashTicks * 0.9)
		vector.StrokePath(dst, &path, &vector.StrokeOptions{Width: 3, LineCap: vector.LineCapRound}, op)
	}
}

// drawFlash подсвечивает края экрана после полученного удара — урон видно, даже
// когда смотришь не на героя.
func (e *effects) drawFlash(dst *ebiten.Image) {
	if e.flash <= 0 {
		return
	}
	col := fxEdge
	col.A = uint8(160 * e.flash / flashTicks)
	const t = 6 // толщина полосы
	vector.FillRect(dst, 0, 0, config.ScreenW, t, col, false)
	vector.FillRect(dst, 0, config.ScreenH-t, config.ScreenW, t, col, false)
	vector.FillRect(dst, 0, 0, t, config.ScreenH, col, false)
	vector.FillRect(dst, config.ScreenW-t, 0, t, config.ScreenH, col, false)
}
