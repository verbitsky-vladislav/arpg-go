// Инструмент bossgen — процедурный генератор спрайтов босса «Пожиратель миров»
// в проекции ВИД СВЕРХУ ~65° (референс Necesse + боди-хоррор Terraria/PoE).
// Отдельный package main на stdlib, вне игрового бинарника.
//
// Ключевой приём: свет ЕДИНЫЙ и глобальный (сверху-слева). Голова компонуется в
// локальной системе «лицом вперёд», а 8 направлений получаются переразмещением
// частей по углу поворота — при этом каждая примитив-функция затеняет себя по
// тому же глобальному свету, поэтому освещение остаётся корректным во всех
// направлениях (без зеркал: mirrored = []).
//
// Кадры раскладываются в спрайтшиты: СТРОКИ = направления, СТОЛБЦЫ = кадры.
// Рядом кладутся palette.hex и world_eater.json (полные метаданные).
//
// Запуск: go run ./tools/bossgen assets/sprites/bosses/world_eater
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

// ── Палитра: приглушённая землистая база + висцеральные акценты (<10% площади). ─

func rgb(r, g, b uint8) color.RGBA     { return color.RGBA{r, g, b, 255} }
func rgba(r, g, b, a uint8) color.RGBA { return color.RGBA{r, g, b, a} }

var (
	carapace     = []color.RGBA{rgb(0x20, 0x1b, 0x26), rgb(0x37, 0x2f, 0x3f), rgb(0x53, 0x49, 0x5b), rgb(0x77, 0x6a, 0x7d), rgb(0x9e, 0x91, 0xa2)}
	carapaceSpec = rgb(0xc4, 0xba, 0xca)
	flesh        = []color.RGBA{rgb(0x5f, 0x51, 0x4a), rgb(0x82, 0x70, 0x63), rgb(0xa2, 0x8e, 0x7d), rgb(0xc0, 0xac, 0x98), rgb(0xd9, 0xc9, 0xb2)}
	dirt         = []color.RGBA{rgb(0x24, 0x1b, 0x12), rgb(0x3a, 0x2c, 0x1d), rgb(0x52, 0x40, 0x2b), rgb(0x6d, 0x58, 0x3b), rgb(0x88, 0x72, 0x50)}
	acid         = []color.RGBA{rgb(0x22, 0x34, 0x18), rgb(0x3d, 0x66, 0x1e), rgb(0x6a, 0xa8, 0x2b), rgb(0xa2, 0xe0, 0x48), rgb(0xd8, 0xff, 0x9e)}
	eye          = []color.RGBA{rgb(0x46, 0x10, 0x14), rgb(0x86, 0x1c, 0x20), rgb(0xd0, 0x38, 0x30), rgb(0xff, 0x86, 0x58)}
	gore         = []color.RGBA{rgb(0x22, 0x0c, 0x0e), rgb(0x3d, 0x14, 0x14), rgb(0x5c, 0x1f, 0x1b), rgb(0x84, 0x30, 0x27), rgb(0xa6, 0x47, 0x39)}
	wetSpec      = rgb(0xef, 0xe6, 0xd6)

	outlineDark  = rgb(0x16, 0x12, 0x1e)
	outlineLight = rgb(0x38, 0x30, 0x40)
)

var light = normalize3(-0.45, -0.72, 0.52) // единый свет: сверху-слева-спереди

func normalize3(x, y, z float64) [3]float64 {
	l := math.Sqrt(x*x + y*y + z*z)
	return [3]float64{x / l, y / l, z / l}
}

func ramp(r []color.RGBA, t float64) color.RGBA {
	if math.IsNaN(t) || t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return r[int(t*float64(len(r)-1)+0.5)]
}

func mix(a, b color.RGBA, t float64) color.RGBA {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	l := func(x, y uint8) uint8 { return uint8(float64(x)*(1-t) + float64(y)*t) }
	return color.RGBA{l(a.R, b.R), l(a.G, b.G), l(a.B, b.B), a.A}
}

// ── Холст ────────────────────────────────────────────────────────────────────

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
func (c *canvas) over(x, y int, col color.RGBA) {
	if !c.in(x, y) || col.A == 0 {
		return
	}
	if col.A == 255 {
		c.px[y*c.w+x] = col
		return
	}
	d := c.px[y*c.w+x]
	a := float64(col.A) / 255
	m := func(s, dd uint8) uint8 { return uint8(float64(s)*a + float64(dd)*(1-a)) }
	na := uint8(float64(col.A) + float64(d.A)*(1-a))
	if na < d.A {
		na = d.A
	}
	c.px[y*c.w+x] = color.RGBA{m(col.R, d.R), m(col.G, d.G), m(col.B, d.B), na}
}
func (c *canvas) darken(x, y int, k float64) {
	if !c.in(x, y) {
		return
	}
	p := c.px[y*c.w+x]
	if p.A == 0 {
		return
	}
	f := 1 - k
	c.px[y*c.w+x] = color.RGBA{uint8(float64(p.R) * f), uint8(float64(p.G) * f), uint8(float64(p.B) * f), p.A}
}
func lighten(p color.RGBA, k float64) color.RGBA {
	if p.A == 0 {
		return p
	}
	l := func(v uint8) uint8 { return uint8(float64(v) + (255-float64(v))*k) }
	return color.RGBA{l(p.R), l(p.G), l(p.B), p.A}
}

// ── Объёмное затенение с поворотом ang и опциональным глянцем. ────────────────
func (c *canvas) dome(cx, cy, rx, ry, ang, bulge float64, r []color.RGBA, biasHi float64, spec bool) {
	ca, sa := math.Cos(ang), math.Sin(ang)
	rad := math.Max(rx, ry) + 1
	for y := int(cy - rad); y <= int(cy+rad); y++ {
		for x := int(cx - rad); x <= int(cx+rad); x++ {
			dx, dy := float64(x)-cx, float64(y)-cy
			lx := dx*ca + dy*sa
			ly := -dx*sa + dy*ca
			nx, ny := lx/rx, ly/ry
			if nx*nx+ny*ny > 1 {
				continue
			}
			nz := math.Sqrt(1-nx*nx-ny*ny) * bulge
			wnx, wny := nx*ca-ny*sa, nx*sa+ny*ca
			n := normalize3(wnx, wny, nz)
			lam := n[0]*light[0] + n[1]*light[1] + n[2]*light[2]
			col := ramp(r, 0.5+0.62*lam+biasHi)
			if spec && lam > 0.9 {
				g := (lam - 0.9) / 0.1
				if g > 1 {
					g = 1
				}
				col = mix(col, carapaceSpec, g*0.8)
			}
			c.set(x, y, col)
		}
	}
}

func (c *canvas) disc(cx, cy, r float64, col color.RGBA) {
	for y := int(cy - r - 1); y <= int(cy+r+1); y++ {
		for x := int(cx - r - 1); x <= int(cx+r+1); x++ {
			if math.Hypot(float64(x)-cx, float64(y)-cy) <= r {
				c.set(x, y, col)
			}
		}
	}
}

func glow(c *canvas, cx, cy, r float64, col color.RGBA, intensity float64) {
	if r <= 0 {
		return
	}
	for y := int(cy - r); y <= int(cy+r); y++ {
		for x := int(cx - r); x <= int(cx+r); x++ {
			d := math.Hypot(float64(x)-cx, float64(y)-cy)
			if d > r {
				continue
			}
			cc := col
			cc.A = uint8(intensity * (1 - d/r) * 255)
			c.over(x, y, cc)
		}
	}
}

