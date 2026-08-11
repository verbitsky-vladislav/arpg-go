// Инструмент weavergen — процедурный генератор спрайтов второго босса
// «Ткачиха» (weaver), гигантская паучиха-матка. Отдельный package main, только
// stdlib, НЕ часть игрового бинарника (как tools/bossgen и tools/spritegen).
//
// СТИЛЬ — принципиально иной, чем у world_eater. world_eater нарисован в
// сайд-вью (Terraria / Hollow Knight): плоский силуэт с контуром. Ткачиха
// нарисована в НАКЛОННОМ ВИДЕ СВЕРХУ ~65° (референс — Necesse): объёмные формы
// с мягкой растушёвкой, видна верхняя поверхность существа и немного передней.
//
// ЕДИНЫЙ СВЕТ. Источник один для всех спрайтов, всех направлений и всех кадров:
// сверху и чуть спереди-слева, в ЭКРАННЫХ координатах (см. lightDir). Он НЕ
// поворачивается вместе с направлением героя — именно поэтому направления
// рисуются честно, а не поворотом/отражением базовой картинки (см. ниже).
//
// НОГИ. Тело и ноги — РАЗДЕЛЬНЫЕ спрайты (вариант 2 из ТЗ): тело баз-ориентации,
// 8 ног как 3-сегментные конечности (coxa/femur/tibia) с пивотами в суставах,
// которые игровая процедурная IK-походка переставляет сама. Поэтому тело почти
// не зависит от направления, а ноги «шагают» и снимают проблему 8 направлений.
// В weaver.json выдаются leg_attach_points (8 точек на теле) и leg_segments
// (длины сегментов + пивоты).
//
// Запуск из корня репозитория:
//
//	go run ./tools/weavergen assets/sprites/bosses/weaver
//
// На выходе: body/ legs/ spiderling/ eggsac/ fx/ shadows/ со спрайтшитами,
// palette_additions.hex и weaver.json (метаданные всех анимаций: pivot, hitbox
// по кадрам, weakpoint_box, height_offset, leg_attach_points, leg_segments).
package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
)

func rgb(r, g, b uint8) color.RGBA { return color.RGBA{r, g, b, 255} }

// ─────────────────────────────────────────────────────────────────────────────
// ПАЛИТРА. База наследуется от world_eater (те же хитин/плоть/кислота/тень).
// Новых оттенков ровно 4 (лимит ТЗ — не более 4): три ступени ШЁЛКА и один
// глубокий тон ЯДА, отличный от кислоты первого босса. Стиль Necesse —
// приглушённый, землистый: хитин уводим в пыльно-фиолетовый, добавляем оливку.
// ─────────────────────────────────────────────────────────────────────────────

var (
	Outline = rgb(0x1b, 0x14, 0x2a) // 1px контур — тёмно-фиолетовый, НЕ чёрный

	// Хитин (наследие world_eater): иссиня-чёрный → пыльно-фиолетовый.
	// Рампа из 6 значений для сферы брюшка (ТЗ требует ≥6 на брюшко).
	ChitinSh2 = rgb(0x0e, 0x0b, 0x18) // глубокая тень (глубже, чем у червя)
	ChitinSh  = rgb(0x14, 0x11, 0x20)
	ChitinDk  = rgb(0x24, 0x1b, 0x38)
	ChitinMd  = rgb(0x3b, 0x2b, 0x57)
	ChitinLt  = rgb(0x56, 0x3f, 0x7d)
	ChitinHi  = rgb(0x7c, 0x5e, 0xa8)

	// Плоть/кость (наследие): бледно-кремовая — жвала, вросшая кисть, лицо.
	FleshSh = rgb(0x9a, 0x80, 0x6c)
	FleshMd = rgb(0xcf, 0xb6, 0x95)
	FleshLt = rgb(0xf2, 0xe6, 0xc8)

	// Яд (НОВОЕ, 1 оттенок-ядро) + производные тона строятся смешением с плотью.
	// Отличается от кислоты червя: не едко-салатовый, а болотно-жёлто-зелёный.
	VenomCore = rgb(0x9e, 0xb8, 0x2e) // NEW #1 — ядро яда/свечения выводка

	// Шёлк/паутина (НОВОЕ, 3 оттенка): холодный серо-фиолетовый, почти
	// бескровный — нити, кокон, пятна на полу.
	SilkSh = rgb(0x3a, 0x3a, 0x4c) // NEW #2 — тень шёлка
	SilkMd = rgb(0x8b, 0x8a, 0x9e) // NEW #3 — основной тон нитей
	SilkHi = rgb(0xe6, 0xe4, 0xf0) // NEW #4 — блик влажной нити

	// Глаза: кроваво-красные, светящиеся (НАСЛЕДИЕ из world_eater/palette.hex —
	// не новые оттенки). Много глаз кластером — источник взгляда-угрозы.
	EyeDk = rgb(0x5e, 0x12, 0x1c)
	EyeBt = rgb(0xe2, 0x2a, 0x33)
	EyeGl = rgb(0xff, 0x9a, 0x6a)

	// Глухая тень / AO / контактные тени и эллипс на полу (наследие).
	Shadow = rgb(0x0c, 0x0a, 0x14)
	DirtMd = rgb(0x5a, 0x40, 0x2b)
)

// Тёмная рампа хитина для лап и щетины (мрачнее тела — угрожающий силуэт).
var chitinDark = []color.RGBA{ChitinSh2, ChitinSh, ChitinDk, ChitinMd}

// Больная плоть вросших человеческих остатков (хитин-тон, трупная бледность).
var corpseFlesh = []color.RGBA{
	mixCol(FleshSh, ChitinDk, 0.45),
	mixCol(FleshMd, ChitinMd, 0.30),
	mixCol(FleshLt, ChitinLt, 0.15),
}

// Производные тона яда — смешением ядра с тьмой/светом (не считаются новыми:
// это рампа одного добавленного цвета).
var (
	VenomDk = mixCol(VenomCore, ChitinSh, 0.55) // тёмный яд
	VenomHi = mixCol(VenomCore, FleshLt, 0.55)   // светлый блик яда
	VenomGl = mixCol(VenomCore, rgb(0xff, 0xff, 0xd0), 0.6)
)

// Рампы материалов.
var (
	// 6-ступенчатая рампа хитина — для сферы брюшка и головогруди.
	chitin6 = []color.RGBA{ChitinSh2, ChitinSh, ChitinDk, ChitinMd, ChitinLt, ChitinHi}
	fleshR  = []color.RGBA{FleshSh, FleshMd, FleshLt}
	venomR  = []color.RGBA{VenomDk, VenomCore, VenomHi, VenomGl}
	silkR   = []color.RGBA{SilkSh, SilkMd, SilkHi}
)

var paletteAdditions = []struct {
	name string
	c    color.RGBA
}{
	{"venom_core", VenomCore},
	{"silk_shadow", SilkSh},
	{"silk_mid", SilkMd},
	{"silk_hi", SilkHi},
}

// lightDir — единый источник света в ЭКРАННЫХ координатах: сверху-слева и чуть
// «спереди» (небольшой уклон к камере). НЕ меняется между направлениями/кадрами.
var lightDir = [2]float64{-0.55, -0.78} // x влево, y вверх (экран: +y вниз)

// ─────────────────────────────────────────────────────────────────────────────
// canvas — нативный пиксельный холст (как в bossgen).
// ─────────────────────────────────────────────────────────────────────────────

type canvas struct {
	w, h int
	px   []color.RGBA
}

func newCanvas(w, h int) *canvas { return &canvas{w, h, make([]color.RGBA, w*h)} }

func (c *canvas) in(x, y int) bool { return x >= 0 && y >= 0 && x < c.w && y < c.h }

func (c *canvas) set(x, y int, col color.RGBA) {
	if c.in(x, y) && col.A > 0 {
		c.px[y*c.w+x] = col
	}
}

func (c *canvas) at(x, y int) color.RGBA {
	if !c.in(x, y) {
		return color.RGBA{}
	}
	return c.px[y*c.w+x]
}

// blend — альфа-наложение col поверх текущего пикселя.
func (c *canvas) blend(x, y int, col color.RGBA) {
	if !c.in(x, y) || col.A == 0 {
		return
	}
	if col.A == 255 {
		c.px[y*c.w+x] = col
		return
	}
	dst := c.px[y*c.w+x]
	a := float64(col.A) / 255
	mix := func(s, d uint8) uint8 { return uint8(float64(s)*a + float64(d)*(1-a)) }
	na := uint8(float64(col.A) + float64(dst.A)*(1-a))
	c.px[y*c.w+x] = color.RGBA{mix(col.R, dst.R), mix(col.G, dst.G), mix(col.B, dst.B), maxu8(na, dst.A)}
}

func maxu8(a, b uint8) uint8 {
	if a > b {
		return a
	}
	return b
}

func mixCol(a, b color.RGBA, t float64) color.RGBA {
	l := func(x, y uint8) uint8 { return uint8(float64(x)*(1-t) + float64(y)*t) }
	return color.RGBA{l(a.R, b.R), l(a.G, b.G), l(a.B, b.B), 255}
}

func rampAt(r []color.RGBA, t float64) color.RGBA {
	if math.IsNaN(t) || t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	i := int(t*float64(len(r)-1) + 0.5)
	return r[i]
}

func clamp01(t float64) float64 {
	if t < 0 {
		return 0
	}
	if t > 1 {
		return 1
	}
	return t
}

// ─────────────────────────────────────────────────────────────────────────────
// ТОП-ДАУН ПРИМИТИВЫ. Ключевое отличие от bossgen: свет считается в экранных
// координатах и одинаков всюду; формы объёмные (сфера/эллипсоид), с мягкой
// ступенчатой растушёвкой по рампе и с контактной тенью (AO) на стыках.
// ─────────────────────────────────────────────────────────────────────────────

