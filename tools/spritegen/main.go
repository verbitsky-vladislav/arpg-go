// Инструмент spritegen — генератор покадровых анимаций героев из статичного
// base.png. НЕ часть игрового бинарника (отдельный package main). Только
// стандартная библиотека Go, без внешних зависимостей.
//
// Что делает: для каждого героя берёт assets/sprites/heroes/<id>/base.png
// (256×256, нативная сетка 32×32 ×8) как вид СПЕРЕДИ (down), строит из него
// вид СЗАДИ (up) — закрывая лицо цветом головного убора — и для каждого вида
// печёт по 16 кадров на действие (idle/walk/attack/punch/take_damage/loot)
// обратным попиксельным преобразованием (боб, наклон, присед, подъём ноги,
// удар, красная вспышка урона), апскейлит ×8 и раскладывает в
// <id>/<dir>/<action>/frame_00.png … frame_15.png  (dir ∈ {down, up}).
//
// Направления лево/право в игре — это ФЛИП фронта (down), отдельного профиля
// (side) нет: настоящий профиль деформацией из фронта не построить (см.
// docs/design/art-direction.md), это ручная дорисовка на будущее.
//
// Запуск из корня репозитория:
//
//	go run ./tools/spritegen assets/sprites/heroes
//
// Меняешь base.png героя — перегоняешь кадры этой командой.
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	native = 32 // нативная пиксель-сетка героя
	scale  = 8  // апскейл до 256x256
	frames = 16 // 16-кадровые анимации (плавнее)
)

var heroes = []string{"assassin", "barbarian", "engineer", "knight", "mage", "ranger"}
var anims = []string{"idle", "walk", "attack", "punch", "take_damage", "loot"}
var dirs = []string{"down", "up", "side"}

// grid — нативный 32x32 кадр в RGBA.
type grid struct {
	px [native][native]color.RGBA
}

func (g *grid) at(x, y int) color.RGBA {
	if x < 0 || y < 0 || x >= native || y >= native {
		return color.RGBA{}
	}
	return g.px[y][x]
}

func (g *grid) clone() *grid { c := *g; return &c }

// loadNative читает 256x256 PNG и понижает до нативной 32x32 сетки (центр блока).
func loadNative(path string) (*grid, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	src, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	g := &grid{}
	for y := 0; y < native; y++ {
		for x := 0; x < native; x++ {
			sx := b.Min.X + x*scale + scale/2
			sy := b.Min.Y + y*scale + scale/2
			r, gg, bb, a := src.At(sx, sy).RGBA()
			g.px[y][x] = color.RGBA{uint8(r >> 8), uint8(gg >> 8), uint8(bb >> 8), uint8(a >> 8)}
		}
	}
	return g, nil
}

// bounds возвращает непрозрачный bbox и производные ориентиры.
type marks struct {
	minX, minY, maxX, maxY int
	cx, gy, hipY           int
}

func (g *grid) marks() marks {
	m := marks{minX: native, minY: native, maxX: -1, maxY: -1}
	for y := 0; y < native; y++ {
		for x := 0; x < native; x++ {
			if g.px[y][x].A > 8 {
				if x < m.minX {
					m.minX = x
				}
				if y < m.minY {
					m.minY = y
				}
				if x > m.maxX {
					m.maxX = x
				}
				if y > m.maxY {
					m.maxY = y
				}
			}
		}
	}
	if m.maxY < 0 { // пусто
		m.minX, m.minY, m.maxX, m.maxY = 0, 0, native-1, native-1
	}
	m.cx = (m.minX + m.maxX) / 2
	m.gy = m.maxY
	h := m.maxY - m.minY
	m.hipY = m.maxY - int(math.Round(0.32*float64(h))) // низ ~1/3 — ноги
	return m
}

// --- Вид сзади (up) ---

// upParam — доли высоты bbox: полоса лица (faceTop..faceBot) и уровень, откуда
// брать цвет заливки (sample — обычно волосы/убор над лицом). Если fill задан —
// вся полоса заливается этим цветом (когда над лицом нет плотного убора и
// «протяжка сверху» размазывает кожу — как у инженера).
type upParam struct {
	faceTop, faceBot, sample float64
	fill                     string
}