// spike — острый шип/клык: треугольник от основания (2*halfw) к кончику (1px),
// объём по глобальному свету на поперечной оси, блик на кончике.
func (c *canvas) spike(bx, by, tx, ty, halfw float64, r []color.RGBA, spec bool) {
	dx, dy := tx-bx, ty-by
	L := math.Hypot(dx, dy)
	if L < 0.5 {
		return
	}
	ux, uy := dx/L, dy/L
	px, py := -uy, ux
	litSide := px*light[0] + py*light[1]
	steps := int(L) + 1
	for i := 0; i <= steps; i++ {
		f := float64(i) / float64(steps)
		w := halfw * (1 - f)
		mx, my := bx+dx*f, by+dy*f
		for j := -w; j <= w; j += 1 {
			t := 0.5 + 0.42*litSide*(j/(w+0.01)) + 0.12*(1-f)
			c.set(int(mx+px*j), int(my+py*j), ramp(r, t))
		}
	}
	if spec {
		c.set(int(tx), int(ty), carapaceSpec)
		c.set(int(tx-ux), int(ty-uy), ramp(r, 0.95))
	}
}

func prand(i float64) float64 { s := math.Sin(i*12.9898) * 43758.5453; return s - math.Floor(s) }

func vein(c *canvas, x0, y0, x1, y1 float64) {
	steps := int(math.Hypot(x1-x0, y1-y0)) + 1
	nx, ny := -(y1 - y0), (x1 - x0)
	nl := math.Hypot(nx, ny) + 0.001
	nx, ny = nx/nl, ny/nl
	for i := 0; i <= steps; i++ {
		f := float64(i) / float64(steps)
		wob := math.Sin(f*7.5) * 1.6 * (1 - f)
		cc := ramp(gore, 0.35)
		cc.A = 170
		c.over(int(x0+(x1-x0)*f+nx*wob), int(y0+(y1-y0)*f+ny*wob), cc)
	}
}

// outlined — selective outline: тёмный контур на теневой стороне (верх/лево
// сосед), светлый — со стороны света.
func outlined(src *canvas) *canvas {
	out := newCanvas(src.w, src.h)
	copy(out.px, src.px)
	for y := 0; y < src.h; y++ {
		for x := 0; x < src.w; x++ {
			if src.at(x, y).A != 0 {
				continue
			}
			left, up := src.at(x-1, y).A != 0, src.at(x, y-1).A != 0
			right, down := src.at(x+1, y).A != 0, src.at(x, y+1).A != 0
			if left || up {
				out.px[y*out.w+x] = outlineDark
			} else if right || down {
				out.px[y*out.w+x] = outlineLight
			}
		}
	}
	return out
}

func (c *canvas) bbox() (int, int, int, int) {
	minx, miny, maxx, maxy := c.w, c.h, -1, -1
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
	return minx, miny, maxx, maxy
}

// blitTransform — экранная трансформация (сдвиг/присед/альфа/тон-вспышка).
func blitTransform(dst, src *canvas, tx, ty, squash, alpha float64, tint color.RGBA, tintAmt float64) {
	gy := float64(src.h - 1)
	for y := 0; y < dst.h; y++ {
		for x := 0; x < dst.w; x++ {
			fx, fy := float64(x)-tx, float64(y)-ty
			if squash != 0 && squash != 1 {
				fy = gy - (gy-fy)/squash
			}
			col := src.at(int(math.Round(fx)), int(math.Round(fy)))
			if col.A == 0 {
				continue
			}
			if tintAmt > 0 {
				col = mix(col, tint, tintAmt)
			}
			if alpha < 1 {
				col.A = uint8(float64(col.A) * alpha)
			}
			dst.set(x, y, col)
		}
	}
}

// ── Ориентация: локальные (v=вправо, u=вперёд к пасти) → экранные координаты. ─
type ori struct {
	cx, cy, fx, fy, px, py, dphi float64
}

func newOri(cx, cy, phi float64) ori {
	fx, fy := math.Cos(phi), math.Sin(phi)
	return ori{cx, cy, fx, fy, fy, -fx, phi - math.Pi/2}
}

// at — локальные (dv=вправо, du=вперёд) → экран.
func (o ori) at(dv, du float64) (float64, float64) {
	return o.cx + du*o.fx + dv*o.px, o.cy + du*o.fy + dv*o.py
}

// ── ГОЛОВА (боди-хоррор), направление задаётся phi. Кадр 112×112. ─────────────

type headP struct {
	throat, mouth, eyePulse float64 // 0..1
}

const fwHead, fhHead = 112, 112

func renderHead(phi float64, hp headP) *canvas {
	c := newCanvas(fwHead, fhHead)
	o := newOri(56, 56, phi)
	dp := o.dphi
	at := o.at

	// 1) «Шея» позади головы: 2 кольца панциря с плотью в швах.
	for i := 0.0; i < 2; i++ {
		nx, ny := at(0, -34-i*9)
		c.dome(nx, ny, 22-i*3, 11, dp, 0.8, carapace, -0.16-0.05*i, false)
	}

	// 2) Основной бронепанцирь-купол.
	hx, hy := at(0, 0)
	c.dome(hx, hy, 34, 38, dp, 1.05, carapace, 0.0, true)

	// 3) Швы между пластинами — обнажённое мокрое мясо (перпендикулярные оси f).
	for si := 0.0; si < 3; si++ {
		du := -18 + si*16
		for dv := -34.0; dv <= 34; dv++ {
			nv := dv / 34
			if nv*nv > 1 {
				continue
			}
			x, y := at(dv, du-5*(1-nv*nv))
			c.darken(int(x), int(y), 0.5)
			c.over(int(x)+int(o.fx), int(y)+int(o.fy), ramp(flesh, 0.42+0.22*math.Sin(dv*0.5+si)))
		}
		ax, ay := at(-26, du)
		bx, by := at(24, du)
		vein(c, ax, ay, bx, by)
	}

	// 3b) Текстура: оспины.
	for _, pm := range [][2]float64{{-14, -8}, {10, -14}, {-6, 6}, {16, 2}} {
		x, y := at(pm[0], pm[1])
		c.darken(int(x), int(y), 0.3)
	}

	// 3c) АСИММЕТРИЯ: пролом брони с сырым мясом, костью и кислотой.
	wx, wy := at(-19, 6)
	for y := int(wy - 7); y <= int(wy+7); y++ {
		for x := int(wx - 8); x <= int(wx+8); x++ {
			d := math.Hypot((float64(x)-wx)/1.1, float64(y)-wy)
			if d > 6.2 {
				continue
			}
			if d > 4.4 {
				c.set(x, y, ramp(flesh, 0.45+0.35*prand(float64(x*7+y))))
			} else {
				c.set(x, y, ramp(gore, 0.12+0.5*(d/4.4)))
			}
		}
	}
	vein(c, wx-4, wy-3, wx+5, wy+4)
	c.set(int(wx), int(wy-2), ramp(flesh, 0.95))
	c.over(int(wx+2), int(wy+3), ramp(acid, 0.6))

	// 4) Острый гребень шипов + боковые ряды (кривые, неровные). Кончики назад.
	crest := [][4]float64{{0, 24, 16, 3.0}, {0, 10, 15, 2.8}, {0, -4, 13, 2.4}, {0, -16, 10, 2.0},
		{-12, 18, 12, 2.4}, {12, 18, 12, 2.4}, {-16, 3, 11, 2.2}, {16, 3, 11, 2.2}}
	for _, s := range crest {
		bx, by := at(s[0], s[1])
		jl := s[2] * (0.8 + 0.4*prand(s[0]*1.3+s[1]))
		side := s[0] * 0.35
		tx, ty := at(s[0]+side+(prand(s[1]*2.1)-0.5)*3, s[1]-jl) // кончик назад/вбок
		for yy := int(by - 2); yy <= int(by+3); yy++ {
			for xx := int(bx - s[3]*1.7); xx <= int(bx+s[3]*1.7); xx++ {
				if math.Hypot(float64(xx)-bx, float64(yy)-by) < s[3]*1.5 {
					c.darken(xx, yy, 0.26)
				}
			}
		}
		c.spike(bx, by, tx, ty, s[3], carapace, true)
	}

	// 4b) Серрации по кромке (кроме зон пасти впереди и шеи сзади).
	for i := 0; i < 16; i++ {
		a := 2 * math.Pi * float64(i) / 16
		du, dv := math.Cos(a)*38, math.Sin(a)*34 // локально: у=cos*.. по f
		if du > 22 || du < -26 {                 // пропуск пасти(перед) и шеи(зад)
			continue
		}
		bx, by := at(dv, du)
		tx, ty := at(dv*1.18, du*1.18)
		c.spike(bx, by, tx, ty, 2.1, carapace, true)
	}

	// 5) Пасть впереди (по f), раскрытие mouth, свечение горла throat.
	drawMaw(c, o, 24, 22, hp.throat, hp.mouth)

	// 6) ГИГАНТСКИЙ ГЛАЗ + мелкие глаза вокруг.
	drawEye(c, o, 0, -8, 12, hp.eyePulse)
	small := [][2]float64{{-27, 8}, {27, 8}, {-31, -3}, {31, -3}, {-5, 19}, {7, 19}}
	for i, e := range small {
		x, y := at(e[0], e[1])
		c.set(int(x), int(y), eye[2])
		if prand(float64(i)*5) > 0.4 {
			c.set(int(x), int(y), eye[3])
		}
		glow(c, x, y, 2.3, eye[2], 0.5+0.4*hp.eyePulse)
	}

	return c
}

