// Инструмент envgen — процедурный генератор ЭЛЕМЕНТОВ ПРОСТРАНСТВА (плитки/пропы)
// боссовой арены «Плетёная Могила» (лэйр Ткачихи). Отдельный package main, только
// stdlib, НЕ часть игрового бинарника — как tools/weavergen/bossgen для существ.
//
// Рисует всё кодом в той же наклонной проекции ~65° (Necesse) и с тем же единым
// светом сверху-спереди-слева, что и Ткачиха, — чтобы стиль совпадал. Печёт:
//
//	tiles/  — отдельные плитки/пропы (пол, стена, помост, альков, кладка,
//	          свисающий кокон, husk, мембрана входа)
//	preview.png — СОБРАННОЕ превью всей арены по слоям (rendering.md):
//	          фон → пол → напольный свет → стена → помост → объекты → сущность
//	          (Ткачиха, если найдена) → мембрана → надземные коконы → виньетка.
//	README.md — каталог с превью и плитками.
//
// По умолчанию пишет в docs/design/maps/pletenaya-mogila (НЕ в assets/), чтобы
// сперва посмотреть. Когда одобрим — тот же запуск с путём внутрь assets/ печёт
// боевые ассеты:  go run ./tools/envgen assets/sprites/env/weaver_grave
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

func rgb(r, g, b uint8) color.RGBA { return color.RGBA{r, g, b, 255} }

// ─── Палитра: наследуется от weaver/world_eater + шёлк. Приглушённо-землистая. ───
var (
	Void = rgb(0x0a, 0x09, 0x12) // бездна за стенами

	// Шёлк (grey-violet) — пол, стены, коконы, мембрана.
	SilkSh = rgb(0x2a, 0x2a, 0x38)
	SilkDk = rgb(0x3a, 0x3a, 0x4c)
	SilkMd = rgb(0x6f, 0x6e, 0x82)
	SilkLt = rgb(0x9a, 0x99, 0xad)
	SilkHi = rgb(0xd6, 0xd4, 0xe2)

	// Хитин (для husks — высохших панцирей) — пыльно-фиолетовый.
	ChitinSh2 = rgb(0x0e, 0x0b, 0x18)
	ChitinSh  = rgb(0x1a, 0x15, 0x2a)
	ChitinDk  = rgb(0x2a, 0x20, 0x40)
	ChitinMd  = rgb(0x40, 0x30, 0x5e)
	ChitinLt  = rgb(0x5c, 0x45, 0x82)

	// Яд — свечение кладок (зелёный акцент, точечно).
	VenomCore = rgb(0x9e, 0xb8, 0x2e)
	// Трупная плоть — силуэт человека внутри кокона.
	Corpse = rgb(0x6a, 0x5c, 0x52)

	Shadow = rgb(0x08, 0x07, 0x0e)
)

var (
	silkFloor = []color.RGBA{SilkSh, SilkDk, SilkMd, SilkLt}
	silkR     = []color.RGBA{SilkDk, SilkMd, SilkLt, SilkHi}
	chitin5   = []color.RGBA{ChitinSh2, ChitinSh, ChitinDk, ChitinMd, ChitinLt}
	venomR    = []color.RGBA{mixCol(VenomCore, Shadow, 0.5), VenomCore, mixCol(VenomCore, SilkHi, 0.5)}
	corpseR   = []color.RGBA{mixCol(Corpse, Shadow, 0.4), Corpse, mixCol(Corpse, SilkLt, 0.3)}
)

var lightDir = [2]float64{-0.55, -0.78} // единый экранный свет: сверху-слева

// ─────────────────────────── canvas и примитивы ───────────────────────────

type canvas struct {
	w, h int
	px   []color.RGBA
}