// domeSphere — наклонный вид сверху на СФЕРУ (брюшко). Блик смещён к источнику
// света, глубокая тень — в противоположную сторону (снизу-справа). tilt<1
// сплющивает сферу по Y (перспектива вида сверху). Рампа ≥6 значений.
func (c *canvas) domeSphere(cx, cy, r, tilt float64, ramp []color.RGBA, litBoost float64) {
	ry := r * tilt
	x0, x1 := int(cx-r-1), int(cx+r+1)
	y0, y1 := int(cy-ry-1), int(cy+ry+1)
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			nx := (float64(x) - cx) / r
			ny := (float64(y) - cy) / ry
			d2 := nx*nx + ny*ny
			if d2 > 1 {
				continue
			}
			// нормаль полусферы (z вверх к камере)
			nz := math.Sqrt(1 - d2)
			// диффуз к источнику + добавка от близости к «верхушке» (nz) — объём.
			diff := nx*lightDir[0] + ny*lightDir[1]
			t := 0.30 + 0.55*(diff*0.5+0.5) + 0.15*nz
			t = clamp01(t*litBoost)
			c.set(x, y, rampAt(ramp, t))
		}
	}
}

// contactShadow — мягкая контактная тень (AO) кольцом под формой: затемняет уже
// нарисованные пиксели по эллипсу [r..r+width]. Главный источник объёма на стыках.
func (c *canvas) contactShadow(cx, cy, r, tilt, width, strength float64) {
	ry := r * tilt
	x0, x1 := int(cx-r-width-1), int(cx+r+width+1)
	y0, y1 := int(cy-ry-width-1), int(cy+ry+width+1)
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			nx := (float64(x) - cx) / (r + width)
			ny := (float64(y) - cy) / (ry + width)
			d := math.Hypot(nx*(r+width), ny*(ry+width))
			inner := math.Hypot(nx*(r+width)/r*r, ny) // приблизит.
			_ = inner
			// нормализованное расстояние во внешнем кольце
			dd := math.Hypot(nx, ny)
			if dd <= 1 && dd >= r/(r+width) {
				fall := (dd - r/(r+width)) / (1 - r/(r+width))
				a := strength * (1 - fall)
				if a > 0 {
					c.blend(x, y, color.RGBA{Shadow.R, Shadow.G, Shadow.B, uint8(clamp01(a) * 255)})
				}
			}
			_ = d
		}
	}
}

// softEllipse — сплошной эллипс с трубчатым затенением по нормали к свету
// (для головогруди, сегментов ног в проекции).
func (c *canvas) softEllipse(cx, cy, rx, ry, ang float64, ramp []color.RGBA) {
	ca, sa := math.Cos(ang), math.Sin(ang)
	x0, x1 := int(cx-rx-ry-1), int(cx+rx+ry+1)
	y0, y1 := int(cy-rx-ry-1), int(cy+rx+ry+1)
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			dx, dy := float64(x)-cx, float64(y)-cy
			lxx := (dx*ca + dy*sa) / rx
			lyy := (-dx*sa + dy*ca) / ry
			if lxx*lxx+lyy*lyy > 1 {
				continue
			}
			nz := math.Sqrt(clamp01(1 - (lxx*lxx + lyy*lyy)))
			diff := lxx*(lightDir[0]*ca-lightDir[1]*sa)/1 + lyy*0
			// используем экранный свет напрямую
			sdiff := (dx/(rx+0.001))*lightDir[0] + (dy/(ry+0.001))*lightDir[1]
			t := 0.32 + 0.5*(sdiff*0.5+0.5) + 0.18*nz
			_ = diff
			c.set(x, y, rampAt(ramp, clamp01(t)))
		}
	}
}

// taperedLimb — коническая цепочка сфер от основания к кончику (нога/жвало),
// объёмное затенение, каждый круг — top-down сфера. Возвращает контур для теней.
func (c *canvas) taperedLimb(bx, by, tx, ty, r0, r1 float64, ramp []color.RGBA) {
	steps := int(math.Hypot(tx-bx, ty-by)) + 1
	for i := 0; i <= steps; i++ {
		f := float64(i) / float64(steps)
		x := bx + (tx-bx)*f
		y := by + (ty-by)*f
		c.domeSphere(x, y, r0+(r1-r0)*f, 1.0, ramp, 1.0)
	}
}

// line — Брезенхэм одним цветом.
func (c *canvas) line(x0, y0, x1, y1 int, col color.RGBA) {
	dx := abs(x1 - x0)
	dy := -abs(y1 - y0)
	sx, sy := sign(x1-x0), sign(y1-y0)
	err := dx + dy
	for {
		c.set(x0, y0, col)
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

// softGlow — мягкое аддитивное свечение (яд, свет яиц изнутри брюшка).
func (c *canvas) softGlow(cx, cy, r float64, ramp []color.RGBA, intensity float64) {
	if r <= 0 {
		return
	}
	x0, x1 := int(cx-r-1), int(cx+r+1)
	y0, y1 := int(cy-r-1), int(cy+r+1)
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			d := math.Hypot(float64(x)-cx, float64(y)-cy)
			if d > r {
				continue
			}
			t := (1 - d/r) * intensity
			col := rampAt(ramp, clamp01(t))
			col.A = uint8(clamp01(t) * 255)
			c.blend(x, y, col)
		}
	}
}

func abs(a int) int {
	if a < 0 {
		return -a
	}
	return a
}
func sign(a int) int {
	switch {
	case a > 0:
		return 1
	case a < 0:
		return -1
	}
	return 0
}

// outlined — selective outline: 1px Outline вокруг силуэта, но НЕ со стороны
// света (ТЗ, правило 7: со стороны света контур светлее/отсутствует).
func outlined(src *canvas) *canvas {
	out := newCanvas(src.w, src.h)
	copy(out.px, src.px)
	for y := 0; y < src.h; y++ {
		for x := 0; x < src.w; x++ {
			if src.at(x, y).A != 0 {
				continue
			}
			// сосед непрозрачен?
			var nx, ny int
			has := false
			if src.at(x-1, y).A != 0 {
				nx, ny, has = 1, 0, true
			} else if src.at(x+1, y).A != 0 {
				nx, ny, has = -1, 0, true
			} else if src.at(x, y-1).A != 0 {
				nx, ny, has = 0, 1, true
			} else if src.at(x, y+1).A != 0 {
				nx, ny, has = 0, -1, true
			}
			if !has {
				continue
			}
			// направление «наружу» = (nx,ny). Если оно к свету — контур мягче.
			towardLight := float64(nx)*lightDir[0] + float64(ny)*lightDir[1]
			col := Outline
			if towardLight > 0.35 {
				continue // со стороны света контур отсутствует
			}
			out.px[y*out.w+x] = col
		}
	}
	return out
}

func (c *canvas) bbox() (minx, miny, maxx, maxy int) {
	minx, miny, maxx, maxy = c.w, c.h, -1, -1
	for y := 0; y < c.h; y++ {
		for x := 0; x < c.w; x++ {
			if c.at(x, y).A > 8 {
				if x < minx {
					minx = x
				}
				if y < miny {
					miny = y
				}
				if x > maxx {
					maxx = x
				}
				if y > maxy {
					maxy = y
				}
			}
		}
	}
	if maxx < 0 {
		return 0, 0, 0, 0
	}
	return
}

// blitTransform — перенос src в dst с подъёмом (height для прыжка), squash,
// alpha и тонировкой hurt (как в bossgen). ty здесь — вертикальный сдвиг вверх.
func blitTransform(dst, src *canvas, tx, ty, squash, alpha float64, tint color.RGBA, tintAmt float64) {
	gy := float64(src.h - 1)
	for y := 0; y < dst.h; y++ {
		for x := 0; x < dst.w; x++ {
			fx := float64(x) - tx
			fy := float64(y) - ty
			if squash != 0 && squash != 1 {
				fy = gy - (gy-fy)/squash
			}
			col := src.at(int(math.Round(fx)), int(math.Round(fy)))
			if col.A == 0 {
				continue
			}
			if tintAmt > 0 {
				col = mixColA(col, tint, tintAmt)
			}
			if alpha < 1 {
				col.A = uint8(float64(col.A) * alpha)
			}
			dst.set(x, y, col)
		}
	}
}

func mixColA(a, b color.RGBA, t float64) color.RGBA {
	l := func(x, y uint8) uint8 { return uint8(float64(x)*(1-t) + float64(y)*t) }
	return color.RGBA{l(a.R, b.R), l(a.G, b.G), l(a.B, b.B), a.A}
}

// ─────────────────────────────────────────────────────────────────────────────
// ТЕЛО ТКАЧИХИ. Нативно 64×64 (финал ×2 = 128×128). Вид сверху.
// Состав: раздутое БРЮШКО (сфера-эллипсоид, тыл), ГОЛОВОГРУДЬ (меньший купол,
// перёд), жвала, и на спине — вросшие человеческие остатки (кисть/лицо/ткань).
// Брюшко просвечивает: внутри силуэты яиц + свечение, растущее к призыву.
// Ноги здесь НЕ рисуются (отдельные спрайты, вариант 2).
//
// facing (радианы, 0 = «вниз к камере», по часовой) поворачивает лишь РАЗМЕЩЕНИЕ
// головогруди/жвал/остатков относительно брюшка. Свет остаётся экранным —
// поэтому направления честные, без отражения.
//
// НОГИ ЗАПЕКАЮТСЯ В КАДР. Хотя движок может ставить их IK-цепью (leg_segments +
// leg_attach_points в JSON), каждый кадр тела дополнительно рисует 8 ног
// процедурной позой (drawLegs), чтобы спрайт-лист был САМОДОСТАТОЧЕН и действие
// (шаг/выпад/присед/подгибание) читалось в самом PNG, а не только в рантайме.
// ─────────────────────────────────────────────────────────────────────────────

type bodyP struct {
	facing   float64 // направление морды
	eggGlow  float64 // 0..1 свечение выводка внутри брюшка
	legLift  float64 // приподнятость на передних ногах (bite windup) — визуальный сдвиг
	cracked  bool    // фаза 3: брюшко треснуло, слабая точка открыта
	cocoon   float64 // 0..1 степень обмотки шёлком (фаза 2)
	venomJaw float64 // 0..1 яд на жвалах (venom spray)
	hurt     float64
	dead     float64 // 0..1 оседание

	// Поза ног (все амплитуды — в нативных пикселях/долях).
	gait  float64 // фаза походки (0..2π)
	step  float64 // амплитуда переступа стоп
	rear  float64 // 0..1: встаёт на дыбы — передние ноги вскинуты к камере (телеграф)
	tuck  float64 // 0..1: ноги поджаты под тело (прыжок в воздухе)
	splay float64 // 0..1: ноги растопырены наружу (приземление)
}