// drawMaw — пасть-мясорубка впереди головы (по f), с раскрытием mouth.
func drawMaw(c *canvas, o ori, du, R, throat, mouth float64) {
	cx, cy := o.at(0, du)
	R *= 1 + 0.25*mouth // раскрытие
	dp := o.dphi
	rMaw := func(a float64) float64 { return R * (0.42 + 0.58*math.Pow(math.Abs(math.Cos(2*(a-dp))), 0.7)) }
	for y := int(cy - R - 2); y <= int(cy+R+2); y++ {
		for x := int(cx - R - 2); x <= int(cx+R+2); x++ {
			dx, dy := float64(x)-cx, float64(y)-cy
			d := math.Hypot(dx, dy)
			if d > rMaw(math.Atan2(dy, dx))+1.2 {
				continue
			}
			c.set(x, y, ramp(carapace, 0.02+0.2*(d/(R+0.1))))
		}
	}
	// внешние кривые клыки внутрь
	for i := 0; i < 16; i++ {
		a := 2*math.Pi*float64(i)/16 + dp + (prand(float64(i))-0.5)*0.2
		rm := rMaw(a)
		lf := 0.30 + 0.12*prand(float64(i)*2.3)
		if prand(float64(i)*5.1) > 0.82 {
			lf = 0.72
		}
		c.spike(cx+math.Cos(a)*rm*0.98, cy+math.Sin(a)*rm*0.98,
			cx+math.Cos(a)*rm*lf, cy+math.Sin(a)*rm*lf, 1.3+0.8*prand(float64(i)*1.7), flesh, false)
	}
	// внутреннее кольцо
	for i := 0; i < 10; i++ {
		a := 2*math.Pi*(float64(i)+0.5)/10 + dp
		rm := rMaw(a)
		c.spike(cx+math.Cos(a)*rm*0.66, cy+math.Sin(a)*rm*0.66,
			cx+math.Cos(a)*rm*0.15, cy+math.Sin(a)*rm*0.15, 1.1+0.5*prand(float64(i)), flesh, false)
	}
	// кровь у корней
	for a := 0.0; a < 2*math.Pi; a += 0.22 {
		rm := rMaw(a)
		cc := ramp(gore, 0.4)
		cc.A = 150
		c.over(int(cx+math.Cos(a)*rm*0.86), int(cy+math.Sin(a)*rm*0.86), cc)
	}
	// глотка: тьма + свечение изнутри
	for y := int(cy - R*0.32); y <= int(cy+R*0.32); y++ {
		for x := int(cx - R*0.32); x <= int(cx+R*0.32); x++ {
			d := math.Hypot(float64(x)-cx, float64(y)-cy)
			if d > R*0.3 {
				continue
			}
			c.set(x, y, carapace[0])
			if throat > 0.02 {
				gl := ramp(acid, 0.45+0.5*throat)
				gl.A = uint8((0.4 + 0.55*throat) * (1 - d/(R*0.3)) * 255)
				c.over(x, y, gl)
			}
		}
	}
	// внешние хитиновые крюки в вершинах лепестков
	for k := 0.0; k < 4; k++ {
		a := dp + k*math.Pi/2
		rm := rMaw(a)
		c.spike(cx+math.Cos(a)*rm*0.9, cy+math.Sin(a)*rm*0.9,
			cx+math.Cos(a)*(rm+7)-math.Sin(a)*3, cy+math.Sin(a)*(rm+7)+math.Cos(a)*3, 2.3, carapace, true)
	}
	// AO-кольцо
	for a := 0.0; a < 2*math.Pi; a += 0.05 {
		rm := rMaw(a)
		c.darken(int(cx+math.Cos(a)*(rm+1)), int(cy+math.Sin(a)*(rm+1)), 0.3)
	}
	// кислотная слюна вперёд (по f)
	if throat > 0.02 {
		sx, sy := cx+o.fx*R*0.9, cy+o.fy*R*0.9
		for k := 0; k < 4+int(4*throat); k++ {
			gl := ramp(acid, 0.6)
			gl.A = uint8((1 - float64(k)/8) * 200)
			c.over(int(sx+o.fx*float64(k)), int(sy+o.fy*float64(k)), gl)
		}
	}
}