var upParams = map[string]upParam{
	"knight":    {faceTop: 0.30, faceBot: 0.46, sample: 0.24},
	"barbarian": {faceTop: 0.16, faceBot: 0.46, sample: 0.06}, // накрыть глаза под повязкой
	"ranger":    {faceTop: 0.28, faceBot: 0.46, sample: 0.20},
	"assassin":  {faceTop: 0.30, faceBot: 0.48, sample: 0.22},
	"mage":      {faceTop: 0.30, faceBot: 0.48, sample: 0.24},
	"engineer":  {faceTop: 0.05, faceBot: 0.50, sample: 0.10, fill: "#545c68"}, // затылок серой кепки (и очки)
}

func isOutline(c color.RGBA) bool {
	mx := c.R
	if c.G > mx {
		mx = c.G
	}
	if c.B > mx {
		mx = c.B
	}
	return c.A > 8 && mx < 64
}

// viewUp строит вид сзади: в полосе лица заливает всю внутренность (между
// левым и правым краем силуэта строки) цветом убора/волос сверху, убирая глаза
// и черты. Крайние пиксели строки (силуэтный контур) сохраняются. Надёжно вне
// зависимости от цвета глаз (не полагаемся на классификацию контура).
func viewUp(src *grid, m marks, p upParam) *grid {
	out := src.clone()
	H := float64(m.maxY - m.minY)
	fTop := m.minY + int(math.Round(p.faceTop*H))
	fBot := m.minY + int(math.Round(p.faceBot*H))
	_ = p.sample
	// Явный цвет заливки (если задан) — сплошной, когда над лицом нет плотного
	// убора и «протяжка сверху» размазывала бы кожу.
	var fixed color.RGBA
	useFixed := p.fill != ""
	if useFixed {
		fixed = hx(p.fill)
	}
	// Иначе — цвет на столбец: первый непрозрачный пиксель, сканируя ВВЕРХ от
	// строки над полосой лица (линия волос/убора), тянем его вниз.
	fillAt := func(x int) (color.RGBA, bool) {
		if useFixed {
			return fixed, true
		}
		for y := fTop - 1; y >= m.minY; y-- {
			if c := src.at(x, y); c.A > 8 {
				return c, true
			}
		}
		return color.RGBA{}, false
	}
	for y := fTop; y <= fBot; y++ {
		// края силуэта в строке
		l, r := -1, -1
		for x := m.minX; x <= m.maxX; x++ {
			if src.at(x, y).A > 8 {
				if l < 0 {
					l = x
				}
				r = x
			}
		}
		if l < 0 {
			continue
		}
		for x := l + 1; x < r; x++ { // нутро, без крайних (контур)
			if src.at(x, y).A <= 8 {
				continue
			}
			if f, ok := fillAt(x); ok {
				if useFixed && (x <= l+2 || y >= fBot-1) {
					f = shade(f, 0.80) // форма: тень по спине (левый край) и низу
				}
				out.px[y][x] = f
			}
		}
	}
	return out
}

// --- Вид сбоку (side): профиль лицом ВПРАВО, нарисованный параметрически ---
//
// Настоящий профиль нельзя получить деформацией фронта (другой силуэт), поэтому
// он рисуется как пиксель-арт из палитры героя (sideParams). Лево = флип этого
// профиля в отрисовке. Голова в профиль (один глаз + нос), оружие в передней
// руке, авто-контур по силуэту.

func hx(s string) color.RGBA {
	s = strings.TrimPrefix(s, "#")
	r, _ := strconv.ParseUint(s[0:2], 16, 8)
	g, _ := strconv.ParseUint(s[2:4], 16, 8)
	b, _ := strconv.ParseUint(s[4:6], 16, 8)
	return color.RGBA{uint8(r), uint8(g), uint8(b), 255}
}

