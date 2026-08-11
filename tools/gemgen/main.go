// Инструмент gemgen — генератор иконок гемов (native 32x32, ×8 = 256×256).
// Отдельный package main, только stdlib, В ИГРУ НЕ ВХОДИТ. Огранённый камень
// (цвет = категория/тип урона) + маленький глиф-подсказка. Две формы: round =
// СКИЛЛ-гем (6 умений), rhombus = КАМЕНЬ ПОДДЕРЖКИ (18 тегов, см.
// docs/mechanics/tags.md). Символ на прозрачном фоне с тёмным авто-контуром.
//
// Запуск из корня репозитория:
//
//	go run ./tools/gemgen assets/sprites/gems
package main

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const native, scale = 32, 8

type grid struct{ px [native][native]color.RGBA }

func (g *grid) set(x, y int, c color.RGBA) {
	if x >= 0 && y >= 0 && x < native && y < native {
		g.px[y][x] = c
	}
}
func (g *grid) at(x, y int) color.RGBA {
	if x < 0 || y < 0 || x >= native || y >= native {
		return color.RGBA{}
	}
	return g.px[y][x]
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
func (g *grid) line(x0, y0, x1, y1, thick int, c color.RGBA) {
	steps := int(math.Max(math.Abs(float64(x1-x0)), math.Abs(float64(y1-y0)))) + 1
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := int(math.Round(float64(x0) + t*float64(x1-x0)))
		y := int(math.Round(float64(y0) + t*float64(y1-y0)))
		g.disc(x, y, thick, c)
	}
}

// discOnly ставит пиксель только поверх непрозрачного (для затенения камня).
func (g *grid) overDisc(cx, cy, r int, c color.RGBA) {
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			dx, dy := x-cx, y-cy
			if dx*dx+dy*dy <= r*r+r/2 && g.at(x, y).A > 0 {
				g.set(x, y, c)
			}
		}
	}
}

func (g *grid) outline(o color.RGBA) {
	snap := *g
	f := func(x, y int) bool { return snap.at(x, y).A > 0 }
	for y := 0; y < native; y++ {
		for x := 0; x < native; x++ {
			if snap.at(x, y).A > 0 {
				continue
			}
			if f(x-1, y) || f(x+1, y) || f(x, y-1) || f(x, y+1) ||
				f(x-1, y-1) || f(x+1, y-1) || f(x-1, y+1) || f(x+1, y+1) {
				g.set(x, y, o)
			}
		}
	}
}

func hx(s string) color.RGBA {
	s = strings.TrimPrefix(s, "#")
	r, _ := strconv.ParseUint(s[0:2], 16, 8)
	gg, _ := strconv.ParseUint(s[2:4], 16, 8)
	b, _ := strconv.ParseUint(s[4:6], 16, 8)
	return color.RGBA{uint8(r), uint8(gg), uint8(b), 255}
}
func tint(c color.RGBA, k float64) color.RGBA {
	lp := func(v uint8) uint8 { return uint8(float64(v) + (255-float64(v))*k) }
	return color.RGBA{lp(c.R), lp(c.G), lp(c.B), 255}
}
func shade(c color.RGBA, k float64) color.RGBA {
	return color.RGBA{uint8(float64(c.R) * k), uint8(float64(c.G) * k), uint8(float64(c.B) * k), 255}
}
func luma(c color.RGBA) float64 { return 0.3*float64(c.R) + 0.59*float64(c.G) + 0.11*float64(c.B) }

var OUT = hx("#141018")

const cx, cy = 16, 16

// gemBase рисует огранённый камень (round|rhombus) цвета base с гранями и бликом.
func gemBase(g *grid, shape string, base color.RGBA) {
	r := 11
	if shape == "round" {
		g.disc(cx, cy, r, base)
	} else { // rhombus (ромб)
		for y := cy - r; y <= cy+r; y++ {
			hw := r - int(math.Abs(float64(y-cy)))
			g.rect(cx-hw, y, cx+hw, y, base)
		}
	}
	dark := shade(base, 0.62)
	light := tint(base, 0.45)
	// нижние грани темнее
	g.overDisc(cx, cy+6, r, dark)
	// «стол» сверху светлее (маленький кристалл-блик)
	g.overDisc(cx-2, cy-4, 4, light)
	// грани от центра
	g.line(cx, cy, cx-r, cy-2, 0, dark)
	g.line(cx, cy, cx+r, cy-2, 0, dark)
	g.line(cx, cy, cx, cy+r, 0, dark)
	// блик
	g.set(cx-4, cy-5, hx("#ffffff"))
	g.set(cx-3, cy-5, tint(light, 0.6))
}