// drawEye — гигантский немигающий глаз; бровь со стороны «зад» (−f),
// мокрый блик — фиксирован в экранном верх-лево (глобальный свет).
func drawEye(c *canvas, o ori, dv, du, R, pulse float64) {
	ex, ey := o.at(dv, du)
	for y := int(ey - R - 3); y <= int(ey+R+3); y++ { // AO-сокет
		for x := int(ex - R - 3); x <= int(ex+R+3); x++ {
			d := math.Hypot(float64(x)-ex, float64(y)-ey)
			if d > R && d < R+3 {
				c.darken(x, y, 0.4*(1-(d-R)/3))
			}
		}
	}
	c.dome(ex, ey, R, R, 0, 1.15, flesh, 0.05, false)
	for i := 0; i < 8; i++ {
		a := 2 * math.Pi * prand(float64(i)*3.1)
		vein(c, ex+math.Cos(a)*R*0.98, ey+math.Sin(a)*R*0.98, ex+math.Cos(a)*R*0.45, ey+math.Sin(a)*R*0.45)
	}
	ir := R * 0.62
	for y := int(ey - ir); y <= int(ey+ir); y++ {
		for x := int(ex - ir); x <= int(ex+ir); x++ {
			d := math.Hypot(float64(x)-ex, float64(y)-ey)
			if d > ir {
				continue
			}
			t := d / ir
			c.set(x, y, ramp(gore, 0.12+0.5*t))
			if t > 0.8 {
				c.set(x, y, ramp(acid, 0.5))
			}
		}
	}
	pr := ir * (0.55 - 0.15*pulse) // зрачок сужается при возбуждении
	c.disc(ex, ey, pr, carapace[0])
	glow(c, ex, ey, ir*1.3, eye[2], 0.4+0.3*pulse)
	c.set(int(ex-R*0.34), int(ey-R*0.4), wetSpec)
	c.set(int(ex-R*0.28), int(ey-R*0.4), wetSpec)
	c.set(int(ex-R*0.3), int(ey-R*0.32), ramp(flesh, 0.92))
	// бровь: армированный навес со стороны «зад» (−f)
	ba := math.Atan2(-o.fy, -o.fx)
	for a := ba - 1.3; a < ba+1.3; a += 0.03 {
		x, y := ex+math.Cos(a)*R, ey+math.Sin(a)*R
		c.set(int(x), int(y), ramp(carapace, 0.6))
		c.darken(int(x-o.fx), int(y-o.fy), 0.35)
	}
}

// ── СЕГМЕНТ ТЕЛА (варианты a/b/c). Кадр 80×80. ────────────────────────────────

type bodyP struct {
	variant  int
	cracked  bool
	glow     float64
	submerge float64 // 0..1 доля ухода под грунт
}

const fwBody, fhBody = 80, 80

func renderSegment(phi float64, bp bodyP) *canvas {
	c := newCanvas(fwBody, fhBody)
	o := newOri(40, 40, phi)
	dp := o.dphi
	at := o.at

	// плоть-подложка + бронепластина
	bxx, byy := at(0, 0)
	c.dome(bxx, byy, 17, 18, dp, 0.9, flesh, 0.0, false)
	switch bp.variant {
	case 0:
		c.dome(bxx, byy, 14, 17, dp, 1.05, carapace, 0, true)
	case 1:
		c.dome(bxx, byy, 12, 18, dp, 1.05, carapace, 0, true)
		rx, ry := at(0, -5)
		c.dome(rx, ry, 7, 5, dp, 1.2, carapace, 0.12, true) // гребень
	case 2:
		lx, ly := at(-5, 0)
		rx, ry := at(5, 0)
		c.dome(lx, ly, 8, 17, dp, 1.05, carapace, 0, true)
		c.dome(rx, ry, 8, 17, dp, 1.05, carapace, 0, true)
	}
	// швы плоти спереди/сзади
	for _, du := range []float64{-14, 14} {
		ax, ay := at(-15, du)
		bx2, by2 := at(15, du)
		for t := 0.0; t <= 1; t += 0.05 {
			c.over(int(ax+(bx2-ax)*t), int(ay+(by2-ay)*t), ramp(flesh, 0.4))
		}
	}
	// спинные шипы (по гребню, кончик назад)
	spk := map[int][]float64{0: {-6, 6}, 1: {-8, 0, 8}, 2: {-7, 7}}[bp.variant]
	for _, dv := range spk {
		bx, by := at(dv, 0)
		tx, ty := at(dv, -9)
		c.spike(bx, by, tx, ty, 2.3, carapace, true)
	}
	if bp.cracked {
		x0, y0 := at(-8, -8)
		x1, y1 := at(-5, 6)
		x2, y2 := at(-9, 12)
		c.spike(x0, y0, x1, y1, 0, gore, false)
		vein(c, x0, y0, x2, y2)
		glow(c, bxx, byy, 8, acid[3], 0.35*bp.glow)
	}

	// частичное погружение: грунтовый «воротник» съедает кромку, наверху — шипы.
	if bp.submerge > 0.02 {
		earthR := 18 * (1 - 0.15*bp.submerge)
		for y := 0; y < c.h; y++ {
			for x := 0; x < c.w; x++ {
				d := math.Hypot(float64(x)-bxx, float64(y)-byy)
				keep := earthR * (1 - bp.submerge*0.9)
				if d > keep && c.at(x, y).A != 0 && bp.variant != 9 {
					// заменяем кромку сегмента взрытой землёй
					if d < earthR+3 {
						c.set(x, y, ramp(dirt, 0.3+0.4*prand(float64(x*3+y))))
					}
				}
			}
		}
		drawEarthRing(c, bxx, byy, earthR+2, bp.submerge)
	}
	return c
}

// drawEarthRing — кольцо взрытого грунта вокруг точки входа + контактная тень.
func drawEarthRing(c *canvas, cx, cy, r, intensity float64) {
	for a := 0.0; a < 2*math.Pi; a += 0.14 {
		rr := r + prand(a)*3
		x, y := cx+math.Cos(a)*rr, cy+math.Sin(a)*rr
		c.over(int(x), int(y), ramp(dirt, 0.2+0.5*prand(a*2)))
		c.darken(int(cx+math.Cos(a)*(r-2)), int(cy+math.Sin(a)*(r-2)), 0.4*intensity) // AO входа
	}
}

// ── ХВОСТ: сегмент + костяное жало-булава на конце (−f). Кадр 80×80. ──────────

type tailP struct {
	swing   float64 // угол замаха жала (рад)
	cracked bool
}

func renderTail(phi float64, tp tailP) *canvas {
	c := newCanvas(fwBody, fhBody)
	o := newOri(44, 40, phi)
	dp := o.dphi
	bxx, byy := o.at(0, 4)
	c.dome(bxx, byy, 12, 15, dp, 1.0, flesh, 0, false)
	c.dome(bxx, byy, 9, 15, dp, 1.05, carapace, 0, true)
	sbx, sby := o.at(0, 0)
	stx, sty := o.at(0, -8)
	c.spike(sbx, sby, stx, sty, 2.2, carapace, true)

	// костяная булава на −f, отклонённая на swing
	ba := math.Atan2(-o.fy, -o.fx) + tp.swing
	mx, my := bxx+math.Cos(ba)*20, byy+math.Sin(ba)*20
	c.dome(mx, my, 7, 7, 0, 1.15, flesh, 0.05, false) // кость
	c.disc(mx, my, 3, ramp(flesh, 0.5))
	for _, da := range []float64{-2.4, -1.6, -0.8, 0, 0.8, 1.6, 2.4, 3.14} {
		c.spike(mx, my, mx+math.Cos(ba+da)*10, my+math.Sin(ba+da)*10, 1.6, flesh, true)
	}
	c.set(int(mx), int(my), ramp(flesh, 0.95))
	if tp.cracked {
		glow(c, mx, my, 9, acid[3], 0.4)
	}
	return c
}

// ── FX ───────────────────────────────────────────────────────────────────────

func fxAcidProjectile(k, n int) *canvas { // 20×20 loop
	c := newCanvas(20, 20)
	ph := 2 * math.Pi * float64(k) / float64(n)
	r := 6 + 0.8*math.Sin(ph)
	glow(c, 10, 10, r+3, acid[3], 0.9)
	c.dome(10, 10, r, r, 0, 1.1, acid, 0.1, false)
	c.set(8, 8, acid[4])
	c.over(int(10+math.Cos(ph)*3), int(12+math.Sin(ph)*2), ramp(acid, 0.2))
	return c
}