func newCanvas(w, h int) *canvas   { return &canvas{w, h, make([]color.RGBA, w*h)} }
func (c *canvas) in(x, y int) bool { return x >= 0 && y >= 0 && x < c.w && y < c.h }
func (c *canvas) at(x, y int) color.RGBA {
	if !c.in(x, y) {
		return color.RGBA{}
	}
	return c.px[y*c.w+x]
}
func (c *canvas) set(x, y int, col color.RGBA) {
	if c.in(x, y) && col.A > 0 {
		c.px[y*c.w+x] = col
	}
}
func (c *canvas) blend(x, y int, col color.RGBA) {
	if !c.in(x, y) || col.A == 0 {
		return
	}
	if col.A == 255 {
		c.px[y*c.w+x] = col
		return
	}
	d := c.px[y*c.w+x]
	a := float64(col.A) / 255
	mix := func(s, dd uint8) uint8 { return uint8(float64(s)*a + float64(dd)*(1-a)) }
	na := uint8(float64(col.A) + float64(d.A)*(1-a))
	c.px[y*c.w+x] = color.RGBA{mix(col.R, d.R), mix(col.G, d.G), mix(col.B, d.B), maxu8(na, d.A)}
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
	t = clamp01(t)
	return r[int(t*float64(len(r)-1)+0.5)]
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

// domeSphere — top-down объёмная сфера (свет экранный сверху-слева), tilt<1 сплющивает.
func (c *canvas) domeSphere(cx, cy, r, tilt float64, ramp []color.RGBA, boost float64) {
	ry := r * tilt
	for y := int(cy - ry - 1); y <= int(cy+ry+1); y++ {
		for x := int(cx - r - 1); x <= int(cx+r+1); x++ {
			nx, ny := (float64(x)-cx)/r, (float64(y)-cy)/ry
			d2 := nx*nx + ny*ny
			if d2 > 1 {
				continue
			}
			nz := math.Sqrt(1 - d2)
			diff := nx*lightDir[0] + ny*lightDir[1]
			t := 0.30 + 0.55*(diff*0.5+0.5) + 0.15*nz
			c.set(x, y, rampAt(ramp, clamp01(t*boost)))
		}
	}
}

func (c *canvas) softEllipse(cx, cy, rx, ry float64, ramp []color.RGBA) {
	for y := int(cy - ry - 1); y <= int(cy+ry+1); y++ {
		for x := int(cx - rx - 1); x <= int(cx+rx+1); x++ {
			dx, dy := (float64(x)-cx)/rx, (float64(y)-cy)/ry
			if dx*dx+dy*dy > 1 {
				continue
			}
			nz := math.Sqrt(clamp01(1 - (dx*dx + dy*dy)))
			sd := (float64(x)-cx)/(rx+0.001)*lightDir[0] + (float64(y)-cy)/(ry+0.001)*lightDir[1]
			t := 0.32 + 0.5*(sd*0.5+0.5) + 0.18*nz
			c.set(x, y, rampAt(ramp, clamp01(t)))
		}
	}
}

func (c *canvas) softGlow(cx, cy, r float64, ramp []color.RGBA, intensity float64) {
	if r <= 0 {
		return
	}
	for y := int(cy - r - 1); y <= int(cy+r+1); y++ {
		for x := int(cx - r - 1); x <= int(cx+r+1); x++ {
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

func (c *canvas) taperedLimb(bx, by, tx, ty, r0, r1 float64, ramp []color.RGBA) {
	steps := int(math.Hypot(tx-bx, ty-by)) + 1
	for i := 0; i <= steps; i++ {
		f := float64(i) / float64(steps)
		c.domeSphere(bx+(tx-bx)*f, by+(ty-by)*f, r0+(r1-r0)*f, 1, ramp, 1)
	}
}

func drawSoftLine(c *canvas, x0, y0, x1, y1 float64, col color.RGBA) {
	steps := int(math.Hypot(x1-x0, y1-y0)) + 1
	for i := 0; i <= steps; i++ {
		f := float64(i) / float64(steps)
		c.blend(int(x0+(x1-x0)*f), int(y0+(y1-y0)*f), col)
	}
}

func groundShadow(c *canvas, cx, cy, rx, ry, alpha float64) {
	for y := int(cy - ry - 1); y <= int(cy+ry+1); y++ {
		for x := int(cx - rx - 1); x <= int(cx+rx+1); x++ {
			dx, dy := (float64(x)-cx)/rx, (float64(y)-cy)/ry
			d := dx*dx + dy*dy
			if d <= 1 {
				c.blend(x, y, color.RGBA{Shadow.R, Shadow.G, Shadow.B, uint8(clamp01(alpha*(1-d*d)) * 255)})
			}
		}
	}
}

// h2 — детерминированный хеш-шум 0..1 (для крапа/вариаций; wrap для бесшовности).
func h2(x, y int) float64 {
	n := x*374761393 + y*668265263
	n = (n ^ (n >> 13)) * 1274126177
	n = n ^ (n >> 16)
	if n < 0 {
		n = -n
	}
	return float64(n%100000) / 100000.0
}

// ─────────────────────────────── ПЛИТКИ / ПРОПЫ ───────────────────────────────

const tileN = 48 // сторона плитки пола (нативно)

// renderFloor — плитка шёлкового пола, БЕСШОВНАЯ (нити-волны с периодом = tileN).
func renderFloor(variant int) *canvas {
	c := newCanvas(tileN, tileN)
	for y := 0; y < tileN; y++ {
		for x := 0; x < tileN; x++ {
			// лёгкая тонировка (слабая волна + мелкое зерно) — без «пузырей»
			m := 0.05*math.Sin(2*math.Pi*float64(x)/tileN) + 0.05*math.Sin(2*math.Pi*float64(y)/tileN)
			n := (h2(x, y) - 0.5) * 0.10
			c.set(x, y, rampAt(silkFloor, 0.42+m+n))
		}
	}
	// плетение: горизонтальные и вертикальные нити с лёгкой волной (период tileN)
	off := variant * 5
	for i := 0; i < 4; i++ {
		yy := (i*12 + off) % tileN
		for x := 0; x < tileN; x++ {
			wy := yy + int(math.Round(1.2*math.Sin(2*math.Pi*float64(x)/tileN)))
			c.set(x, (wy+tileN)%tileN, rampAt(silkR, 0.55))
			c.set(x, (wy+1+tileN)%tileN, rampAt(silkFloor, 0.2)) // тень под нитью
		}
		xx := (i*12 + off + 6) % tileN
		for y := 0; y < tileN; y++ {
			wx := xx + int(math.Round(1.0*math.Sin(2*math.Pi*float64(y)/tileN)))
			c.blend((wx+tileN)%tileN, y, color.RGBA{SilkLt.R, SilkLt.G, SilkLt.B, 120})
		}
	}
	// крап: пылинки и редкие ошмётки хитина
	for y := 0; y < tileN; y++ {
		for x := 0; x < tileN; x++ {
			hv := h2(x+off, y-off)
			if hv > 0.986 {
				c.set(x, y, ChitinSh)
			} else if hv > 0.978 {
				c.blend(x, y, color.RGBA{SilkHi.R, SilkHi.G, SilkHi.B, 90})
			}
		}
	}
	return c
}

// renderWall — прямой сегмент стены-паутины с ВЫСОТОЙ: верхняя грань (светлее) +
// передняя грань (темнее) → читается как приподнятый борт. Нативно 48×40.
func renderWall() *canvas {
	c := newCanvas(48, 40)
	const topH = 16 // высота верхней грани
	for y := 0; y < 40; y++ {
		for x := 0; x < 48; x++ {
			if y < topH { // верхняя грань — плотная паутина, освещена
				m := 0.55 + 0.2*math.Sin(2*math.Pi*float64(x)/24) + 0.15*(1-float64(y)/topH)
				c.set(x, y, rampAt(silkR, clamp01(m)))
			} else { // передняя грань — темнее, тяжи вниз
				t := 0.32 - 0.22*float64(y-topH)/float64(40-topH)
				c.set(x, y, rampAt(silkFloor, clamp01(t)))
			}
		}
	}
	// гребень паутины по верху + нити свисают с передней грани внутрь
	for x := 0; x < 48; x += 3 {
		c.set(x, 0, SilkHi)
		c.set(x, 1, SilkLt)
	}
	for i := 0; i < 6; i++ {
		x := 4 + i*8
		for y := topH; y < 40; y += 1 {
			if h2(x, y) > 0.4 {
				c.blend(x, y, color.RGBA{SilkMd.R, SilkMd.G, SilkMd.B, 150})
			}
		}
	}
	return c
}

// renderDais — центральный шёлковый помост (низкий купол, обмотан прядями,
// в центре — тёмная лунка, куда уходит в кокон; лёгкое ядовитое свечение). 120×120.
func renderDais() *canvas {
	c := newCanvas(120, 120)
	const cx, cy = 60.0, 62.0
	groundShadow(c, cx, cy+8, 54, 24, 0.55)
	c.domeSphere(cx, cy, 50, 0.5, silkFloor, 1.05) // плоский купол шёлка
	// пряди-обмотка
	for b := 0; b < 7; b++ {
		ang := float64(b)/7*math.Pi + 0.2
		for t := -50.0; t <= 50; t += 0.7 {
			x := cx + math.Cos(ang)*t
			y := cy + math.Sin(ang)*t*0.5
			off := math.Sin(t*0.5+float64(b)) * 2
			shade := 0.4 + 0.45*((math.Cos(ang)*lightDir[0]+math.Sin(ang)*lightDir[1])*0.5+0.5)
			c.blend(int(x+math.Cos(ang+math.Pi/2)*off), int(y+math.Sin(ang+math.Pi/2)*off*0.5),
				color.RGBA{rampAt(silkR, shade).R, rampAt(silkR, shade).G, rampAt(silkR, shade).B, 150})
		}
	}
	// тёмная лунка в центре + слабое ядовитое свечение изнутри
	c.domeSphere(cx, cy-2, 16, 0.5, []color.RGBA{Shadow, ChitinSh2, ChitinSh}, 1)
	c.softGlow(cx, cy-2, 12, venomR, 0.30)
	return c
}

// renderAlcove — периметральная ниша: тёмная арка-углубление в паутине с
// шёлковым обрамлением и ложем для кокона выводка. 72×56.
func renderAlcove() *canvas {
	c := newCanvas(72, 56)
	const cx, cy = 36.0, 30.0
	// шёлковое обрамление (светлое кольцо)
	c.softEllipse(cx, cy, 30, 24, silkR)
	// тёмный провал ниши
	c.softEllipse(cx, cy-2, 22, 17, []color.RGBA{Shadow, ChitinSh2, ChitinSh})
	c.softEllipse(cx, cy-3, 16, 12, []color.RGBA{Void, Shadow})
	// ложе-паутина по нижнему краю
	for x := 14; x < 58; x++ {
		if h2(x, 44) > 0.35 {
			c.blend(x, 44+int(2*math.Sin(float64(x)*0.4)), color.RGBA{SilkMd.R, SilkMd.G, SilkMd.B, 170})
		}
	}
	return c
}

// renderEggCluster — кладка светящихся яиц (источник зелёного света). 40×40.
func renderEggCluster() *canvas {
	c := newCanvas(40, 40)
	const cx, cy = 20.0, 22.0
	groundShadow(c, cx, cy+4, 15, 7, 0.4)
	c.softGlow(cx, cy, 18, venomR, 0.45) // ореол света
	// шёлковая подстилка
	c.softEllipse(cx, cy+2, 14, 8, silkFloor)
	// гроздь яиц
	eggs := [][2]float64{{-6, 0}, {0, -3}, {6, 1}, {-3, 4}, {3, 4}, {0, 8}}
	for i, e := range eggs {
		ex, ey := cx+e[0], cy+e[1]
		c.domeSphere(ex, ey, 4.2, 0.85, []color.RGBA{mixCol(VenomCore, Shadow, 0.6), mixCol(VenomCore, Shadow, 0.2), VenomCore}, 1)
		c.set(int(ex-1), int(ey-1), rampAt(venomR, 1)) // блик
		_ = i
	}
	c.softGlow(cx, cy, 8, venomR, 0.5)
	return c
}

// renderHangingCocoon — свисающий кокон-жертва (НАДЗЕМНОЕ): вертикальный свёрток
// шёлка, внутри — силуэт человека; сверху нить крепления. 44×72.
func renderHangingCocoon() *canvas {
	c := newCanvas(44, 72)
	const cx = 22.0
	// нить крепления сверху
	for y := 0; y < 14; y++ {
		c.blend(int(cx+math.Sin(float64(y)*0.3)), y, color.RGBA{SilkLt.R, SilkLt.G, SilkLt.B, 180})
	}
	// тело кокона (веретено)
	c.softEllipse(cx, 42, 15, 28, silkR)
	// силуэт человека внутри (просвечивает)
	c.softEllipse(cx, 34, 6, 8, corpseR) // голова/плечи
	c.softEllipse(cx, 50, 7, 14, []color.RGBA{mixCol(Corpse, Shadow, 0.5), mixCol(Corpse, Shadow, 0.2)})
	c.set(int(cx-2), 33, Shadow) // тёмные глазницы
	c.set(int(cx+2), 33, Shadow)
	// обмотка прядями поверх
	for b := 0; b < 8; b++ {
		yy := 18 + b*6
		for x := -14.0; x <= 14; x += 1 {
			xx := cx + x
			yb := float64(yy) + math.Sin(x*0.3+float64(b))*1.5
			if (x*x)/196+math.Pow(yb-42, 2)/784 <= 1 {
				c.blend(int(xx), int(yb), color.RGBA{SilkHi.R, SilkHi.G, SilkHi.B, 90})
			}
		}
	}
	return c
}

// renderHusk — высохший панцирь прошлой добычи (пустой, лапки поджаты). 28×24.
func renderHusk(variant int) *canvas {
	c := newCanvas(28, 24)
	const cx, cy = 14.0, 13.0
	groundShadow(c, cx, cy+3, 11, 5, 0.4)
	// поджатые лапки
	for i := 0; i < 6; i++ {
		a := (float64(i)/6*2*math.Pi + float64(variant))
		tx := cx + math.Cos(a)*9
		ty := cy + math.Sin(a)*6
		mx := cx + math.Cos(a)*5
		my := cy + math.Sin(a)*3 - 2
		c.taperedLimb(cx, cy, mx, my, 1.2, 0.9, chitin5)
		c.taperedLimb(mx, my, tx, ty, 0.9, 0.3, chitin5)
	}
	// пустой панцирь-купол с тёмным провалом (выеден изнутри)
	c.domeSphere(cx, cy, 7, 0.7, chitin5, 0.9)
	c.softEllipse(cx, cy-1, 4, 3, []color.RGBA{Shadow, ChitinSh2})
	return c
}

// renderMembrane — шёлковая мембрана входа: плотные вертикальные пряди,
// полупрозрачные, с намёком на разрыв по центру. 96×40.
func renderMembrane() *canvas {
	c := newCanvas(96, 40)
	for i := 0; i < 40; i++ {
		x := 2 + float64(i)*2.4
		part := math.Abs(x-48) / 48 // к центру — реже (разрыв)
		for y := 0; y < 40; y++ {
			if h2(int(x), y) < 0.2+0.5*part {
				continue
			}
			shade := 0.4 + 0.4*part
			c.blend(int(x+math.Sin(float64(y)*0.2+x)*0.8), y,
				color.RGBA{rampAt(silkR, shade).R, rampAt(silkR, shade).G, rampAt(silkR, shade).B, uint8(110 + 90*part)})
		}
	}
	return c
}

// ─────────────────────────── СБОРКА ПРЕВЬЮ (по слоям) ───────────────────────────

const pN = 380 // нативная сторона превью

func octa(dx, dy, R float64) bool {
	adx, ady := math.Abs(dx), math.Abs(dy)
	return adx <= R && ady <= R && adx+ady <= R*1.35
}

// blitMasked — перенос src на dst в (ox,oy), фильтр inside(px,py).
func (dst *canvas) blitMasked(src *canvas, ox, oy int, inside func(px, py int) bool) {
	for y := 0; y < src.h; y++ {
		for x := 0; x < src.w; x++ {
			px, py := ox+x, oy+y
			if inside == nil || inside(px, py) {
				dst.blend(px, py, src.at(x, y))
			}
		}
	}
}

func compose(floors []*canvas, wall, dais, alcove, egg, cocoon, husk0, husk1, membrane, boss *canvas) *canvas {
	c := newCanvas(pN, pN)
	cx, cy := float64(pN)/2, float64(pN)/2
	R, innerR := 172.0, 158.0
	gapHalf := 30.0 // полуширина входа (юг)
	inFloor := func(px, py int) bool { return octa(float64(px)-cx, float64(py)-cy, innerR) }

	// 1) ФОН — бездна + дальние нити
	for i := range c.px {
		c.px[i] = Void
	}
	for i := 0; i < 40; i++ {
		a := float64(i) / 40 * 2 * math.Pi
		drawSoftLine(c, cx, cy, cx+math.Cos(a)*260, cy+math.Sin(a)*260,
			color.RGBA{SilkSh.R, SilkSh.G, SilkSh.B, 40})
	}

	// 2) ПОЛ — тайлим плитки, обрезая по восьмиугольнику
	for ty := int(cy - R); ty < int(cy+R); ty += tileN {
		for tx := int(cx - R); tx < int(cx+R); tx += tileN {
			v := (tx/tileN + ty/tileN) % len(floors)
			if v < 0 {
				v += len(floors)
			}
			c.blitMasked(floors[v], tx, ty, inFloor)
		}
	}

	// 3) НАПОЛЬНЫЙ СВЕТ — зелёные лужи света под будущими кладками (на полу)
	for i := 0; i < 8; i++ {
		a := float64(i)/8*2*math.Pi + math.Pi/8
		ex, ey := cx+math.Cos(a)*(innerR-24), cy+math.Sin(a)*(innerR-24)
		c.softGlow(ex, ey, 30, venomR, 0.18)
	}

	// 6) СТЕНА — кольцо между octa(R) и octa(innerR); вход на юге вырезан
	for y := 0; y < pN; y++ {
		for x := 0; x < pN; x++ {
			dx, dy := float64(x)-cx, float64(y)-cy
			if !octa(dx, dy, R) || octa(dx, dy, innerR) {
				continue
			}
			if dy > innerR-8 && math.Abs(dx) < gapHalf { // проём входа
				continue
			}
			depth := (R - math.Max(math.Abs(dx), math.Abs(dy))) / (R - innerR) // 0 снаружи..1 внутри
			// внутренняя кромка — верхняя грань (светлее), наружу — темнее
			t := 0.25 + 0.5*clamp01(depth) + 0.15*((dx/R)*lightDir[0]+(dy/R)*lightDir[1])
			c.set(x, y, rampAt(silkR, clamp01(t)))
			if h2(x, y) > 0.9 { // гребень паутины
				c.set(x, y, SilkHi)
			}
		}
	}

	// 4/5) ОБЪЕКТЫ: помост в центре, альковы+кладки по периметру
	c.blitMasked(dais, int(cx)-dais.w/2, int(cy)-dais.h/2, nil)
	for i := 0; i < 8; i++ {
		a := float64(i)/8*2*math.Pi + math.Pi/8
		if math.Abs(math.Sin(a)-1) < 0.25 && math.Cos(a) > -0.4 && math.Cos(a) < 0.4 {
			continue // южную позицию отдаём входу
		}
		ax := cx + math.Cos(a)*(innerR-20)
		ay := cy + math.Sin(a)*(innerR-20)
		c.blitMasked(alcove, int(ax)-alcove.w/2, int(ay)-alcove.h/2, nil)
		c.blitMasked(egg, int(ax)-egg.w/2, int(ay)-egg.h/2+4, nil)
	}

	// объекты на полу: husks (детерминированный разброс)
	husks := [][3]float64{{-70, 40, 0}, {90, -30, 1}, {30, 80, 0}, {-100, -60, 1}, {110, 70, 0}, {-40, -95, 1}}
	for _, hh := range husks {
		hx, hy := cx+hh[0], cy+hh[1]
		if !octa(hx-cx, hy-cy, innerR-10) {
			continue
		}
		hs := husk0
		if hh[2] > 0.5 {
			hs = husk1
		}
		c.blitMasked(hs, int(hx)-hs.w/2, int(hy)-hs.h/2, nil)
	}

	// 5-сущность) ТКАЧИХА в центре (если спрайт найден) — тень + тело
	if boss != nil {
		bx, by := int(cx)-boss.w/2, int(cy)-boss.h/2-6
		groundShadow(c, cx, cy+float64(boss.h)/2-8, float64(boss.w)*0.42, float64(boss.w)*0.18, 0.5)
		c.blitMasked(boss, bx, by, nil)
	}

	// 6-вход) МЕМБРАНА в южном проёме
	c.blitMasked(membrane, int(cx)-membrane.w/2, int(cy+innerR)-membrane.h/2, nil)

	// 7) НАДЗЕМНОЕ — свисающие коконы у верхних граней (поверх всего), с тенью на пол
	hang := [][2]float64{{-110, -120}, {70, -135}, {150, -30}, {-160, 10}, {130, 110}}
	for _, hp := range hang {
		hx, hy := cx+hp[0], cy+hp[1]
		if math.Abs(hx-cx) > R+10 || math.Abs(hy-cy) > R+10 {
			continue
		}
		// слабая тень на полу под коконом (если над полом)
		if octa(hx-cx, hy-cy+40, innerR) {
			groundShadow(c, hx, hy+float64(cocoon.h)/2, 14, 6, 0.25)
		}
		c.blitMasked(cocoon, int(hx)-cocoon.w/2, int(hy)-cocoon.h/2, nil)
	}

	// 8) СВЕТ/ВИНЬЕТКА — мягкое затемнение к краям (внутри), зелёные акценты уже есть
	for y := 0; y < pN; y++ {
		for x := 0; x < pN; x++ {
			d := math.Hypot(float64(x)-cx, float64(y)-cy) / (R * 1.15)
			if d > 0.6 {
				a := clamp01((d - 0.6) / 0.6)
				c.blend(x, y, color.RGBA{Void.R, Void.G, Void.B, uint8(a * 150)})
			}
		}
	}
	return c
}

// ─────────────────────────────── I/O ───────────────────────────────

const scale = 2

func upscale(c *canvas) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, c.w*scale, c.h*scale))
	for y := 0; y < c.h; y++ {
		for x := 0; x < c.w; x++ {
			col := c.at(x, y)
			for dy := 0; dy < scale; dy++ {
				for dx := 0; dx < scale; dx++ {
					img.Set(x*scale+dx, y*scale+dy, col)
				}
			}
		}
	}
	return img
}

func savePNG(path string, img image.Image) {
	os.MkdirAll(filepath.Dir(path), 0o755)
	f, err := os.Create(path)
	if err != nil {
		fmt.Println("save:", err)
		return
	}
	defer f.Close()
	png.Encode(f, img)
}

// loadBossIdle — грузит первый кадр weaver idle (128×128) и ужимает в target нативных
// px боксовым усреднением (для масштаба в превью). Терпимо к отсутствию файла.
func loadBossIdle(target int) *canvas {
	f, err := os.Open("assets/sprites/bosses/weaver/body/idle.png")
	if err != nil {
		return nil
	}
	defer f.Close()
	src, err := png.Decode(f)
	if err != nil {
		return nil
	}
	const fw = 128 // первый кадр (row 0 = down)
	out := newCanvas(target, target)
	sc := float64(fw) / float64(target)
	for y := 0; y < target; y++ {
		for x := 0; x < target; x++ {
			var r, g, b, a, n float64
			for sy := 0; sy < int(sc)+1; sy++ {
				for sx := 0; sx < int(sc)+1; sx++ {
					px, py := int(float64(x)*sc)+sx, int(float64(y)*sc)+sy
					if px >= fw || py >= fw {
						continue
					}
					cr, cg, cb, ca := src.At(px, py).RGBA()
					if ca>>8 < 16 {
						continue
					}
					r += float64(cr >> 8)
					g += float64(cg >> 8)
					b += float64(cb >> 8)
					a += float64(ca >> 8)
					n++
				}
			}
			if n > 0 {
				out.set(x, y, color.RGBA{uint8(r / n), uint8(g / n), uint8(b / n), uint8(a / n)})
			}
		}
	}
	return out
}

func main() {
	out := "docs/design/maps/pletenaya-mogila"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	tiles := filepath.Join(out, "tiles")

	floors := []*canvas{renderFloor(0), renderFloor(1), renderFloor(2)}
	wall := renderWall()
	dais := renderDais()
	alcove := renderAlcove()
	egg := renderEggCluster()
	cocoon := renderHangingCocoon()
	husk0, husk1 := renderHusk(0), renderHusk(1)
	membrane := renderMembrane()

	named := map[string]*canvas{
		"floor_a": floors[0], "floor_b": floors[1], "floor_c": floors[2],
		"wall": wall, "dais": dais, "alcove": alcove, "egg_cluster": egg,
		"hanging_cocoon": cocoon, "husk_a": husk0, "husk_b": husk1, "membrane": membrane,
	}
	for name, cv := range named {
		savePNG(filepath.Join(tiles, name+".png"), upscale(cv))
	}

	boss := loadBossIdle(46) // ~ масштаб к арене
	preview := compose(floors, wall, dais, alcove, egg, cocoon, husk0, husk1, membrane, boss)
	savePNG(filepath.Join(out, "preview.png"), upscale(preview))

	writeReadme(out, named)
	fmt.Printf("pletenaya-mogila: %d плиток + preview.png → %s\n", len(named), out)
}

func writeReadme(out string, named map[string]*canvas) {
	order := []struct{ name, desc string }{
		{"floor_a", "пол — шёлковое плетение (вариант A)"},
		{"floor_b", "пол — вариант B"},
		{"floor_c", "пол — вариант C"},
		{"wall", "стена-паутина (верхняя грань + высота), тайлится по горизонтали"},
		{"dais", "центральный помост (кокон ф.2), тёмная лунка + ядовитое свечение"},
		{"alcove", "периметральный альков (ниша под кокон выводка)"},
		{"egg_cluster", "кладка светящихся яиц (источник зелёного света)"},
		{"hanging_cocoon", "свисающий кокон-жертва (надземный слой), силуэт человека"},
		{"husk_a", "высохший панцирь добычи (пропа на полу)"},
		{"husk_b", "husk — вариант B"},
		{"membrane", "шёлковая мембрана входа (арена-лок)"},
	}
	var b []byte
	b = append(b, []byte("# «Плетёная Могила» — тайлсет и превью\n\n")...)
	b = append(b, []byte("Процедурно нарисованные элементы арены Ткачихи (`tools/envgen`), вид сверху ~65°,\n")...)
	b = append(b, []byte("единый свет сверху-слева — как у боссов. Собрано по слоям (`../../../tech/rendering.md`).\n\n")...)
	b = append(b, []byte("Перегенерация: `go run ./tools/envgen`. В игру (когда одобрим):\n")...)
	b = append(b, []byte("`go run ./tools/envgen assets/sprites/env/weaver_grave`.\n\n")...)
	b = append(b, []byte("## Собранное превью (все слои)\n\n![preview](preview.png)\n\n")...)
	b = append(b, []byte("Слои: фон → пол → напольный свет → стена → помост → альковы+кладки → husks →\n")...)
	b = append(b, []byte("Ткачиха (для масштаба) → мембрана входа → свисающие коконы → виньетка.\n\n")...)
	b = append(b, []byte("## Плитки и пропы\n\n| Элемент | Слой | Что это |\n|---|---|---|\n")...)
	layer := map[string]string{"floor_a": "пол", "floor_b": "пол", "floor_c": "пол", "wall": "стена",
		"dais": "объект", "alcove": "объект", "egg_cluster": "объект/свет", "hanging_cocoon": "надземное",
		"husk_a": "объект", "husk_b": "объект", "membrane": "вход"}
	for _, o := range order {
		b = append(b, []byte(fmt.Sprintf("| ![%s](tiles/%s.png) | %s | %s |\n", o.name, o.name, layer[o.name], o.desc))...)
	}
	os.MkdirAll(out, 0o755)
	os.WriteFile(filepath.Join(out, "README.md"), b, 0o644)
}