// glyphColor — контрастный к камню цвет глифа.
func glyphColor(base color.RGBA) color.RGBA {
	if luma(base) > 150 {
		return hx("#20161c")
	}
	return hx("#ffffff")
}

// --- Глифы (маленькие, ~в боксе 10..22 x 8..20) ---

func gSword(g *grid, c color.RGBA) {
	g.rect(15, 8, 16, 18, c)
	g.rect(13, 16, 18, 16, c)
	g.set(15, 19, c)
	g.set(16, 19, c)
}
func gArrow(g *grid, c color.RGBA) { // стрелка ↗
	g.line(11, 21, 20, 10, 0, c)
	g.line(20, 10, 15, 10, 0, c)
	g.line(20, 10, 20, 15, 0, c)
}
func gDash(g *grid, c color.RGBA) { // →
	g.rect(11, 15, 19, 16, c)
	g.line(19, 15, 15, 11, 0, c)
	g.line(19, 16, 15, 20, 0, c)
}
func gSpark(g *grid, c color.RGBA) { // искра-звезда
	g.line(16, 8, 16, 22, 0, c)
	g.line(9, 15, 23, 15, 0, c)
	g.set(12, 11, c)
	g.set(20, 11, c)
	g.set(12, 19, c)
	g.set(20, 19, c)
}
func gRing(g *grid, c color.RGBA) { // кольцо
	g.disc(16, 15, 5, c)
	g.disc(16, 15, 3, hx("#00000000"))
	// вырез — уберём центр: перекрасим обратно позже сложно; рисуем кольцо линиями
}
func gRingProper(g *grid, c color.RGBA) {
	for a := 0; a < 360; a += 20 {
		rad := float64(a) * math.Pi / 180
		g.set(16+int(math.Round(5*math.Cos(rad))), 15+int(math.Round(5*math.Sin(rad))), c)
	}
}
func gAura(g *grid, c color.RGBA) {
	gRingProper(g, c)
	for a := 0; a < 360; a += 30 {
		rad := float64(a) * math.Pi / 180
		g.set(16+int(math.Round(2.5*math.Cos(rad))), 15+int(math.Round(2.5*math.Sin(rad))), c)
	}
}
func gDot(g *grid, c color.RGBA) { // снаряд: точка + след
	g.disc(19, 15, 2, c)
	g.rect(10, 15, 16, 15, c)
}
func gHour(g *grid, c color.RGBA) { // песочные часы
	g.rect(11, 9, 21, 9, c)
	g.rect(11, 21, 21, 21, c)
	for i := 0; i <= 5; i++ {
		g.set(11+i, 9+i, c)
		g.set(21-i, 9+i, c)
		g.set(16-i, 15+i, c)
		g.set(16+i, 15+i, c)
	}
}
func gHammer(g *grid, c color.RGBA) {
	g.rect(11, 9, 21, 12, c)
	g.rect(15, 12, 16, 21, c)
}
func gTotem(g *grid, c color.RGBA) { // призыв/турель
	g.rect(13, 18, 19, 21, c)
	for y := 10; y <= 18; y++ {
		hw := (y - 10) / 3
		g.rect(15-hw, y, 17+hw, y, c)
	}
	g.disc(16, 10, 1, c)
}
func gSkull(g *grid, c color.RGBA) {
	g.disc(16, 14, 4, c)
	g.rect(14, 17, 18, 19, c)
	dc := shade(c, 0.2)
	if luma(c) < 80 {
		dc = tint(c, 0.6)
	}
	g.set(14, 14, dc)
	g.set(18, 14, dc)
}
func gTri(g *grid, c color.RGBA) { // стихия: 3 точки
	g.disc(16, 11, 1, c)
	g.disc(12, 19, 1, c)
	g.disc(20, 19, 1, c)
}
func gFlame(g *grid, c color.RGBA) {
	g.disc(16, 17, 3, c)
	for y := 17; y >= 9; y-- {
		hw := (y - 9) / 3
		g.rect(16-hw, y, 16+hw, y, c)
	}
}
func gSnow(g *grid, c color.RGBA) {
	for _, a := range []float64{90, 30, 150} {
		rad := a * math.Pi / 180
		dx, dy := math.Cos(rad), math.Sin(rad)
		g.line(16+int(6*dx), 15+int(6*dy), 16-int(6*dx), 15-int(6*dy), 0, c)
	}
}
func gBolt(g *grid, c color.RGBA) {
	g.line(19, 8, 14, 15, 0, c)
	g.line(14, 15, 18, 15, 0, c)
	g.line(18, 15, 13, 22, 0, c)
}
func gDrop(g *grid, c color.RGBA) {
	g.disc(16, 17, 3, c)
	for y := 17; y >= 10; y-- {
		hw := (17 - y) / 3
		g.rect(16-hw, y, 16+hw, y, c)
	}
}
func gArc(g *grid, c color.RGBA) { // дуга-удар
	for a := 200; a <= 340; a += 8 {
		rad := float64(a) * math.Pi / 180
		g.set(16+int(math.Round(7*math.Cos(rad))), 16+int(math.Round(7*math.Sin(rad))), c)
	}
}
func gSwirl(g *grid, c color.RGBA) { // вихрь
	for a := 0; a <= 540; a += 12 {
		rad := float64(a) * math.Pi / 180
		rr := 1.5 + float64(a)/120
		g.set(16+int(math.Round(rr*math.Cos(rad))), 15+int(math.Round(rr*math.Sin(rad))), c)
	}
}
func gFist(g *grid, c color.RGBA) {
	g.disc(16, 15, 4, c)
	g.rect(12, 13, 20, 13, shade(c, 0.6))
}