func fxAcidImpact(k, n int) *canvas { // 20×20 once
	c := newCanvas(20, 20)
	t := float64(k) / float64(n-1)
	r := 2.0 + 7*t
	for i := 0; i < 8; i++ {
		a := 2 * math.Pi * float64(i) / 8
		cc := ramp(acid, 0.7)
		cc.A = uint8((1 - t) * 255)
		c.over(int(10+math.Cos(a)*r), int(10+math.Sin(a)*r), cc)
	}
	if k < 2 {
		glow(c, 10, 10, 6, acid[3], 1-t)
	}
	return c
}

func fxPool(stage string, k, n int) *canvas { // 80×40
	c := newCanvas(80, 40)
	cx, cy := 40.0, 22.0
	var rx, ry, alpha float64
	switch stage {
	case "spawn":
		t := float64(k) / float64(n-1)
		rx, ry, alpha = 8+28*t, 4+13*t, t
	case "loop":
		ph := 2 * math.Pi * float64(k) / float64(n)
		rx, ry, alpha = 36+1.5*math.Sin(ph), 17, 1
	case "fade":
		t := 1 - float64(k)/float64(n-1)
		rx, ry, alpha = 8+28*t, 4+13*t, t
	}
	for y := 0; y < c.h; y++ {
		for x := 0; x < c.w; x++ {
			dx, dy := (float64(x)-cx)/rx, (float64(y)-cy)/ry
			e := dx*dx + dy*dy
			if e <= 1 {
				col := ramp(acid, 0.3+0.55*e)
				col.A = uint8(alpha * 255)
				c.over(x, y, col)
			}
		}
	}
	if stage == "loop" {
		for i := 0; i < 6; i++ {
			c.set(12+i*11, 22-(k+i)%6, acid[4])
		}
	}
	return c
}

func fxDirtBurstRing(k, n int) *canvas { // 112×112 — кольцевой выброс грунта
	c := newCanvas(112, 112)
	t := float64(k) / float64(n-1)
	for i := 0; i < 20; i++ {
		a := 2 * math.Pi * float64(i) / 20
		dist := 46 * t
		x := 56 + math.Cos(a)*dist
		y := 56 + math.Sin(a)*dist*0.55 - 10*math.Sin(math.Pi*t) // подлёт
		r := 3 - 2.5*t
		if r < 0.6 {
			r = 0.6
		}
		c.dome(x, y, r, r, 0, 1, dirt, 0.1, false)
	}
	return c
}

func fxGroundCrack(k, n int) *canvas { // 80×80 — телеграф точки пробоя (вид сверху)
	c := newCanvas(80, 80)
	t := float64(k) / float64(n-1)
	rays := 7
	for i := 0; i < rays; i++ {
		a := 2*math.Pi*float64(i)/float64(rays) + prand(float64(i))
		ln := 34 * t
		for s := 0.0; s < ln; s += 1 {
			x, y := 40+math.Cos(a)*s, 40+math.Sin(a)*s*0.6
			c.set(int(x), int(y), dirt[0])
			if prand(s+float64(i)) > 0.6 {
				c.set(int(x), int(y+1), dirt[1])
			}
		}
	}
	pulse := 0.4 + 0.6*math.Abs(math.Sin(math.Pi*t))
	glow(c, 40, 40, 10*t, eye[2], 0.5*pulse) // зловещее свечение
	return c
}

func fxDustTrail(k, n int) *canvas { // 48×48 loop
	c := newCanvas(48, 48)
	t := float64(k) / float64(n)
	for i := 0; i < 6; i++ {
		x := 8 + float64(i)*6
		y := 30 - math.Mod(t+float64(i)*0.16, 1)*10
		cc := ramp(dirt, 0.75)
		cc.A = uint8((1 - math.Mod(t+float64(i)*0.16, 1)) * 160)
		glow(c, x, y, 4, cc, 1)
	}
	return c
}

func fxMound(k, n int) *canvas { // 112×112 — «холм»: бугор земли с трещинами, loop
	c := newCanvas(112, 112)
	ph := 2 * math.Pi * float64(k) / float64(n)
	c.dome(56, 60, 40, 22, 0, 0.7, dirt, 0.0, false)
	for i := 0; i < 5; i++ {
		a := 2*math.Pi*float64(i)/5 + 0.3
		ln := 22 + 4*math.Sin(ph+float64(i))
		for s := 0.0; s < ln; s += 1 {
			c.set(int(56+math.Cos(a)*s), int(60+math.Sin(a)*s*0.55), dirt[0])
		}
	}
	// осыпающаяся пыль по гребню
	for i := 0; i < 4; i++ {
		x := 40 + i*10
		y := 44 - int(3*math.Sin(ph+float64(i)))
		cc := ramp(dirt, 0.8)
		cc.A = 150
		c.over(x, y, cc)
	}
	return c
}

// ── ТЕНИ (отдельные спрайты). ─────────────────────────────────────────────────

func renderShadow(w, h int, rise float64) *canvas {
	c := newCanvas(w, h)
	cx, cy := float64(w)/2, float64(h)/2
	rx := (float64(w)/2 - 2) * (1 - 0.4*rise)
	ry := (float64(h)/2 - 2) * (1 - 0.4*rise)
	amax := 0.55 * (1 - 0.65*rise)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			d := math.Sqrt(math.Pow((float64(x)-cx)/rx, 2) + math.Pow((float64(y)-cy)/ry, 2))
			if d > 1 {
				continue
			}
			c.set(x, y, rgba(0x0a, 0x08, 0x10, uint8(amax*math.Pow(1-d, 0.8)*255)))
		}
	}
	return c
}

// ── СБОРКА ЦЕЛЬНОГО ЧЕРВЯ + порпойз (выход из земли телом и уход обратно). ─────
//
// Показывает то, чего не видно на отдельных тайлах: голова + цепочка сегментов +
// хвост, и способность ЗАХОДИТЬ ПОД ЗЕМЛЮ — по телу бежит «горб» всплытия
// (surfacing wave): часть тела дугой над землёй (спрайты + тень), остальное под
// землёй (бугор грунта + пыль). За цикл: голова выныривает → горб едет по телу →
// хвост уходит вниз. Части рисуются под phi=0 (движение вправо).

// blitOver — переносит src в dst с масштабом и центром (cx,cy), nearest + alpha.
func blitOver(dst, src *canvas, cx, cy, scale float64) {
	sw, sh := float64(src.w)*scale, float64(src.h)*scale
	x0, y0 := int(cx-sw/2), int(cy-sh/2)
	for y := 0; y < int(sh); y++ {
		for x := 0; x < int(sw); x++ {
			dst.over(x0+x, y0+y, src.at(int(float64(x)/scale), int(float64(y)/scale)))
		}
	}
}

func smallMound(c *canvas, cx, cy float64) {
	c.dome(cx, cy, 16, 8, 0, 0.55, dirt, 0.0, false)
	for i := 0; i < 3; i++ {
		a := 2*math.Pi*float64(i)/3 + 0.4
		for s := 0.0; s < 10; s++ {
			c.set(int(cx+math.Cos(a)*s), int(cy+math.Sin(a)*s*0.6), dirt[0])
		}
	}
}

var wormShHead, wormShSeg *canvas

// wormPart — кэш спрайтов частей под квантованный угол phi (32 шага). Каждый
// рендерится ЗАНОВО под своим углом (свет остаётся глобальным — крутить готовый
// спрайт нельзя). kind: 0 голова, 1..3 сегмент(вариант), 4 хвост.
var wormPartCache = map[int]*canvas{}