func (g *grid) set(x, y int, c color.RGBA) {
	if x >= 0 && y >= 0 && x < native && y < native {
		g.px[y][x] = c
	}
}
func (g *grid) rect(x0, y0, x1, y1 int, c color.RGBA) {
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			g.set(x, y, c)
		}
	}
}
func (g *grid) disc(cx, cy, r int, c color.RGBA) {
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			dx, dy := x-cx, y-cy
			if dx*dx+dy*dy <= r*r+r/2 {
				g.set(x, y, c)
			}
		}
	}
}

// addOutline красит прозрачные пиксели, соседние с заливкой, цветом контура.
func (g *grid) addOutline(o color.RGBA) {
	snap := *g
	filled := func(x, y int) bool { return snap.at(x, y).A > 0 }
	for y := 0; y < native; y++ {
		for x := 0; x < native; x++ {
			if snap.at(x, y).A > 0 {
				continue
			}
			if filled(x-1, y) || filled(x+1, y) || filled(x, y-1) || filled(x, y+1) ||
				filled(x-1, y-1) || filled(x+1, y-1) || filled(x-1, y+1) || filled(x+1, y+1) {
				g.set(x, y, o)
			}
		}
	}
}

type sideDef struct {
	outline, skin, skinSh string
	body, bodySh, legs    string
	hair                  string // затылок/волосы под убором
	hat                   string // hood|helmet|widehat|band|cap
	hatC, hatC2           string
	eye                   string
	weapon                string // sword|hammer|bow|dagger|staff|wrench
	wpC, wpC2             string
	accent                string
	robe                  bool // расклешённая мантия вместо ног (маг)
}

func buildSide(p sideDef) *grid {
	g := &grid{}
	O := hx(p.outline)
	skin, skinSh := hx(p.skin), hx(p.skinSh)
	body, bodySh := hx(p.body), hx(p.bodySh)
	legs := hx(p.legs)
	hair := hx(p.hair)

	if p.robe { // ноги/мантия
		for y := 22; y <= 30; y++ {
			hw := 3 + (y - 22)
			g.rect(15-hw, y, 15+hw, y, body)
		}
		g.rect(9, 30, 21, 30, bodySh)
	} else {
		g.rect(12, 24, 14, 30, legs)
		g.rect(16, 24, 18, 30, legs)
		g.rect(12, 30, 15, 30, O)
		g.rect(16, 30, 19, 30, O)
	}

	g.rect(11, 14, 19, 24, body)         // торс
	g.rect(11, 14, 12, 24, bodySh)       // тень спины
	g.rect(11, 21, 19, 21, hx(p.accent)) // пояс

	g.rect(18, 15, 20, 17, body) // передняя рука
	g.rect(20, 16, 21, 19, skin) // кисть

	g.disc(14, 8, 6, skin) // голова (лицо)
	for y := 2; y <= 13; y++ {
		for x := 8; x <= 20; x++ {
			if g.at(x, y).A == 0 {
				continue
			}
			if x <= 14 || y <= 5 { // затылок/макушка — волосы
				g.set(x, y, hair)
			}
		}
	}
	g.set(18, 12, skinSh)
	g.set(19, 11, skinSh)
	g.set(20, 9, skin) // нос
	g.set(20, 10, skin)
	eye := O
	if p.eye != "" {
		eye = hx(p.eye)
	}
	g.set(18, 8, eye) // глаз

	switch p.hat {
	case "hood":
		hc := hx(p.hatC)
		for y := 1; y <= 14; y++ {
			for x := 7; x <= 20; x++ {
				if x <= 13 || y <= 4 {
					if g.at(x, y).A > 0 || (x <= 12 && y >= 5 && y <= 14) {
						g.set(x, y, hc)
					}
				}
			}
		}
		g.set(9, 2, hc)
		g.set(10, 1, hc)
		g.rect(8, 13, 12, 16, hc)
	case "helmet":
		hc, hc2 := hx(p.hatC), hx(p.hatC2)
		for y := 2; y <= 9; y++ {
			for x := 8; x <= 20; x++ {
				if g.at(x, y).A > 0 && (y <= 4 || x <= 13) {
					g.set(x, y, hc)
				}
			}
		}
		g.rect(15, 5, 20, 5, hc2)
		g.set(20, 6, hc2)
		g.rect(12, 0, 14, 4, hc2)
	case "widehat":
		hc, hc2 := hx(p.hatC), hx(p.hatC2)
		g.rect(6, 6, 23, 7, hc)
		g.rect(7, 7, 22, 7, hc)
		g.rect(11, 1, 17, 6, hc)
		g.rect(12, 0, 16, 1, hc2)
	case "band":
		g.rect(9, 6, 20, 7, hx(p.hatC))
	case "cap":
		hc, hc2 := hx(p.hatC), hx(p.hatC2)
		for y := 2; y <= 6; y++ {
			for x := 8; x <= 20; x++ {
				if g.at(x, y).A > 0 {
					g.set(x, y, hc)
				}
			}
		}
		g.rect(19, 6, 22, 7, hc)
		g.rect(17, 8, 19, 9, hc2)
	}

	drawWeapon(g, p)
	g.addOutline(O)
	return g
}