const bodyN = 64
const deg = math.Pi / 180

// drawLegs — 8 длинных УГЛОВАТЫХ лап во весь кадр (доминирующий силуэт босса).
// Каждая нога — паучий зигзаг: бедро → высокое «колено» (вскинуто к камере) →
// «пятка» у пола → длинная лапка к острому КОГТЮ. Вдоль сегментов — ЩЕТИНА
// (короткие тёмные шипы, ломают силуэт → мохнатость и угроза). Разная длина:
// передняя пара самая длинная (ноги-щупы). Тетраподная походка в противофазе.
// Тёмная рампа (мрачнее тела). Своя тонкая тень на полу.
func drawLegs(c *canvas, cx, cy, facing float64, lp bodyP) {
	fwdAng := math.Pi/2 + facing
	baseRel := []float64{46, 82, 120, 156}
	lenMul := []float64{1.28, 1.06, 1.02, 0.9} // передняя пара — самые длинные
	const hipR, footR = 7.0, 29.0
	for i := 0; i < 8; i++ {
		left := i < 4
		idx := i
		if !left {
			idx = i - 4
		}
		rel := baseRel[idx] * deg
		if !left {
			rel = -rel
		}
		ang := fwdAng + rel
		grp := 0.0
		if (idx+boolToInt(left))%2 == 0 {
			grp = math.Pi
		}
		swing := lp.step * math.Sin(lp.gait+grp)
		lift := math.Max(0, math.Sin(lp.gait+grp))
		length := footR * lenMul[idx] * (1 - 0.42*lp.tuck + 0.18*lp.splay)
		hx := cx + math.Cos(ang)*hipR
		hy := cy + math.Sin(ang)*hipR*0.85
		tang := ang + math.Pi/2
		dx, dy := math.Cos(ang), math.Sin(ang)*0.72
		fxp := hx + dx*length + math.Cos(tang)*swing
		fyp := hy + dy*length + math.Sin(tang)*swing
		kneeRaise := 8 + 5*lift + 12*lp.rear
		if lp.rear > 0 && idx == 0 { // стойка на дыбах: перёд вскинут высоко к камере
			fxp = hx + dx*length*0.5
			fyp = hy + dy*length*0.28 - 20*lp.rear
			kneeRaise = 16
		}
		if lp.dead > 0 { // подгибание внутрь
			fxp = cx + (fxp-cx)*(1-0.72*lp.dead)
			fyp = cy + (fyp-cy)*(1-0.72*lp.dead) + 6*lp.dead
		}
		// Узлы зигзага: колено (высоко) и пятка (у пола).
		kx := hx + (fxp-hx)*0.42
		ky := hy + (fyp-hy)*0.42 - kneeRaise
		hx2 := hx + (fxp-hx)*0.74
		hy2 := hy + (fyp-hy)*0.74 - kneeRaise*0.25
		// Тень: ломаная по проекции ноги (слабее, если стопа поднята).
		sa := uint8(clamp01(0.5*(1-0.85*lift)) * 150)
		shCol := color.RGBA{Shadow.R, Shadow.G, Shadow.B, sa}
		drawSoftLine(c, hx, hy+3, hx2, hy2+4, shCol)
		drawSoftLine(c, hx2, hy2+4, fxp, fyp+3, shCol)
		// Сегменты (тёмный хитин): бедро → голень → лапка к острию.
		c.taperedLimb(hx, hy, kx, ky, 2.3, 1.7, chitinDark)
		c.taperedLimb(kx, ky, hx2, hy2, 1.7, 1.3, chitinDark)
		c.taperedLimb(hx2, hy2, fxp, fyp, 1.3, 0.4, chitinDark)
		// Шип на колене (угловатый сустав).
		c.set(int(kx), int(ky-1), ChitinSh2)
		c.set(int(kx), int(ky), ChitinHi) // блик на суставе
		// Щетина вдоль голени и лапки.
		legBristles(c, kx, ky, hx2, hy2)
		legBristles(c, hx2, hy2, fxp, fyp)
		// Коготь на конце — раздвоенное остриё.
		clx, cly := fxp-hx2, fyp-hy2
		cl := math.Hypot(clx, cly) + 1e-6
		clx, cly = clx/cl, cly/cl
		px, py := -cly, clx
		c.set(int(fxp+clx*1.5+px), int(fyp+cly*1.5+py), ChitinSh2)
		c.set(int(fxp+clx*1.5-px), int(fyp+cly*1.5-py), ChitinSh2)
		c.set(int(fxp), int(fyp), ChitinSh2)
	}
}