func wormPart(kind int, phi float64) *canvas {
	qi := ((int(math.Round(phi/(2*math.Pi)*32)) % 32) + 32) % 32
	key := kind*100 + qi
	if c, ok := wormPartCache[key]; ok {
		return c
	}
	a := float64(qi) * 2 * math.Pi / 32
	var c *canvas
	switch kind {
	case 0:
		c = outlined(renderHead(a, headP{throat: 0.15, mouth: 0.15, eyePulse: 0.4}))
	case 4:
		c = outlined(renderTail(a, tailP{}))
	default:
		c = outlined(renderSegment(a, bodyP{variant: kind - 1}))
	}
	wormPartCache[key] = c
	return c
}

const wormW, wormH = 460, 400

// renderWorm — цельный червь ПОЛЗЁТ по сплюснутому овалу (вид сверху: все 8
// направлений за круг), голова — «лидер», сегменты follow-the-leader. По телу
// бежит подземный ПРОВАЛ: часть тела в тоннеле (бугры грунта), часть на
// поверхности (спрайты под углом касательной + тень). Демонстрирует и движение
// в 2D-пространстве, и заход под землю.
func renderWorm(t float64) *canvas {
	if wormShHead == nil {
		wormShHead = renderShadow(96, 40, 0)
		wormShSeg = renderShadow(64, 28, 0)
	}
	c := newCanvas(wormW, wormH)
	const nSeg = 16
	const cx, cy, R, Rf, dA = 230.0, 196.0, 150.0, 104.0, 0.30

	type part struct {
		x, y, phi, lift float64
		surf            bool
		kind, idx       int
	}
	total := nSeg + 2
	parts := make([]part, total)
	aHead := 2 * math.Pi * t           // голова обходит овал за цикл (бесшовный луп)
	diveC := math.Mod(t*1.0+0.15, 1.0) // подземный провал едет вдоль тела
	for i := 0; i < total; i++ {
		a := aHead - float64(i)*dA
		x := cx + R*math.Cos(a)
		y := cy + Rf*math.Sin(a)
		// касательная к овалу (направление движения) → угол спрайта
		phi := math.Atan2(Rf*math.Cos(a), -R*math.Sin(a))
		u := float64(i) / float64(total-1)
		dive := math.Exp(-math.Pow((u-diveC)*4.2, 2))
		kind := 1 + (i-1)%3
		if i == 0 {
			kind = 0
		} else if i == total-1 {
			kind = 4
		}
		parts[i] = part{x, y, phi, 0.5 * (1 - dive), dive < 0.4, kind, i}
	}

	// 1) под землёй — бугры грунта (тоннель); на границе — кольцо взрытой земли.
	for _, p := range parts {
		if !p.surf {
			smallMound(c, p.x, p.y)
		} else if p.lift < 0.35 {
			drawEarthRing(c, p.x, p.y, 16, 0.6)
		}
	}
	// 2) тени всплывших частей на полу.
	for _, p := range parts {
		if !p.surf {
			continue
		}
		sh := wormShSeg
		if p.kind == 0 {
			sh = wormShHead
		}
		blitOver(c, sh, p.x, p.y+8, 1-0.35*p.lift)
	}
	// 3) части — порядок по экранному Y (дальние/верхние раньше → верный top-down
	//    оверлап). Каждая рисуется под своим углом.
	order := make([]int, total)
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(a, b int) bool { return parts[order[a]].y < parts[order[b]].y })
	for _, i := range order {
		p := parts[i]
		if !p.surf {
			continue
		}
		sc := 0.78 * (1 + 0.08*p.lift)
		if p.kind == 0 {
			sc = 0.76 * (1 + 0.08*p.lift)
		}
		blitOver(c, wormPart(p.kind, p.phi), p.x, p.y-p.lift*12, sc)
	}
	return c
}

// ── Направления (8, честно; без зеркал). ─────────────────────────────────────

type dirDef struct {
	name string
	phi  float64
}

var dirs8 = []dirDef{
	{"down", math.Pi / 2}, {"down_left", 3 * math.Pi / 4}, {"left", math.Pi}, {"up_left", 5 * math.Pi / 4},
	{"up", 3 * math.Pi / 2}, {"up_right", 7 * math.Pi / 4}, {"right", 0}, {"down_right", math.Pi / 4},
}
var dirDown = []dirDef{{"down", math.Pi / 2}}

// ── Метаданные и драйвер. ─────────────────────────────────────────────────────

type animMeta struct {
	Name           string   `json:"name"`
	Part           string   `json:"part"`
	File           string   `json:"file"`
	Directions     int      `json:"directions"`
	DirectionNames []string `json:"direction_names"`
	Mirrored       []string `json:"mirrored"`
	Frames         int      `json:"frames"`
	FrameW         int      `json:"frame_w"`
	FrameH         int      `json:"frame_h"`
	FPS            int      `json:"fps"`
	Loop           bool     `json:"loop"`
	Pivot          [2]int   `json:"pivot"`
	Damaging       []int    `json:"damaging_frames"`
	Hitbox         [][4]int `json:"hitbox"`
	HeightOff      []int    `json:"height_offset"`
	ShadowSprite   string   `json:"shadow_sprite,omitempty"`
	ShadowOffset   [2]int   `json:"shadow_offset,omitempty"`
	AttachFront    *[2]int  `json:"attach_front,omitempty"`
	AttachBack     *[2]int  `json:"attach_back,omitempty"`
}

type spec struct {
	name, part, dir  string
	dirs             []dirDef
	nframes, fps     int
	loop             bool
	fw, fh           int
	damaging         []int
	frame            func(phi float64, k, n int) *canvas
	height           func(k, n int) int
	shadow           string
	shadowOff        [2]int
	attachF, attachB *[2]int
}

// softFX — эффекты/тени, которые не обводим селективным контуром.
var softFX = map[string]bool{
	"pool_spawn": true, "pool_loop": true, "pool_fade": true,
	"dust_trail": true, "ground_crack_warning": true, "acid_impact": true,
}

func prog(k, n int) float64 {
	if n <= 1 {
		return 0
	}
	return float64(k) / float64(n-1)
}
func loopPh(k, n int) float64 { return 2 * math.Pi * float64(k) / float64(n) }

// headAnim — кадр головы: база renderHead(hp) + экранная трансформация.
func headFrame(phi float64, hp headP, ty, squash, alpha, hurt float64) *canvas {
	base := renderHead(phi, hp)
	out := newCanvas(fwHead, fhHead)
	blitTransform(out, base, 0, ty, squash, alpha, rgb(255, 240, 240), hurt)
	return out
}