func drawWeapon(g *grid, p sideDef) {
	switch p.weapon {
	case "sword":
		g.rect(22, 5, 22, 18, hx(p.wpC))
		g.rect(23, 6, 23, 17, hx(p.wpC2))
		g.rect(21, 18, 24, 18, hx(p.accent))
	case "hammer":
		g.rect(22, 8, 22, 22, hx("#563c22"))
		g.rect(20, 6, 24, 10, hx(p.wpC))
		g.rect(20, 6, 24, 7, hx(p.wpC2))
	case "bow":
		c := hx(p.wpC)
		for i := 0; i <= 12; i++ {
			g.set(23-(i*(12-i))/12, 6+i, c)
		}
		g.rect(24, 6, 24, 18, hx(p.wpC2))
	case "dagger":
		g.rect(22, 14, 22, 20, hx(p.wpC))
		g.set(21, 20, hx(p.accent))
	case "staff":
		g.rect(22, 4, 22, 24, hx(p.wpC))
		g.disc(23, 4, 2, hx(p.wpC2))
		g.set(23, 3, hx(p.accent))
	case "wrench":
		g.rect(22, 12, 22, 20, hx(p.wpC))
		g.rect(21, 11, 23, 12, hx(p.wpC))
		g.set(21, 11, hx(p.accent))
	}
}

var sideParams = map[string]sideDef{
	"knight": {outline: "#1c2036", skin: "#f2c496", skinSh: "#cda078",
		body: "#6482be", bodySh: "#4a5678", legs: "#4a5678", hair: "#6482be",
		hat: "helmet", hatC: "#6482be", hatC2: "#ffce4a", eye: "#1c2036",
		weapon: "sword", wpC: "#ffffff", wpC2: "#dfe8f4", accent: "#ffce4a"},
	"barbarian": {outline: "#221812", skin: "#e0a878", skinSh: "#b87e4f",
		body: "#e0a878", bodySh: "#b87e4f", legs: "#7a5834", hair: "#3a281a",
		hat: "band", hatC: "#c43c2e", eye: "#c43c2e",
		weapon: "hammer", wpC: "#788496", wpC2: "#b6c2d0", accent: "#563c22"},
	"ranger": {outline: "#163624", skin: "#f2c496", skinSh: "#cda078",
		body: "#46b464", bodySh: "#2c8048", legs: "#78522e", hair: "#369654",
		hat: "hood", hatC: "#369654", eye: "#163624",
		weapon: "bow", wpC: "#78522e", wpC2: "#e1e1c8", accent: "#966434"},
	"assassin": {outline: "#141228", skin: "#f2c496", skinSh: "#cda078",
		body: "#3d3566", bodySh: "#2a2547", legs: "#2a2547", hair: "#3d3566",
		hat: "hood", hatC: "#3d3566", eye: "#46e0c8",
		weapon: "dagger", wpC: "#d0d8e8", wpC2: "#d0d8e8", accent: "#e6c85a"},
	"mage": {outline: "#1e2246", skin: "#f2c496", skinSh: "#cda078",
		body: "#4a80e0", bodySh: "#2e58b0", legs: "#2e58b0", hair: "#f2c496",
		hat: "widehat", hatC: "#ca9c2e", hatC2: "#ffce4a", eye: "#1e2246",
		weapon: "staff", wpC: "#82542c", wpC2: "#ff8c28", accent: "#ca9c2e", robe: true},
	"engineer": {outline: "#221e18", skin: "#f2c496", skinSh: "#cda078",
		body: "#7a4f2a", bodySh: "#55371d", legs: "#545c68", hair: "#545c68",
		hat: "cap", hatC: "#545c68", hatC2: "#3fd0e0", eye: "#221e18",
		weapon: "wrench", wpC: "#9aa4b2", wpC2: "#687280", accent: "#c98a3a"},
}