// legBristles — короткие тёмные щетинки перпендикулярно сегменту, поочерёдно
// по сторонам. Ломают гладкий силуэт → мохнатая, неприятная фактура.
func legBristles(c *canvas, x0, y0, x1, y1 float64) {
	steps := int(math.Hypot(x1-x0, y1-y0))
	px, py := -(y1 - y0), (x1 - x0)
	l := math.Hypot(px, py) + 1e-6
	px, py = px/l, py/l
	for i := 2; i < steps; i += 2 {
		f := float64(i) / float64(steps)
		bx := x0 + (x1-x0)*f
		by := y0 + (y1-y0)*f
		side := 1.0
		if (i/2)%2 == 0 {
			side = -1
		}
		c.set(int(bx+px*side*1.6), int(by+py*side*1.6), ChitinSh2)
		c.set(int(bx+px*side*2.6), int(by+py*side*2.6), Outline)
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func renderBody(p bodyP) *canvas {
	c := newCanvas(bodyN, bodyN)
	const cx = 32.0
	// центр брюшка смещён «назад» (противоположно facing), головогрудь — вперёд.
	fx, fy := math.Sin(p.facing), math.Cos(p.facing) // 0→(0,1) вниз
	// «вперёд» на экране = (fx, fy); «назад» = (-fx,-fy).
	abCx := cx - fx*8
	abCy := 34.0 - fy*8
	ctCx := cx + fx*11
	ctCy := 34.0 + fy*11
	// Смерть: тело оседает вниз и брюшко приплющивается (tilt→плоский).
	sink := 5 * p.dead
	abCy += sink
	ctCy += sink

	// −1) НОГИ — запечены под тело; тело перекроет бёдра. Крепятся к головогруди.
	if p.cocoon < 0.4 { // в коконе ноги скрыты шёлком
		drawLegs(c, ctCx, (abCy+ctCy)/2, p.facing, p)
	}

	// 0) Контактная тень брюшка на «полу тела» (AO вокруг основания).
	c.contactShadow(abCx, abCy+2, 20, 0.72, 4, 0.5)

	// 1) БРЮШКО — сфера-эллипсоид, наклон сверху (tilt 0.82). Рампа 6 значений.
	abR := 20.0
	abTilt := 0.82
	if p.dead > 0 {
		abR -= 3 * p.dead
		abTilt -= 0.32 * p.dead // приплющивается
	}
	c.domeSphere(abCx, abCy, abR, abTilt, chitin6, 1.0)

	// 1b) Просвечивание: силуэты яиц + внутреннее свечение (усиливается eggGlow).
	if p.cocoon < 0.5 {
		eggs := [][2]float64{{-6, 2}, {4, -3}, {-1, 6}, {8, 4}, {-8, -4}, {2, 9}}
		for i, e := range eggs {
			ex, ey := abCx+e[0], abCy+e[1]*0.82
			// тёмный силуэт яйца
			c.domeSphere(ex, ey, 3.2, 0.9, []color.RGBA{ChitinSh2, ChitinSh, ChitinDk}, 1.0)
			if p.eggGlow > 0.02 {
				gl := 0.35 + 0.65*p.eggGlow
				pulse := 0.8 + 0.2*math.Sin(float64(i)*1.7)
				c.softGlow(ex, ey, 4.2*gl, venomR, 0.5*gl*pulse)
			}
		}
		if p.eggGlow > 0.3 {
			c.softGlow(abCx, abCy, abR*0.7, venomR, 0.25*p.eggGlow)
		}
	}

	// 1b2) ВЕНЫ и ВЗДУТИЯ: раздутые, пульсирующие ядом сосуды под хитином +
	//      бугры-волдыри → больная, органическая фактура (не гладкий шар).
	abdomenVeins(c, abCx, abCy, abR, abTilt, p.eggGlow)

	// 1c) Блик брюшка (смещён к свету) + узкий РИМ-СВЕТ по краю со стороны света.
	hlx := abCx + lightDir[0]*abR*0.45
	hly := abCy + lightDir[1]*abR*0.45*0.82
	c.softGlow(hlx, hly, abR*0.30, []color.RGBA{ChitinHi, ChitinHi}, 0.55)
	rimLight(c, abCx, abCy, abR, abTilt)

	// 1d) Фаза 3 — трещины на брюшке, слабая точка открыта, изнутри яд.
	if p.cracked {
		crackVenom(c, abCx, abCy, abR)
	}

	// 1e) ВРОСШИЕ ЧЕЛОВЕЧЕСКИЕ ОСТАТКИ — кричащее лицо, вплавленное в хитин спины,
	//     и тянущаяся из брони рука. Главный элемент ужаса: читается со второго
	//     взгляда, трупно-бледное на фоне хитина.
	fusedFace(c, abCx-fx*6, abCy-fy*6, p.facing)
	graspingHand(c, abCx-fx*2+abR*0.55, abCy-fy*2+abR*0.28*abTilt, p.facing)

	// 1f) Бахрома щетины по краю брюшка (жёсткие волоски).
	bristleFringe(c, abCx, abCy, abR, abTilt)

	// 1g) ПАУТИННЫЕ БОРОДАВКИ (спиннереты) на тыльном острие брюшка + нити.
	spinnerets(c, abCx-fx*(abR*0.85), abCy-fy*(abR*0.85)*abTilt, p.facing)

	// 2) Контактная тень между брюшком и головогрудью (стык форм).
	c.contactShadow((abCx+ctCx)/2, (abCy+ctCy)/2, 9, 0.8, 3, 0.55)

	// 3) ГОЛОВОГРУДЬ — приплюснутый купол ближе к камере, перекрывает брюшко и на
	//    шаг светлее. Тёмный хитиновый гребень по центру.
	ctR := 12.0
	c.domeSphere(ctCx, ctCy, ctR, 0.8, chitin6, 1.1)
	c.contactShadow(ctCx, ctCy, ctR-1, 0.8, 2, 0.4) // самозатенение по краю
	// продольный гребень
	c.taperedLimb(ctCx-fx*ctR*0.6, ctCy-fy*ctR*0.6, ctCx+fx*ctR*0.5, ctCy+fy*ctR*0.5, 1.4, 0.6, chitinDark)

	// 3b) КЛАСТЕР ГЛАЗ — 8 глаз, 2 крупных главных + 6 малых, светятся кроваво.
	//     Взгляд-угроза; тёмные глазницы вокруг.
	eyeCluster(c, ctCx, ctCy, p.facing, 0.6+0.4*p.eggGlow)

	// 4) ХЕЛИЦЕРЫ с БОЛЬШИМИ ИЗОГНУТЫМИ КЛЫКАМИ спереди, с капающим ядом.
	chelicerae(c, ctCx+fx*ctR*0.7, ctCy+fy*ctR*0.7, p.facing, p.venomJaw)

	// 5) Кокон (фаза 2): заматывает тело в шёлк, скрывает детали, неуязвимость.
	if p.cocoon > 0.02 {
		wrapSilk(c, abCx, abCy, abR+2, p.cocoon)
	}

	// 6) hurt-вспышка.
	if p.hurt > 0 {
		for i := range c.px {
			if c.px[i].A > 0 {
				c.px[i] = mixColA(c.px[i], color.RGBA{255, 120, 110, 255}, 0.5*p.hurt)
			}
		}
	}
	return c
}

// EyeGlowDim — тусклый красный глазок (наследие палитры глаз червя, приглушён).
func EyeGlowDim() color.RGBA { return rgb(0xc2, 0x3a, 0x40) }

// crackVenom — трещины на брюшке фазы 3 с ядовитым свечением изнутри.
func crackVenom(c *canvas, cx, cy, r float64) {
	segs := [][2]float64{{-0.5, -0.6}, {0.1, -0.2}, {-0.2, 0.3}, {0.4, 0.5}}
	prev := [2]float64{cx, cy - r*0.4}
	for _, s := range segs {
		nx := cx + s[0]*r
		ny := cy + s[1]*r*0.82
		c.line(int(prev[0]), int(prev[1]), int(nx), int(ny), VenomDk)
		c.line(int(prev[0]), int(prev[1]), int(nx), int(ny), VenomCore)
		c.softGlow(nx, ny, 3, venomR, 0.6)
		prev = [2]float64{nx, ny}
	}
}

// fusedFace — кричащее человеческое лицо, вплавленное в хитин спины: впалые
// глазницы, нос-гребень, распахнутый в вопле рот с зубами. Трупно-бледное.
func fusedFace(c *canvas, cx, cy, facing float64) {
	fx, fy := math.Sin(facing), math.Cos(facing) // «низ лица» = к морде паучихи
	px, py := fy, -fx                             // поперечная ось лица
	// маска лица (бледная плоть, чуть утоплена в хитин)
	c.softEllipse(cx, cy, 5.5, 4.2, facing, corpseFlesh)
	c.contactShadow(cx, cy, 5, 0.8, 2, 0.5) // вросла — тень по краю
	// впалые глазницы (тёмные ямы + слабый мёртвый блик)
	for _, s := range []float64{-1, 1} {
		ex := cx - fx*1.6 + px*s*2.1
		ey := cy - fy*1.6 + py*s*2.1
		c.domeSphere(ex, ey, 1.7, 0.9, []color.RGBA{Shadow, ChitinSh2, ChitinSh}, 1.0)
		c.set(int(ex), int(ey), Shadow)
		c.set(int(ex-1), int(ey-1), mixCol(FleshSh, Shadow, 0.3)) // холодный блик
	}
	// нос-гребень
	c.set(int(cx), int(cy), rampAt(corpseFlesh, 0.4))
	// распахнутый рот-вопль (тёмный овал) + зубы-засечки
	mx, my := cx+fx*2.4, cy+fy*2.4
	c.softEllipse(mx, my, 2.6, 1.8, facing, []color.RGBA{Shadow, ChitinSh2, ChitinSh})
	c.set(int(mx), int(my), Shadow)
	for t := -2.0; t <= 2; t++ { // верхние зубы
		c.set(int(mx+px*t), int(my-fy*1.2), rampAt(corpseFlesh, 0.8))
	}
}

// graspingHand — тянущаяся из брони человеческая рука: предплечье + ладонь с
// растопыренными скрюченными пальцами. Полувросшая, трупно-бледная.
func graspingHand(c *canvas, cx, cy, facing float64) {
	fx, fy := math.Sin(facing), math.Cos(facing)
	// предплечье, уходящее в хитин
	c.taperedLimb(cx-fx*4, cy-fy*4, cx, cy, 1.4, 2.0, corpseFlesh)
	c.contactShadow(cx-fx*3, cy-fy*3, 3, 0.9, 2, 0.4)
	// ладонь
	c.domeSphere(cx, cy, 2.3, 0.9, corpseFlesh, 1.0)
	// пять скрюченных пальцев, тянущихся вверх-наружу
	for _, a := range []float64{-58, -28, 0, 28, 58} {
		rad := math.Atan2(fy, fx) + a*deg
		// два фаланговых сегмента с изломом (скрюченность)
		kx := cx + math.Cos(rad)*2.6
		ky := cy + math.Sin(rad)*2.6 - 1.2
		tx := kx + math.Cos(rad)*2.2
		ty := ky + math.Sin(rad)*2.2 - 0.6
		c.taperedLimb(cx, cy, kx, ky, 1.0, 0.7, corpseFlesh)
		c.taperedLimb(kx, ky, tx, ty, 0.7, 0.3, corpseFlesh)
		c.set(int(tx), int(ty), rampAt(corpseFlesh, 0)) // тёмный ноготь
	}
}

// abdomenVeins — вздутые вены с ядом под хитином + бугры-волдыри. Даёт больную
// органическую фактуру; вены слегка светятся, если выводок пробуждается.
func abdomenVeins(c *canvas, cx, cy, r, tilt, glow float64) {
	veins := [][3]float64{{-0.6, -0.5, 0.7}, {0.5, -0.55, 0.65}, {-0.2, 0.4, 0.6}, {0.55, 0.35, 0.55}, {0.05, -0.7, 0.5}}
	for _, v := range veins {
		bx, by := cx, cy
		ex := cx + v[0]*r
		ey := cy + v[1]*r*tilt
		// ветвящаяся тёмная жила
		mx := (bx + ex) / 2
		my := (by+ey)/2 + (v[2]-0.6)*4
		drawSoftLine(c, bx, by, mx, my, mixCol(ChitinSh2, VenomDk, 0.4*glow+0.1))
		drawSoftLine(c, mx, my, ex, ey, mixCol(ChitinSh2, VenomDk, 0.4*glow+0.1))
		if glow > 0.25 {
			c.set(int(ex), int(ey), mixCol(VenomCore, ChitinSh2, 0.4))
		}
	}
	// волдыри-бугры (тёмное основание + маленький блик — выпуклость)
	welts := [][2]float64{{-0.35, -0.15}, {0.3, 0.1}, {-0.1, 0.55}, {0.15, -0.45}, {0.5, -0.2}}
	for _, w := range welts {
		wx := cx + w[0]*r
		wy := cy + w[1]*r*tilt
		c.domeSphere(wx, wy, 2.2, 0.9, chitin6, 0.9)
		c.set(int(wx-1), int(wy-1), ChitinHi)
	}
}

// rimLight — тонкий яркий кант по краю формы со стороны источника света.
func rimLight(c *canvas, cx, cy, r, tilt float64) {
	for a := 0.0; a < 2*math.Pi; a += 0.06 {
		nx, ny := math.Cos(a), math.Sin(a)
		if nx*lightDir[0]+ny*lightDir[1] < 0.45 { // только сторона света
			continue
		}
		x := cx + nx*(r-0.5)
		y := cy + ny*(r-0.5)*tilt
		c.blend(int(x), int(y), color.RGBA{ChitinHi.R, ChitinHi.G, ChitinHi.B, 150})
	}
}

// bristleFringe — жёсткие волоски по краю брюшка (силуэт «мохнатый», неприятный).
func bristleFringe(c *canvas, cx, cy, r, tilt float64) {
	for a := 0.0; a < 2*math.Pi; a += 0.5 {
		nx, ny := math.Cos(a), math.Sin(a)
		x0 := cx + nx*r
		y0 := cy + ny*r*tilt
		c.set(int(x0+nx*1.5), int(y0+ny*1.5*tilt), ChitinSh2)
		c.set(int(x0+nx*2.5), int(y0+ny*2.5*tilt), Outline)
	}
}

// spinnerets — паутинные бородавки на тыльном острие брюшка + свисающие нити.
func spinnerets(c *canvas, cx, cy, facing float64) {
	fx, fy := math.Sin(facing), math.Cos(facing)
	px, py := fy, -fx
	for _, s := range []float64{-1.4, 0, 1.4} {
		bx := cx + px*s
		by := cy + py*s
		c.domeSphere(bx-fx*1, by-fy*1, 1.6, 0.9, chitinDark, 0.9)
		// нить шёлка, тянущаяся назад
		c.blend(int(bx-fx*3), int(by-fy*3), color.RGBA{SilkMd.R, SilkMd.G, SilkMd.B, 150})
		c.blend(int(bx-fx*5), int(by-fy*5), color.RGBA{SilkSh.R, SilkSh.G, SilkSh.B, 110})
	}
}

// eyeCluster — 8 глаз пауком: 2 крупных главных по центру + 6 малых вокруг.
// Тёмные глазницы, кроваво-красное свечение, слабый ореол. glow усиливает накал.
func eyeCluster(c *canvas, ctCx, ctCy, facing, glow float64) {
	fx, fy := math.Sin(facing), math.Cos(facing) // «вперёд» — к морде
	px, py := fy, -fx
	// (вперёд, вбок, радиус, главный?)
	layout := [][4]float64{
		{3.5, -1.4, 1.9, 1}, {3.5, 1.4, 1.9, 1}, // главные (AME) — крупные
		{2.0, -3.2, 1.1, 0}, {2.0, 3.2, 1.1, 0},
		{5.0, -2.8, 1.0, 0}, {5.0, 2.8, 1.0, 0},
		{5.4, -0.6, 0.9, 0}, {5.4, 0.6, 0.9, 0},
	}
	for _, e := range layout {
		ex := ctCx + fx*e[0] + px*e[1]
		ey := ctCy + fy*e[0] + py*e[1]
		r := e[2]
		// тёмная глазница-кольцо
		c.domeSphere(ex, ey, r+0.8, 0.9, []color.RGBA{Shadow, ChitinSh2, ChitinSh}, 1.0)
		// стекловидное тело — красное
		c.domeSphere(ex, ey, r, 0.95, []color.RGBA{EyeDk, EyeBt, EyeBt}, 1.0)
		// накал + блик
		c.softGlow(ex, ey, r+1.4, []color.RGBA{EyeBt, EyeGl}, 0.5+0.5*glow)
		c.set(int(ex), int(ey), EyeGl)
		if e[3] > 0 { // у главных — вертикальный «зрачок»-щель
			c.set(int(ex), int(ey-1), EyeDk)
			c.set(int(ex), int(ey+1), EyeDk)
		}
	}
}

// chelicerae — две массивные хелицеры с изогнутыми внутрь клыками, каплей яда.
func chelicerae(c *canvas, cx, cy, facing, venom float64) {
	fx, fy := math.Sin(facing), math.Cos(facing)
	px, py := fy, -fx
	c.contactShadow(cx, cy, 4, 0.8, 2, 0.5)
	for _, s := range []float64{-1, 1} {
		// основание хелицеры (толстый бугор)
		bx := cx + px*s*2.6
		by := cy + py*s*2.6
		c.domeSphere(bx, by, 3.0, 0.85, chitin6, 1.0)
		// клык: изгиб наружу→внутрь, к острию (3 узла), тёмный блестящий хитин
		t1x := bx + fx*3 + px*s*1.5
		t1y := by + fy*3 + py*s*1.5
		t2x := bx + fx*6 - px*s*0.5 // сходятся внутрь
		t2y := by + fy*6 - py*s*0.5
		c.taperedLimb(bx, by, t1x, t1y, 2.2, 1.3, chitinDark)
		c.taperedLimb(t1x, t1y, t2x, t2y, 1.3, 0.3, chitinDark)
		c.set(int(bx-1), int(by-1), ChitinHi) // блик на основании
		// капля яда на кончике
		venomBead := 0.4 + venom
		c.softGlow(t2x, t2y, 1.6*venomBead, venomR, 0.6+0.4*venom)
		c.set(int(t2x), int(t2y), VenomGl)
		if venom > 0.4 { // стекает капля
			c.set(int(t2x+fx), int(t2y+fy+1), VenomCore)
			c.set(int(t2x+fx*1.5), int(t2y+fy+2), VenomDk)
		}
	}
}

// wrapSilk — обмотка тела шёлком (кокон): пряди поверх силуэта, к amt=1 почти
// полностью укрывают, тон холодный серо-фиолетовый.
func wrapSilk(c *canvas, cx, cy, r, amt float64) {
	bands := int(6 * amt)
	for b := 0; b < bands; b++ {
		ang := float64(b)/6*math.Pi + amt*0.5
		for t := -r; t <= r; t += 0.7 {
			x := cx + math.Cos(ang)*t
			y := cy + math.Sin(ang)*t*0.82
			off := math.Sin(t*0.8+float64(b)) * 1.5
			px := x + math.Cos(ang+math.Pi/2)*off
			py := y + math.Sin(ang+math.Pi/2)*off
			shade := 0.4 + 0.5*((math.Cos(ang)*lightDir[0]+math.Sin(ang)*lightDir[1])*0.5+0.5)
			col := rampAt(silkR, shade)
			col.A = uint8(200 * amt)
			c.blend(int(px), int(py), col)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// НОГИ (вариант 2). Каждая нога — 3 сегмента: coxa (короткий, у тела), femur
// (средний), tibia (длинный, к острию). Рисуются вдоль +X, пивот сегмента —
// в его основании (левый край), кончик — правый край. Игра ставит их IK-цепью
// по leg_attach_points. Отдельные single-frame спрайты.
// ─────────────────────────────────────────────────────────────────────────────

type legSeg struct {
	name          string
	nw, nh        int
	length, r0, r1 float64
}

var legSegs = []legSeg{
	{"leg_coxa", 12, 8, 9, 3.0, 2.4},   // короткий толстый у тела
	{"leg_femur", 22, 8, 19, 2.4, 1.7}, // средний
	{"leg_tibia", 28, 8, 25, 1.7, 0.5}, // длинный к острию
}

func renderLegSeg(s legSeg) *canvas {
	c := newCanvas(s.nw, s.nh)
	by := float64(s.nh) / 2
	// собственная тонкая тень сегмента на полу (ТЗ: каждая нога отбрасывает тень)
	c.taperedLimb(1, by+2, s.length, by+2, s.r0*0.7, s.r1*0.7,
		[]color.RGBA{Shadow, Shadow})
	// сам сегмент — хитин, объём top-down
	c.taperedLimb(1, by, s.length, by, s.r0, s.r1, chitin6)
	// сустав на дальнем конце — тёмное «кольцо» (AO шарнира)
	c.set(int(s.length), int(by), ChitinSh2)
	return c
}

// ─────────────────────────────────────────────────────────────────────────────
// ПАУЧОК (spiderling). Нативно 20×20 (финал 40×40). Мини-версия: брюшко + 8
// коротких ножек, запечённых прямо в спрайт (он мелкий, IK не нужен).
// ─────────────────────────────────────────────────────────────────────────────

type spiderP struct {
	phase float64 // фаза шага
	jump  float64
	dead  float64
	att   float64 // рывок-укус
}

func renderSpiderling(p spiderP) *canvas {
	c := newCanvas(20, 20)
	const cx, cy = 10.0, 12.0
	// тень
	c.contactShadow(cx, cy+3-p.jump*4, 6, 0.6, 2, 0.5*(1-p.jump))
	// 8 ножек
	for i := 0; i < 8; i++ {
		side := 1.0
		idx := i
		if i >= 4 {
			side = -1.0
			idx = i - 4
		}
		baseA := (float64(idx)-1.5)*0.5 + math.Pi/2
		swing := 0.35 * math.Sin(p.phase+float64(i)*0.8)
		a := baseA*side + swing
		lx := cx + math.Cos(a+math.Pi/2)*side
		ly := cy
		// угловатая ножка с изломом → колючий силуэт
		kx := lx + math.Cos(a)*4*side
		ky := ly + math.Sin(a)*3 - 1.5
		tx := lx + math.Cos(a)*8*side
		ty := ly + math.Sin(a)*7
		c.taperedLimb(lx, ly, kx, ky, 1.2, 0.8, chitinDark)
		c.taperedLimb(kx, ky, tx, ty, 0.8, 0.2, chitinDark)
	}
	lift := p.jump * 4
	// брюшко (с бликом-выпуклостью)
	c.domeSphere(cx, cy-2-lift, 5.5, 0.85, chitin6, 1.0)
	c.set(int(cx-2), int(cy-4-lift), ChitinHi)
	// головогрудь
	c.domeSphere(cx, cy+2-lift, 3.2, 0.85, chitin6, 1.08)
	// кластер из 4 горящих красных глазок + жвала-жало
	for _, e := range [][2]float64{{-1.4, 3}, {1.4, 3}, {-0.6, 4.2}, {0.6, 4.2}} {
		ex, ey := cx+e[0], cy+e[1]-lift
		c.set(int(ex), int(ey), EyeBt)
		c.softGlow(ex, ey, 1.6, []color.RGBA{EyeBt, EyeGl}, 0.7)
	}
	c.set(int(cx), int(cy+5-lift), VenomGl) // ядовитое жало-капля
	if p.att > 0.3 { // капля яда при атаке
		c.softGlow(cx, cy+5.5-lift, 2.5*p.att, venomR, p.att)
	}
	if p.dead > 0 { // подгибает ножки, оседает
		for i := range c.px {
			if c.px[i].A > 0 {
				c.px[i].A = uint8(float64(c.px[i].A) * (1 - 0.7*p.dead))
			}
		}
	}
	return c
}

// ─────────────────────────────────────────────────────────────────────────────
// КОКОН выводка (eggsac). Нативно 28×28 (финал 56×56). Шёлковый мешок, внутри
// пульсирует ядовитое свечение; при hatch трескается и раскрывается.
// ─────────────────────────────────────────────────────────────────────────────

func renderEggsac(stage string, k, n int) *canvas {
	c := newCanvas(28, 28)
	const cx, cy = 14.0, 15.0
	t := prog(k, n)
	ph := loopPh(k, n)
	c.contactShadow(cx, cy+4, 11, 0.5, 3, 0.5)
	// мешок из шёлка (вытянут по вертикали)
	c.softEllipse(cx, cy, 9, 11, 0, silkR)
	// пряди шёлка
	for b := 0; b < 5; b++ {
		ang := float64(b)/5*math.Pi + 0.3
		for tt := -11.0; tt <= 11; tt += 0.8 {
			x := cx + math.Cos(ang)*tt*0.8
			y := cy + math.Sin(ang)*tt
			c.blend(int(x), int(y), color.RGBA{SilkSh.R, SilkSh.G, SilkSh.B, 120})
		}
	}
	// внутреннее свечение выводка
	glow := 0.4
	switch stage {
	case "idle":
		glow = 0.30 + 0.05*math.Sin(ph)
	case "pulse":
		glow = 0.35 + 0.4*math.Abs(math.Sin(ph*1.5))
	case "hatch":
		glow = 0.4 + 0.6*t
	case "destroyed":
		glow = 0.4 * (1 - t)
	}
	c.softGlow(cx, cy, 7, venomR, glow)
	// силуэты паучков внутри
	for _, e := range [][2]float64{{-3, -2}, {3, 1}, {0, 4}} {
		c.domeSphere(cx+e[0], cy+e[1], 2, 0.9, []color.RGBA{ChitinSh2, ChitinDk}, 1.0)
	}
	if stage == "hatch" && t > 0.5 { // разрыв
		c.line(int(cx), int(cy-9), int(cx+3*int(4*t)), int(cy+9), SilkHi)
		c.softGlow(cx, cy, 10*t, venomR, 0.8)
	}
	if stage == "destroyed" { // лопается, шёлк опадает
		for i := range c.px {
			if c.px[i].A > 0 {
				c.px[i].A = uint8(float64(c.px[i].A) * (1 - 0.8*t))
			}
		}
	}
	return c
}

// ─────────────────────────────────────────────────────────────────────────────
// FX. Снаряд-паутина, пятно паутины на полу (тайлится), лужа яда, тень-телеграф
// прыжка, всплеск шёлка, тень прыжка (синхронно с leap_air).
// ─────────────────────────────────────────────────────────────────────────────

func fxWebProjectile(k, n int) *canvas { // 10×10 loop — комок паутины, вращается
	c := newCanvas(10, 10)
	ph := loopPh(k, n)
	c.softGlow(5, 5, 4, silkR, 0.4)
	// клубок нитей
	for i := 0; i < 6; i++ {
		a := ph + float64(i)*math.Pi/3
		x0 := 5 + math.Cos(a)*3.5
		y0 := 5 + math.Sin(a)*3.5
		c.line(5, 5, int(x0), int(y0), rampAt(silkR, 0.6))
	}
	c.domeSphere(5, 5, 2.4, 1, silkR, 1.0)
	c.set(4, 4, SilkHi)
	return c
}

func fxWebPatch(stage string, k, n int) *canvas { // 32×32 tileable — пятно паутины
	c := newCanvas(32, 32)
	t := prog(k, n)
	ph := loopPh(k, n)
	var alpha float64
	switch stage {
	case "spawn":
		alpha = t
	case "loop":
		alpha = 0.85 + 0.1*math.Sin(ph)
	case "break":
		alpha = 1 - t
	}
	// радиальная сеть из центра + концентрические нити (тайлится по краям слабо)
	cx, cy := 16.0, 16.0
	for i := 0; i < 12; i++ {
		a := float64(i) / 12 * 2 * math.Pi
		ex := cx + math.Cos(a)*16
		ey := cy + math.Sin(a)*16
		col := rampAt(silkR, 0.5)
		col.A = uint8(alpha * 160)
		drawSoftLine(c, cx, cy, ex, ey, col)
	}
	for ring := 4.0; ring < 16; ring += 4 {
		for a := 0.0; a < 2*math.Pi; a += 0.15 {
			x := cx + math.Cos(a)*ring
			y := cy + math.Sin(a)*ring
			col := rampAt(silkR, 0.55)
			col.A = uint8(alpha * 120)
			c.blend(int(x), int(y), col)
		}
	}
	if stage == "break" { // рвётся клочьями
		for i := 0; i < 20; i++ {
			a := float64(i) * 2.3
			d := 16 * t
			c.set(int(cx+math.Cos(a)*d), int(cy+math.Sin(a)*d), SilkSh)
		}
	}
	return c
}

func drawSoftLine(c *canvas, x0, y0, x1, y1 float64, col color.RGBA) {
	steps := int(math.Hypot(x1-x0, y1-y0)) + 1
	for i := 0; i <= steps; i++ {
		f := float64(i) / float64(steps)
		x := x0 + (x1-x0)*f
		y := y0 + (y1-y0)*f
		c.blend(int(x), int(y), col)
	}
}

func fxVenomPool(k, n int) *canvas { // 32×16 loop — лужа яда (конус venom spray)
	c := newCanvas(32, 16)
	ph := loopPh(k, n)
	cx, cy := 16.0, 9.0
	rx, ry := 14+0.5*math.Sin(ph), 6.0
	for y := 0; y < c.h; y++ {
		for x := 0; x < c.w; x++ {
			dx, dy := (float64(x)-cx)/rx, (float64(y)-cy)/ry
			e := dx*dx + dy*dy
			if e <= 1 {
				col := rampAt(venomR, 0.3+0.5*e)
				col.A = 210
				c.blend(x, y, col)
			}
		}
	}
	for i := 0; i < 4; i++ { // пузыри
		c.set(6+i*6, 9-(k+i)%4, VenomGl)
	}
	return c
}

func fxLeapWarning(k, n int) *canvas { // 32×32 — тень-телеграф места приземления
	c := newCanvas(32, 32)
	t := prog(k, n)
	pulse := 0.4 + 0.6*math.Abs(math.Sin(math.Pi*t))
	// кольцо-мишень
	for a := 0.0; a < 2*math.Pi; a += 0.1 {
		r := 13.0
		x := 16 + math.Cos(a)*r
		y := 16 + math.Sin(a)*r*0.6 // сплюснуто (пол)
		col := VenomCore
		col.A = uint8(pulse * 200)
		c.blend(int(x), int(y), col)
	}
	c.softGlow(16, 16, 12*t, []color.RGBA{Shadow, Shadow}, 0.4*pulse)
	return c
}

func fxShadowLeap(k, n int) *canvas { // 112×48-ниже: тень тела в прыжке, сжимается
	c := newCanvas(56, 24)
	// t=0 на земле (крупная), в середине прыжка — маленькая и светлее
	t := prog(k, n)
	rise := math.Sin(math.Pi * t) // 0..1..0
	rx := 26 * (1 - 0.55*rise)
	ry := 11 * (1 - 0.55*rise)
	alpha := 0.55 * (1 - 0.5*rise)
	drawGroundShadow(c, 28, 12, rx, ry, alpha)
	return c
}

func fxSilkBurst(k, n int) *canvas { // 32×32 — всплеск шёлка (dash/cocoon break)
	c := newCanvas(32, 32)
	t := prog(k, n)
	for i := 0; i < 16; i++ {
		a := float64(i) / 16 * 2 * math.Pi
		d := 15 * t
		x0 := 16 + math.Cos(a)*d*0.4
		y0 := 16 + math.Sin(a)*d*0.4
		x1 := 16 + math.Cos(a)*d
		y1 := 16 + math.Sin(a)*d
		col := rampAt(silkR, 0.7)
		col.A = uint8((1 - t) * 220)
		drawSoftLine(c, x0, y0, x1, y1, col)
	}
	c.softGlow(16, 16, 8*(1-t), silkR, 0.6*(1-t))
	return c
}

// ─────────────────────────────────────────────────────────────────────────────
// ТЕНИ на полу — отдельные спрайты (правило 6). Мягкий край, полупрозрачные.
// ─────────────────────────────────────────────────────────────────────────────

func drawGroundShadow(c *canvas, cx, cy, rx, ry, alpha float64) {
	for y := 0; y < c.h; y++ {
		for x := 0; x < c.w; x++ {
			dx, dy := (float64(x)-cx)/rx, (float64(y)-cy)/ry
			d := dx*dx + dy*dy
			if d <= 1 {
				a := alpha * (1 - d*d) // мягкий спад к краю
				c.blend(x, y, color.RGBA{Shadow.R, Shadow.G, Shadow.B, uint8(clamp01(a) * 255)})
			}
		}
	}
}

func fxBodyShadow(k, n int) *canvas { // 56×24 (финал 112×48) — тень тела на земле
	c := newCanvas(56, 24)
	drawGroundShadow(c, 28, 12, 26, 11, 0.5)
	return c
}

func fxSpiderShadow(k, n int) *canvas { // 20×10 — тень паучка
	c := newCanvas(20, 10)
	drawGroundShadow(c, 10, 5, 8, 4, 0.45)
	return c
}

// ─────────────────────────────────────────────────────────────────────────────
// Анимации: описание, генерация, сборка спрайтшита (СЕТКА: строки=направления,
// столбцы=кадры) + метаданные (weaver.json).
// ─────────────────────────────────────────────────────────────────────────────

const scale = 2

// 8 направлений. Свет ЭКРАННЫЙ → отражение недопустимо (перевернуло бы свет),
// поэтому все 8 рисуются честно. Порядок строк листа:
var dirNames = []string{"down", "down_left", "left", "up_left", "up", "up_right", "right", "down_right"}
var dirAngles = []float64{0, math.Pi / 4, math.Pi / 2, 3 * math.Pi / 4, math.Pi,
	5 * math.Pi / 4, 3 * math.Pi / 2, 7 * math.Pi / 4}

type animMeta struct {
	Name         string   `json:"name"`
	Part         string   `json:"part"`
	File         string   `json:"file"`
	Frames       int      `json:"frames"`
	Directions   int      `json:"directions"`
	FrameW       int      `json:"frame_w"`
	FrameH       int      `json:"frame_h"`
	FPS          int      `json:"fps"`
	Loop         bool     `json:"loop"`
	Pivot        [2]int   `json:"pivot"`
	Damaging     []int    `json:"damaging_frames"`
	Hitbox       [][4]int `json:"hitbox"`
	WeakpointBox *[4]int  `json:"weakpoint_box,omitempty"` // хитбокс брюшка
	WeakOpen     []int    `json:"weakpoint_open_frames,omitempty"`
	HeightOffset []int    `json:"height_offset,omitempty"` // подъём по кадрам (прыжок)
}

// animSpec — единичная анимация. multiDir=true → рендер по всем 8 направлениям
// (строки листа); иначе одно направление (down).
type animSpec struct {
	name       string
	part       string
	dir        string
	nw, nh     int
	fps        int
	loop       bool
	multiDir   bool
	damaging   []int
	weakOpen   []int // кадры, где слабая точка (брюшко) открыта
	height     func(k, n int) int
	frame      func(facing float64, k, n int) *canvas
	nframes    int
}

func prog(k, n int) float64 {
	if n <= 1 {
		return 0
	}
	return float64(k) / float64(n-1)
}
func loopPh(k, n int) float64 { return 2 * math.Pi * float64(k) / float64(n) }

// bodyFrame — обёртка: рендер тела с направлением + подъём/тонировка.
func bodyFrame(p bodyP, facing float64, lift float64) *canvas {
	p.facing = facing
	base := renderBody(p)
	if lift == 0 {
		return base
	}
	out := newCanvas(bodyN, bodyN)
	blitTransform(out, base, 0, -lift, 1, 1, color.RGBA{}, 0)
	return out
}

func specs() []animSpec {
	var S []animSpec
	body := func(name string, n, fps int, loop bool, dmg, weak []int, height func(k, n int) int, f func(facing float64, k, n int) *canvas) {
		S = append(S, animSpec{name, "body", "body", bodyN, bodyN, fps, loop, true, dmg, weak, height, f, n})
	}
	weakAll := func(n int) []int { // брюшко видно сверху всегда → слабая точка всегда «есть»
		d := make([]int, n)
		for i := range d {
			d[i] = i
		}
		return d
	}

	// ── ТЕЛО (все 8 направлений). Брюшко-слабая-точка открыта во всех кадрах,
	//    кроме кокона; в фазе 3 (_p3) трещины. ──
	body("idle", 6, 8, true, nil, weakAll(6), nil, func(f float64, k, n int) *canvas {
		ph := loopPh(k, n)
		// лёгкое «дыхание»: небольшой переступ + покачивание тела.
		return bodyFrame(bodyP{eggGlow: 0.15 + 0.05*math.Sin(ph), gait: ph, step: 1.6}, f, 0.5*math.Sin(ph))
	})
	body("walk_loop", 8, 12, true, nil, weakAll(8), nil, func(f float64, k, n int) *canvas {
		ph := loopPh(k, n)
		// явный шаг: большая амплитуда переступа + вертикальный боб тела.
		return bodyFrame(bodyP{eggGlow: 0.12, gait: ph, step: 7}, f, 2*math.Sin(ph))
	})
	body("dash_web", 8, 16, false, nil, weakAll(8), nil, func(f float64, k, n int) *canvas {
		ph := loopPh(k, n) * 2
		// быстрый семенящий рывок: частый переступ, тело низко.
		return bodyFrame(bodyP{eggGlow: 0.2, gait: ph, step: 5, splay: 0.2}, f, 1.2*math.Sin(ph))
	})
	body("bite_windup", 6, 10, false, nil, weakAll(6), nil, func(f float64, k, n int) *canvas { // ТЕЛЕГРАФ
		t := prog(k, n)
		// встаёт на дыбы: передние ноги вскинуты, тело откинуто назад и поднято.
		return bodyFrame(bodyP{eggGlow: 0.1, rear: t, step: 1, legLift: t}, f, 5*t)
	})
	body("bite_strike", 4, 18, false, []int{0, 1}, weakAll(4), nil, func(f float64, k, n int) *canvas {
		t := prog(k, n)
		// выпад: с дыбов резко вниз-вперёд, жвала бьют, тело ныряет.
		return bodyFrame(bodyP{eggGlow: 0.1, rear: 0.9 * (1 - t), venomJaw: 0.3}, f, 5-9*t)
	})
	body("bite_recover", 5, 10, false, nil, weakAll(5), nil, func(f float64, k, n int) *canvas { // брюшко открыто (уязвим)
		t := prog(k, n)
		return bodyFrame(bodyP{eggGlow: 0.25, splay: 0.3 * (1 - t)}, f, -2*(1-t))
	})
	body("leap_windup", 6, 10, false, nil, weakAll(6), nil, func(f float64, k, n int) *canvas {
		t := prog(k, n)
		// глубокий присед: ноги сжаты под тело, тело низко перед толчком.
		return bodyFrame(bodyP{eggGlow: 0.1, tuck: 0.5 * t}, f, -4*t)
	})
	body("leap_air", 4, 12, true, nil, weakAll(4), func(k, n int) int { return 18 }, func(f float64, k, n int) *canvas {
		ph := loopPh(k, n)
		// в воздухе: ноги плотно поджаты, тело высоко (тень на полу сжата — см. fx).
		return bodyFrame(bodyP{eggGlow: 0.15, tuck: 1}, f, 18+0.5*math.Sin(ph))
	})
	body("leap_land", 5, 16, false, []int{0}, weakAll(5), nil, func(f float64, k, n int) *canvas {
		t := prog(k, n)
		// удар о землю: ноги растопырены наружу, тело оседает и выпрямляется.
		return bodyFrame(bodyP{eggGlow: 0.15, splay: 1 - t, tuck: 0.4 * (1 - t)}, f, 6*(1-t))
	})
	body("web_shot_windup", 5, 10, false, nil, weakAll(5), nil, func(f float64, k, n int) *canvas { // брюшко к цели
		t := prog(k, n)
		// разворачивается брюшком к цели: задняя часть приподнимается, свечение растёт.
		return bodyFrame(bodyP{eggGlow: 0.2 + 0.3*t, step: 1, splay: 0.2 * t}, f, -1.5*t)
	})
	body("web_shot_fire", 4, 16, false, nil, weakAll(4), nil, func(f float64, k, n int) *canvas {
		t := prog(k, n)
		return bodyFrame(bodyP{eggGlow: 0.35 - 0.2*t}, f, -1.5+3*t) // отдача брюшка
	})
	body("venom_spray", 8, 12, false, []int{2, 3, 4, 5}, weakAll(8), nil, func(f float64, k, n int) *canvas { // фаза 3
		t := prog(k, n)
		return bodyFrame(bodyP{eggGlow: 0.2, cracked: true, rear: 0.6 * math.Sin(math.Pi*t),
			venomJaw: math.Sin(math.Pi * t)}, f, 3*math.Sin(math.Pi*t))
	})
	body("lay_eggsac", 6, 12, false, nil, weakAll(6), nil, func(f float64, k, n int) *canvas {
		t := prog(k, n)
		// приседает и выталкивает мешок: задние ноги растопырены, брюшко опускается.
		return bodyFrame(bodyP{eggGlow: 0.5 - 0.4*t, splay: math.Sin(math.Pi * t)}, f, -2*math.Sin(math.Pi*t))
	})

	// summon_screech — 1 направление (ТЗ). Встаёт на дыбы, свечение выводка вспыхивает.
	S = append(S, animSpec{"summon_screech", "body", "body", bodyN, bodyN, 10, false, false, nil, weakAll(8), nil,
		func(f float64, k, n int) *canvas {
			t := prog(k, n)
			return bodyFrame(bodyP{eggGlow: math.Abs(math.Sin(3 * math.Pi * t)),
				rear: math.Sin(math.Pi * t)}, 0, -3*math.Sin(math.Pi*t))
		}, 8})

	// Кокон-цикл фазы 2 (1 направление): обмотка → неуязвимость → вскрытие.
	single := func(name string, n, fps int, loop bool, weak []int, f func(k, n int) *canvas) {
		S = append(S, animSpec{name, "body", "body", bodyN, bodyN, fps, loop, false, nil, weak, nil,
			func(_ float64, k, n int) *canvas { return f(k, n) }, n})
	}
	single("cocoon_wrap", 10, 12, false, nil, func(k, n int) *canvas {
		t := prog(k, n)
		return bodyFrame(bodyP{cocoon: t, eggGlow: 0.1}, 0, 0)
	})
	single("cocoon_idle", 6, 6, true, nil, func(k, n int) *canvas {
		ph := loopPh(k, n)
		return bodyFrame(bodyP{cocoon: 1, eggGlow: 0.2 + 0.1*math.Sin(ph)}, 0, 0)
	})
	single("cocoon_break", 10, 14, false, nil, func(k, n int) *canvas {
		t := prog(k, n)
		return bodyFrame(bodyP{cocoon: 1 - t, eggGlow: 0.4}, 0, 0)
	})
	single("phase_transition", 10, 10, false, nil, func(k, n int) *canvas { // трескается брюшко
		t := prog(k, n)
		return bodyFrame(bodyP{cracked: t > 0.4, eggGlow: 0.3 + 0.3*math.Sin(6*math.Pi*t)}, 0, -1.5*math.Sin(math.Pi*t))
	})

	// hurt / death — все направления? death 1 направление достаточно, hurt все.
	body("hurt", 2, 20, false, nil, weakAll(2), nil, func(f float64, k, n int) *canvas {
		h := 1.0
		if k == 1 {
			h = 0.4
		}
		return bodyFrame(bodyP{eggGlow: 0.15, hurt: h}, f, 0)
	})
	single("death", 14, 10, false, nil, func(k, n int) *canvas { // ноги подгибаются внутрь, тело оседает
		t := prog(k, n)
		// краткая агония в начале (лёгкая дрожь ног), затем подгиб и оседание.
		twitch := 0.0
		if t < 0.25 {
			twitch = 1.2 * math.Sin(20*t)
		}
		return bodyFrame(bodyP{cracked: true, eggGlow: 0.4 * (1 - t), dead: t, gait: 20 * t, step: twitch}, 0, 0)
	})

	// Фаза-3 варианты видимых циклов (трещина постоянна).
	body("idle_p3", 6, 8, true, nil, weakAll(6), nil, func(f float64, k, n int) *canvas {
		ph := loopPh(k, n)
		return bodyFrame(bodyP{cracked: true, eggGlow: 0.2 + 0.05*math.Sin(ph), gait: ph, step: 1.6}, f, 0.5*math.Sin(ph))
	})
	body("walk_loop_p3", 8, 12, true, nil, weakAll(8), nil, func(f float64, k, n int) *canvas {
		ph := loopPh(k, n)
		return bodyFrame(bodyP{cracked: true, eggGlow: 0.18, gait: ph, step: 7}, f, 2*math.Sin(ph))
	})

	// ── НОГИ (single-frame сегменты) ──
	for _, ls := range legSegs {
		ls := ls
		S = append(S, animSpec{ls.name, "legs", "legs", ls.nw, ls.nh, 1, false, false, nil, nil, nil,
			func(_ float64, k, n int) *canvas { return renderLegSeg(ls) }, 1})
	}

	// ── ПАУЧОК ──
	spider := func(name string, n, fps int, loop bool, dmg []int, f func(k, n int) *canvas) {
		S = append(S, animSpec{name, "spiderling", "spiderling", 20, 20, fps, loop, false, dmg, nil, nil,
			func(_ float64, k, n int) *canvas { return f(k, n) }, n})
	}
	spider("idle", 4, 8, true, []int{0, 1, 2, 3}, func(k, n int) *canvas {
		return renderSpiderling(spiderP{phase: loopPh(k, n)})
	})
	spider("walk", 6, 14, true, []int{0, 1, 2, 3, 4, 5}, func(k, n int) *canvas {
		return renderSpiderling(spiderP{phase: loopPh(k, n) * 2})
	})
	spider("jump", 4, 14, false, nil, func(k, n int) *canvas {
		return renderSpiderling(spiderP{jump: math.Sin(math.Pi * prog(k, n))})
	})
	spider("attack", 4, 16, false, []int{1, 2}, func(k, n int) *canvas {
		return renderSpiderling(spiderP{phase: loopPh(k, n), att: prog(k, n)})
	})
	spider("death", 6, 14, false, nil, func(k, n int) *canvas {
		return renderSpiderling(spiderP{dead: prog(k, n)})
	})

	// ── КОКОН выводка ──
	sac := func(name, stage string, n, fps int, loop bool) {
		S = append(S, animSpec{name, "eggsac", "eggsac", 28, 28, fps, loop, false, nil, nil, nil,
			func(_ float64, k, n int) *canvas { return renderEggsac(stage, k, n) }, n})
	}
	sac("eggsac_idle", "idle", 4, 6, true)
	sac("eggsac_pulse", "pulse", 6, 10, false)
	sac("eggsac_hatch", "hatch", 8, 14, false)
	sac("eggsac_destroyed", "destroyed", 6, 14, false)

	// ── FX ──
	fx := func(name, dir string, nw, nh, n, fps int, loop bool, dmg []int, f func(k, n int) *canvas) {
		S = append(S, animSpec{name, "fx", dir, nw, nh, fps, loop, false, dmg, nil, nil,
			func(_ float64, k, n int) *canvas { return f(k, n) }, n})
	}
	fx("web_projectile", "fx", 10, 10, 4, 12, true, []int{0, 1, 2, 3}, fxWebProjectile)
	fx("web_patch_spawn", "fx", 32, 32, 6, 12, false, nil, func(k, n int) *canvas { return fxWebPatch("spawn", k, n) })
	fx("web_patch_loop", "fx", 32, 32, 6, 8, true, nil, func(k, n int) *canvas { return fxWebPatch("loop", k, n) })
	fx("web_patch_break", "fx", 32, 32, 6, 14, false, nil, func(k, n int) *canvas { return fxWebPatch("break", k, n) })
	fx("venom_pool", "fx", 32, 16, 6, 8, true, []int{0, 1, 2, 3, 4, 5}, fxVenomPool)
	fx("leap_shadow_warning", "fx", 32, 32, 6, 10, false, nil, fxLeapWarning)
	fx("silk_burst", "fx", 32, 32, 8, 16, false, nil, fxSilkBurst)
	fx("shadow_leap", "fx", 56, 24, 10, 16, false, nil, fxShadowLeap)

	// ── ТЕНИ на полу (shadows/) ──
	S = append(S, animSpec{"body_shadow", "shadows", "shadows", 56, 24, 6, false, false, nil, nil, nil,
		func(_ float64, k, n int) *canvas { return fxBodyShadow(k, n) }, 1})
	S = append(S, animSpec{"spiderling_shadow", "shadows", "shadows", 20, 10, 6, false, false, nil, nil, nil,
		func(_ float64, k, n int) *canvas { return fxSpiderShadow(k, n) }, 1})

	return S
}

// upscaleInto — вписывает нативный кадр в лист по (row,col) с апскейлом ×2.
func upscaleInto(sheet *image.RGBA, frame *canvas, row, col int) {
	ox := col * frame.w * scale
	oy := row * frame.h * scale
	for y := 0; y < frame.h; y++ {
		for x := 0; x < frame.w; x++ {
			c := frame.at(x, y)
			for dy := 0; dy < scale; dy++ {
				for dx := 0; dx < scale; dx++ {
					sheet.Set(ox+x*scale+dx, oy+y*scale+dy, c)
				}
			}
		}
	}
}

func savePNG(path string, img image.Image) error {
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
	root := "assets/sprites/bosses/weaver"
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		fmt.Println("mkdir:", err)
		os.Exit(1)
	}

	// palette_additions.hex — ровно 4 новых оттенка (лимит ТЗ).
	var pb []byte
	pb = append(pb, []byte("# weaver — ДОБАВЛЕНИЯ к палитре world_eater (4 новых оттенка)\n")...)
	pb = append(pb, []byte("# База (хитин/плоть/тень/глаза) наследуется из world_eater/palette.hex\n")...)
	for _, p := range paletteAdditions {
		pb = append(pb, []byte(fmt.Sprintf("#%02X%02X%02X  %s\n", p.c.R, p.c.G, p.c.B, p.name))...)
	}
	if err := os.WriteFile(filepath.Join(root, "palette_additions.hex"), pb, 0o644); err != nil {
		fmt.Println("palette:", err)
	}

	var metas []animMeta
	for _, sp := range specs() {
		rows := 1
		if sp.multiDir {
			rows = len(dirNames)
		}
		fw, fh := sp.nw*scale, sp.nh*scale
		sheet := image.NewRGBA(image.Rect(0, 0, fw*sp.nframes, fh*rows))
		hitboxes := make([][4]int, sp.nframes) // хитбокс берём с направления down (row 0)
		for r := 0; r < rows; r++ {
			facing := 0.0
			if sp.multiDir {
				facing = dirAngles[r]
			}
			for k := 0; k < sp.nframes; k++ {
				native := outlined(sp.frame(facing, k, sp.nframes))
				upscaleInto(sheet, native, r, k)
				if r == 0 {
					minx, miny, maxx, maxy := native.bbox()
					hitboxes[k] = [4]int{minx * scale, miny * scale, (maxx - minx + 1) * scale, (maxy - miny + 1) * scale}
				}
			}
		}
		file := filepath.Join(sp.dir, sp.name+".png")
		if err := savePNG(filepath.Join(root, file), sheet); err != nil {
			fmt.Println("save:", file, err)
			continue
		}

		m := animMeta{
			Name: sp.name, Part: sp.part, File: file, Frames: sp.nframes,
			Directions: rows, FrameW: fw, FrameH: fh, FPS: sp.fps, Loop: sp.loop,
			Pivot: [2]int{fw / 2, fh / 2}, Damaging: sp.damaging, Hitbox: hitboxes,
		}
		if sp.damaging == nil {
			m.Damaging = []int{}
		}
		// weakpoint_box — только у тела: хитбокс брюшка (тыльная половина кадра).
		if sp.part == "body" && sp.weakOpen != nil {
			// брюшко в native ~ центр (32,34) r=20 → в кадре ×2, но смещено «назад»
			wb := [4]int{22 * scale, 24 * scale, 24 * scale, 24 * scale}
			m.WeakpointBox = &wb
			m.WeakOpen = sp.weakOpen
		}
		if sp.height != nil {
			hs := make([]int, sp.nframes)
			for k := range hs {
				hs[k] = sp.height(k, sp.nframes) * scale
			}
			m.HeightOffset = hs
		}
		metas = append(metas, m)
	}

	sort.Slice(metas, func(i, j int) bool {
		if metas[i].Part != metas[j].Part {
			return metas[i].Part < metas[j].Part
		}
		return metas[i].Name < metas[j].Name
	})

	// leg_attach_points — 8 точек крепления ног к телу (в координатах кадра тела,
	// после ×2). Порядок: 4 слева (перёд→зад), 4 справа. Свет экранный.
	legAttach := [][2]int{}
	for _, s := range []float64{-1, 1} {
		for i := 0; i < 4; i++ {
			// вдоль оси тела: перёд (головогрудь) → зад (брюшко)
			ax := 32 + int(s*9)          // сбоку от оси
			ay := 26 + i*6               // от головогруди к брюшку
			legAttach = append(legAttach, [2]int{ax * scale, ay * scale})
		}
	}

	legMeta := []map[string]any{}
	pivot := 0
	for _, ls := range legSegs {
		legMeta = append(legMeta, map[string]any{
			"name":       ls.name,
			"file":       filepath.Join("legs", ls.name+".png"),
			"length_px":  int(ls.length) * scale,
			"root_pivot": [2]int{1 * scale, (ls.nh / 2) * scale}, // основание (шарнир к предыдущему)
			"tip_pivot":  [2]int{int(ls.length) * scale, (ls.nh / 2) * scale},
		})
		pivot++
	}

	doc := map[string]any{
		"name":             "weaver",
		"title":            "Ткачиха",
		"style":            "pixel-art, tilted top-down ~65° (reference: Necesse), volumetric soft shading",
		"reference":        "The Weaver (Path of Exile 1)",
		"frame_convention": "grid spritesheet — rows = directions, columns = frames (left-to-right)",
		"directions":       dirNames,
		"native_scale":     scale,
		"palette_base":     "../world_eater/palette.hex",
		"palette":          "palette_additions.hex",
		"light_dir":        "screen-space top-front-left, fixed for all directions/frames (no mirroring)",
		"legs_mode":        "hybrid — legs baked into every body frame (procedural gait/pose, so sheets are self-legible) AND modular leg_segments + leg_attach_points exported for runtime IK if desired",
		"leg_attach_points": legAttach,
		"leg_segments":     legMeta,
		"weakpoint":        map[string]any{"part": "abdomen (brzucho)", "damage_mult": 2, "builds_stagger": true, "note": "brzucho видно сверху всегда; weakpoint_box в кадрах тела"},
		"phase3_suffix":    "_p3",
		"coordinate_origin": "top-left, pixels в координатах кадра (после апскейла ×2)",
		"animations":       metas,
	}
	jb, _ := json.MarshalIndent(doc, "", "  ")
	if err := os.WriteFile(filepath.Join(root, "weaver.json"), jb, 0o644); err != nil {
		fmt.Println("json:", err)
	}

	fmt.Printf("weaver: %d анимаций → %s\n", len(metas), root)
}