func specs() []spec {
	var S []spec
	H := func(name string, ds []dirDef, n, fps int, loop bool, dmg []int, shadow string, ho func(k, n int) int, f func(phi float64, k, n int) *canvas) {
		S = append(S, spec{name: name, part: "head", dirs: ds, nframes: n, fps: fps, loop: loop, fw: fwHead, fh: fhHead, damaging: dmg, frame: f, height: ho, shadow: shadow})
	}
	// idle
	H("head_idle", dirs8, 6, 8, true, nil, "shadow_head", nil, func(phi float64, k, n int) *canvas {
		ph := loopPh(k, n)
		return headFrame(phi, headP{throat: 0.15 + 0.1*(1-math.Cos(ph))/2, eyePulse: 0.15}, 0.8*math.Sin(ph), 1, 1, 0)
	})
	// burrowed mound — 1 направление
	S = append(S, spec{name: "head_burrowed_mound", part: "fx", dirs: dirDown, nframes: 6, fps: 10, loop: true, fw: 112, fh: 112, frame: func(_ float64, k, n int) *canvas { return fxMound(k, n) }})
	// breach — выныривание вверх, тень отдельно (rise)
	H("head_breach", dirs8, 10, 16, false, []int{4, 5}, "shadow_head_rise",
		func(k, n int) int { return int(38 * prog(k, n)) },
		func(phi float64, k, n int) *canvas {
			t := prog(k, n)
			return headFrame(phi, headP{throat: 0.1, mouth: 0.3 * t, eyePulse: t}, 38*(1-t), 0.8+0.2*t, 1, 0)
		})
	H("head_submerge", dirs8, 8, 14, false, nil, "shadow_head_rise", func(k, n int) int { return int(38 * (1 - prog(k, n))) },
		func(phi float64, k, n int) *canvas {
			t := prog(k, n)
			return headFrame(phi, headP{throat: 0.1, eyePulse: 1 - t}, 38*t, 1-0.2*t, 1, 0)
		})
	H("head_bite_windup", dirs8, 6, 10, false, nil, "shadow_head", nil, func(phi float64, k, n int) *canvas {
		t := prog(k, n)
		return headFrame(phi, headP{throat: 0.15, mouth: t, eyePulse: 0.5 + 0.5*t}, 0, 1, 1, 0)
	})
	H("head_bite_strike", dirs8, 4, 18, false, []int{0, 1}, "shadow_head", nil, func(phi float64, k, n int) *canvas {
		t := prog(k, n)
		o := newOri(0, 0, phi)
		return headFrame(phi, headP{throat: 0.1, mouth: 1 - 0.7*t, eyePulse: 1}, o.fy*10*t, 1, 1, 0)
	})
	H("head_bite_recover", dirs8, 5, 10, false, nil, "shadow_head", nil, func(phi float64, k, n int) *canvas {
		t := prog(k, n)
		return headFrame(phi, headP{throat: 0.1, mouth: 0.3 * (1 - t), eyePulse: 1 - 0.5*t}, 0, 1, 1, 0)
	})
	H("head_spit_windup", dirs8, 6, 10, false, nil, "shadow_head", nil, func(phi float64, k, n int) *canvas {
		t := prog(k, n)
		return headFrame(phi, headP{throat: t, mouth: 0.3, eyePulse: 0.6 + 0.4*t}, 0, 1, 1, 0)
	})
	// radial spit — 1 направление
	H("head_spit_radial", dirDown, 6, 16, false, []int{1}, "shadow_head", nil, func(phi float64, k, n int) *canvas {
		t := prog(k, n)
		return headFrame(phi, headP{throat: 1 - 0.7*t, mouth: 0.6, eyePulse: 1}, 0, 1, 1, 0)
	})
	H("head_spit_pool", dirs8, 6, 12, false, []int{2, 3}, "shadow_head", nil, func(phi float64, k, n int) *canvas {
		t := prog(k, n)
		return headFrame(phi, headP{throat: 0.6, mouth: 0.4, eyePulse: 0.7}, 0, 1-0.05*t, 1, 0)
	})
	// roar — 1 направление, фаза 2
	H("head_roar", dirDown, 8, 10, false, nil, "shadow_head", nil, func(phi float64, k, n int) *canvas {
		t := prog(k, n)
		return headFrame(phi, headP{throat: 0.5 + 0.5*math.Abs(math.Sin(3*math.Pi*t)), mouth: 1, eyePulse: 1}, -1.5*math.Sin(math.Pi*t), 1, 1, 0)
	})
	H("head_hurt", dirs8, 2, 20, false, nil, "shadow_head", nil, func(phi float64, k, n int) *canvas {
		hurt := 1.0
		if k == 1 {
			hurt = 0.4
		}
		return headFrame(phi, headP{throat: 0.15, mouth: 0.2, eyePulse: 0.6}, 0, 1, 1, hurt)
	})
	H("head_death", dirs8, 14, 10, false, nil, "shadow_head", nil, func(phi float64, k, n int) *canvas {
		t := prog(k, n)
		return headFrame(phi, headP{throat: 0.4 * (1 - t), mouth: 0.6 - 0.5*t, eyePulse: 0.6 * (1 - t)}, 4*t, 1-0.5*t, 1-0.7*t, 0)
	})

	// ── Тело ──
	B := func(name string, ds []dirDef, n, fps int, loop bool, f func(phi float64, k, n int) *canvas) {
		dmg := make([]int, n)
		for i := range dmg {
			dmg[i] = i // сегменты на поверхности всегда контактны
		}
		S = append(S, spec{name: name, part: "body", dirs: ds, nframes: n, fps: fps, loop: loop, fw: fwBody, fh: fhBody, damaging: dmg,
			frame: f, shadow: "shadow_segment", attachF: &[2]int{0, 0}, attachB: &[2]int{0, 0}})
	}
	for v, id := range []string{"a", "b", "c"} {
		vv := v
		B("body_"+id+"_surface_loop", dirs8, 6, 12, true, func(phi float64, k, n int) *canvas {
			ph := loopPh(k, n)
			c := renderSegment(phi, bodyP{variant: vv})
			out := newCanvas(fwBody, fhBody)
			blitTransform(out, c, 0, 0.6*math.Sin(ph), 1, 1, color.RGBA{}, 0)
			return out
		})
		for _, pc := range []struct {
			suf string
			amt float64
		}{{"25", 0.25}, {"50", 0.5}, {"75", 0.75}} {
			amt := pc.amt
			B("body_"+id+"_partial_"+pc.suf, dirs8, 1, 1, false, func(phi float64, k, n int) *canvas {
				return renderSegment(phi, bodyP{variant: vv, submerge: amt})
			})
		}
	}

	// ── Хвост ──
	T := func(name string, ds []dirDef, n, fps int, loop bool, dmg []int, f func(phi float64, k, n int) *canvas) {
		S = append(S, spec{name: name, part: "tail", dirs: ds, nframes: n, fps: fps, loop: loop, fw: fwBody, fh: fhBody, damaging: dmg, frame: f, shadow: "shadow_segment", attachF: &[2]int{0, 0}})
	}
	T("tail_idle", dirs8, 4, 8, true, nil, func(phi float64, k, n int) *canvas {
		return renderTail(phi, tailP{swing: 0.15 * math.Sin(loopPh(k, n))})
	})
	T("tail_sweep_windup", dirs8, 6, 10, false, nil, func(phi float64, k, n int) *canvas {
		return renderTail(phi, tailP{swing: -1.4 * prog(k, n)})
	})
	T("tail_sweep_strike", dirs8, 6, 18, false, []int{1, 2, 3}, func(phi float64, k, n int) *canvas {
		return renderTail(phi, tailP{swing: -1.4 + 3.1*prog(k, n)}) // дуга ~180°
	})
	T("tail_submerge", dirs8, 6, 14, false, nil, func(phi float64, k, n int) *canvas {
		t := prog(k, n)
		out := newCanvas(fwBody, fhBody)
		blitTransform(out, renderTail(phi, tailP{}), 0, 30*t, 1-0.3*t, 1-0.6*t, color.RGBA{}, 0)
		return out
	})

	// ── FX ──
	F := func(name string, nw, nh, n, fps int, loop bool, dmg []int, f func(k, n int) *canvas) {
		S = append(S, spec{name: name, part: "fx", dirs: dirDown, nframes: n, fps: fps, loop: loop, fw: nw, fh: nh, damaging: dmg,
			frame: func(_ float64, k, n int) *canvas { return f(k, n) }})
	}
	F("acid_projectile", 20, 20, 4, 12, true, []int{0, 1, 2, 3}, fxAcidProjectile)
	F("acid_impact", 20, 20, 6, 16, false, nil, fxAcidImpact)
	F("pool_spawn", 80, 40, 6, 12, false, nil, func(k, n int) *canvas { return fxPool("spawn", k, n) })
	F("pool_loop", 80, 40, 8, 8, true, []int{0, 1, 2, 3, 4, 5, 6, 7}, func(k, n int) *canvas { return fxPool("loop", k, n) })
	F("pool_fade", 80, 40, 6, 10, false, nil, func(k, n int) *canvas { return fxPool("fade", k, n) })
	F("dirt_burst_ring", 112, 112, 8, 16, false, nil, fxDirtBurstRing)
	F("ground_crack_warning", 80, 80, 6, 10, false, nil, fxGroundCrack)
	F("dust_trail", 48, 48, 6, 12, true, nil, fxDustTrail)

	// ── Тени ──
	F2 := func(name string, nw, nh, n, fps int, loop bool, f func(k, n int) *canvas) {
		S = append(S, spec{name: name, part: "shadows", dirs: dirDown, nframes: n, fps: fps, loop: loop, fw: nw, fh: nh,
			frame: func(_ float64, k, n int) *canvas { return f(k, n) }})
	}
	F2("shadow_head", 96, 40, 1, 1, false, func(k, n int) *canvas { return renderShadow(96, 40, 0) })
	F2("shadow_segment", 64, 28, 1, 1, false, func(k, n int) *canvas { return renderShadow(64, 28, 0) })
	F2("shadow_head_rise", 96, 40, 10, 16, false, func(k, n int) *canvas { return renderShadow(96, 40, prog(k, n)) })

	// ── Сборка: цельный червь, порпойз-цикл (тело выходит из земли и уходит вниз).
	S = append(S, spec{name: "burrow_cycle", part: "showcase", dirs: dirDown, nframes: 24, fps: 14, loop: true,
		fw: wormW, fh: wormH, frame: func(_ float64, k, n int) *canvas { return renderWorm(float64(k) / float64(n)) }})

	return S
}