// params — параметры трансформации одного кадра.
type params struct {
	tx, ty  float64 // сдвиг (px), ty>0 = вниз
	shear   float64 // наклон: верх сдвигается на shear относительно земли (>0 назад/вправо)
	squashY float64 // вертикальный масштаб относительно земли (1=нет, <1 присед)
	legLift float64 // подъём «шагающей» ноги (px)
	legSide int     // -1 левая, +1 правая, 0 нет
	tint    color.RGBA
	tintAmt float64
}

// transform строит нативный кадр по параметрам через обратное отображение (nearest).
func transform(src *grid, m marks, p params) *grid {
	out := &grid{}
	gy := float64(m.gy)
	for y := 0; y < native; y++ {
		for x := 0; x < native; x++ {
			fx := float64(x) - p.tx
			fy := float64(y) - p.ty
			// обратный вертикальный присед относительно земли
			if p.squashY != 0 && p.squashY != 1 {
				fy = gy - (gy-fy)/p.squashY
			}
			// обратный наклон: сдвиг тем сильнее, чем выше над землёй
			if p.shear != 0 && gy > 0 {
				fx -= p.shear * (gy - fy) / gy
			}
			// шаг: поднять ногу выбранной стороны
			if p.legLift != 0 && fy > float64(m.hipY) {
				side := -1
				if fx >= float64(m.cx) {
					side = 1
				}
				if side == p.legSide {
					fy += p.legLift // источник выше -> нога поднята
					fx -= p.legLift * 0.3 * float64(p.legSide)
				}
			}
			sx := int(math.Round(fx))
			sy := int(math.Round(fy))
			c := src.at(sx, sy)
			if c.A > 0 && p.tintAmt > 0 {
				c = blend(c, p.tint, p.tintAmt)
			}
			out.px[y][x] = c
		}
	}
	return out
}

func shade(c color.RGBA, k float64) color.RGBA {
	return color.RGBA{uint8(float64(c.R) * k), uint8(float64(c.G) * k), uint8(float64(c.B) * k), c.A}
}

func blend(a, b color.RGBA, t float64) color.RGBA {
	lerp := func(x, y uint8) uint8 { return uint8(float64(x)*(1-t) + float64(y)*t) }
	return color.RGBA{lerp(a.R, b.R), lerp(a.G, b.G), lerp(a.B, b.B), a.A}
}

// --- Наборы кадров по анимациям ---