type gem struct {
	name, shape, col string
	glyph            func(*grid, color.RGBA)
}

var skills = []gem{
	{"raskol", "round", "#8a94a8", gArc},
	{"vortex", "round", "#d0403a", gSwirl},
	{"pierce", "round", "#46b45a", gArrow},
	{"dash", "round", "#5a5ad0", gDash},
	{"fireball", "round", "#ff8c28", gFlame},
	{"turret", "round", "#caa23a", gTotem},
}

var supports = []gem{
	{"attack", "rhombus", "#d0403a", gSword},
	{"melee", "rhombus", "#d0403a", gFist},
	{"ranged", "rhombus", "#46b45a", gArrow},
	{"spell", "rhombus", "#4a80e0", gSpark},
	{"area", "rhombus", "#4a80e0", gRingProper},
	{"projectile", "rhombus", "#46b45a", gDot},
	{"movement", "rhombus", "#46b45a", gDash},
	{"summon", "rhombus", "#4a80e0", gTotem},
	{"minions", "rhombus", "#4a80e0", gSkull},
	{"aura", "rhombus", "#4a80e0", gAura},
	{"curse", "rhombus", "#a05ad0", gSkull},
	{"duration", "rhombus", "#4a80e0", gHour},
	{"physical", "rhombus", "#9aa2b0", gHammer},
	{"elemental", "rhombus", "#e8e8f0", gTri},
	{"fire", "rhombus", "#ff8c28", gFlame},
	{"cold", "rhombus", "#4ac8e0", gSnow},
	{"lightning", "rhombus", "#ffe23a", gBolt},
	{"chaos", "rhombus", "#a05ad0", gDrop},
}

func render(gm gem) *grid {
	g := &grid{}
	base := hx(gm.col)
	gemBase(g, gm.shape, base)
	gm.glyph(g, glyphColor(base))
	g.outline(OUT)
	return g
}

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
func save(dir, name string, g *grid) {
	os.MkdirAll(dir, 0o755)
	f, _ := os.Create(filepath.Join(dir, name+".png"))
	png.Encode(f, upscale(g))
	f.Close()
}

func main() {
	out := os.Args[1]
	for _, gm := range skills {
		save(filepath.Join(out, "skill"), gm.name, render(gm))
	}
	for _, gm := range supports {
		save(filepath.Join(out, "support"), gm.name, render(gm))
	}
}