func upscaleFrame(sheet *image.RGBA, fr *canvas, col, row int) {
	ox, oy := col*fr.w, row*fr.h
	for y := 0; y < fr.h; y++ {
		for x := 0; x < fr.w; x++ {
			sheet.Set(ox+x, oy+y, fr.at(x, y))
		}
	}
}

func savePNG(path string, img image.Image) {
	os.MkdirAll(filepath.Dir(path), 0o755)
	f, _ := os.Create(path)
	defer f.Close()
	png.Encode(f, img)
}

func writePalette(path string) {
	groups := []struct {
		n string
		r []color.RGBA
	}{{"carapace", carapace}, {"flesh", flesh}, {"dirt", dirt}, {"acid", acid}, {"eye", eye}, {"gore", gore}}
	var b []byte
	b = append(b, []byte("# world_eater — приглушённая землистая палитра (рампы 5 значений) + акценты\n")...)
	for _, g := range groups {
		b = append(b, []byte("# "+g.n+"\n")...)
		for _, cc := range g.r {
			b = append(b, []byte(fmt.Sprintf("#%02X%02X%02X\n", cc.R, cc.G, cc.B))...)
		}
	}
	for _, e := range []struct {
		n string
		c color.RGBA
	}{{"carapace_spec", carapaceSpec}, {"wet_spec", wetSpec}, {"outline_dark", outlineDark}, {"outline_light", outlineLight}} {
		b = append(b, []byte(fmt.Sprintf("#%02X%02X%02X  %s\n", e.c.R, e.c.G, e.c.B, e.n))...)
	}
	os.WriteFile(path, b, 0o644)
}

func main() {
	root := "assets/sprites/bosses/world_eater"
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	os.MkdirAll(root, 0o755)
	writePalette(filepath.Join(root, "palette.hex"))

	var metas []animMeta
	for _, sp := range specs() {
		rows := len(sp.dirs)
		sheet := image.NewRGBA(image.Rect(0, 0, sp.fw*sp.nframes, sp.fh*rows))
		hitboxes := make([][4]int, sp.nframes)
		// Тени и мягкие эффекты не обводим контуром (это ломает полупрозрачность).
		// Тени, showcase (части уже обведены) и мягкие эффекты не обводим.
		soft := sp.part == "shadows" || sp.part == "showcase" || softFX[sp.name]
		for ri, d := range sp.dirs {
			for k := 0; k < sp.nframes; k++ {
				fr := sp.frame(d.phi, k, sp.nframes)
				if !soft {
					fr = outlined(fr)
				}
				upscaleFrame(sheet, fr, k, ri)
				if ri == 0 {
					x0, y0, x1, y1 := fr.bbox()
					hitboxes[k] = [4]int{x0, y0, x1 - x0 + 1, y1 - y0 + 1}
				}
			}
		}
		sub := sp.part
		file := filepath.Join(sub, sp.name+".png")
		savePNG(filepath.Join(root, file), sheet)

		dirNames := make([]string, rows)
		for i, d := range sp.dirs {
			dirNames[i] = d.name
		}
		ho := make([]int, sp.nframes)
		if sp.height != nil {
			for k := range ho {
				ho[k] = sp.height(k, sp.nframes)
			}
		}
		dmg := sp.damaging
		if dmg == nil {
			dmg = []int{}
		}
		pivot := [2]int{sp.fw / 2, sp.fh / 2}
		metas = append(metas, animMeta{
			Name: sp.name, Part: sp.part, File: file, Directions: rows, DirectionNames: dirNames, Mirrored: []string{},
			Frames: sp.nframes, FrameW: sp.fw, FrameH: sp.fh, FPS: sp.fps, Loop: sp.loop,
			Pivot: pivot, Damaging: dmg, Hitbox: hitboxes, HeightOff: ho,
			ShadowSprite: sp.shadow, ShadowOffset: sp.shadowOff, AttachFront: sp.attachF, AttachBack: sp.attachB,
		})
	}

	doc := map[string]any{
		"name":             "world_eater",
		"title":            "Пожиратель миров",
		"style":            "top-down ~65°, volumetric soft-shaded, body-horror (Necesse + Terraria/PoE)",
		"engine_target":    "Godot 4, 640x360, tile 32",
		"light":            "global, top-left-front; identical across all directions and frames",
		"sheet_layout":     "rows = directions, cols = frames",
		"directions_order": []string{"down", "down_left", "left", "up_left", "up", "up_right", "right", "down_right"},
		"mirrored_note":    "пусто: свет асимметричен, отражение ломает освещение — все 8 нарисованы честно",
		"palette":          "palette.hex",
		"chain":            map[string]any{"segments": 24, "spacing_px": 26, "follow": "follow-the-leader"},
		"depth_sort":       "используй height_offset[кадр] при выныривании: спрайт смещён вверх, тень остаётся на полу",
		"animations":       metas,
	}
	jb, _ := json.MarshalIndent(doc, "", "  ")
	os.WriteFile(filepath.Join(root, "world_eater.json"), jb, 0o644)

	fmt.Printf("world_eater: %d анимаций, %d направлений → %s\n", len(metas), 8, root)
}