func framesFor(anim string) []params {
	out := make([]params, frames)
	for k := 0; k < frames; k++ {
		ph := 2 * math.Pi * float64(k) / float64(frames)
		switch anim {
		case "idle": // мягкое дыхание: подъём корпуса ~1px
			rise := (1 - math.Cos(ph)) / 2 // 0..1..0
			out[k] = params{ty: -1.2 * rise, squashY: 1 - 0.04*rise, shear: 0.3 * math.Sin(ph)}
		case "walk": // шаг: по одному чистому подъёму каждой ноги за цикл
			half := frames / 2
			side := -1   // первая половина цикла — левая нога
			local := 0.0 // 0..1 внутри текущего полушага
			if k < half {
				local = float64(k) / float64(half)
			} else {
				side = 1
				local = float64(k-half) / float64(half)
			}
			step := math.Sin(math.Pi * local) // 0..1..0 — один плавный подъём
			out[k] = params{
				ty:      -1.2 * step, // корпус приподнимается в середине шага
				tx:      0.7 * math.Sin(ph),
				shear:   -0.8, // лёгкий наклон вперёд
				legLift: 2.2 * step,
				legSide: side,
			}
		case "attack": // замах -> выпад -> возврат (фазы масштабируются под frames)
			wind := frames * 3 / 8 // конец замаха
			hit := frames * 5 / 8  // конец удара
			switch {
			case k < wind: // замах назад
				a := float64(k) / float64(wind-1)
				out[k] = params{shear: 2.0 * a, ty: -0.6 * a, squashY: 1 - 0.03*a}
			case k < hit: // удар: от замаха к выпаду вперёд/к камере
				a := float64(k-wind) / float64(hit-wind)
				out[k] = params{
					shear:   2.0 + (-3.5-2.0)*a,
					ty:      -0.6 + (1.2-(-0.6))*a,
					squashY: 1 + 0.06*a,
				}
			default: // плавный возврат в нейтраль
				a := float64(k-hit) / float64(frames-hit)
				out[k] = params{shear: -3.5 * (1 - a), ty: 1.2 * (1 - a)}
			}
		case "punch": // короткий прямой удар вперёд и возврат
			half := frames / 2
			if k < half {
				a := float64(k) / float64(half-1) // выпад
				out[k] = params{shear: -3.0 * a, ty: 1.0 * a, squashY: 1 + 0.05*a}
			} else {
				a := float64(k-half) / float64(frames-half) // возврат
				out[k] = params{shear: -3.0 * (1 - a), ty: 1.0 * (1 - a)}
			}
		case "take_damage": // отдача назад + тряска + красная вспышка, затухание
			decay := float64(frames-1-k) / float64(frames-1) // 1..0
			shake := 0.0
			if k < frames/4 { // тряска в первой четверти
				if k%2 == 0 {
					shake = 1
				} else {
					shake = -1
				}
			}
			out[k] = params{
				tx:      2.0*decay + shake,
				ty:      -0.8 * decay,
				shear:   2.2 * decay,
				tint:    color.RGBA{255, 60, 50, 255},
				tintAmt: 0.75 * decay,
			}
		case "loot": // присесть/нагнуться и подобрать, затем встать
			t := (1 - math.Cos(ph)) / 2 // 0..1..0, минимум приседа в середине
			out[k] = params{
				squashY: 1 - 0.28*t,
				shear:   -3.2 * t, // тянется вперёд
				ty:      1.4 * t,
			}
		}
	}
	return out
}

// upscale превращает нативный кадр в 256x256 PNG-изображение.
func upscale(g *grid) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, native*scale, native*scale))
	for y := 0; y < native; y++ {
		for x := 0; x < native; x++ {
			c := g.px[y][x]
			for dy := 0; dy < scale; dy++ {
				for dx := 0; dx < scale; dx++ {
					img.Set(x*scale+dx, y*scale+dy, c)
				}
			}
		}
	}
	return img
}

func savePNG(path string, img *image.RGBA) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func main() {
	outRoot := os.Args[1] // assets/sprites/heroes
	for _, hero := range heroes {
		// Источник — канонический base.png (вид спереди / down).
		down, err := loadNative(filepath.Join(outRoot, hero, "base.png"))
		if err != nil {
			fmt.Println("skip", hero, err)
			continue
		}
		m := down.marks()
		views := map[string]*grid{
			"down": down,
			"up":   viewUp(down, m, upParams[hero]), // спина: тот же силуэт
			"side": buildSide(sideParams[hero]),     // профиль: свой силуэт
		}
		total := 0
		for _, dir := range dirs {
			src := views[dir]
			mv := src.marks() // ориентиры (земля/бёдра) — под силуэт вида
			for _, anim := range anims {
				for k, p := range framesFor(anim) {
					fr := transform(src, mv, p)
					name := fmt.Sprintf("frame_%02d.png", k)
					out := filepath.Join(outRoot, hero, dir, anim, name)
					if err := savePNG(out, upscale(fr)); err != nil {
						fmt.Println("err", hero, dir, anim, k, err)
					}
					total++
				}
			}
		}
		fmt.Printf("%-10s -> %d кадров (%d вид × %d действий)\n", hero, total, len(dirs), len(anims))
	}
}
