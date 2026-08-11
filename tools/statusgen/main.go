// Инструмент statusgen — генератор иконок статусов для HUD (native 32×32, ×8 =
// 256×256). Отдельный package main, только stdlib, В ИГРУ НЕ ВХОДИТ. Каждая
// иконка — символ на прозрачном фоне с тёмным авто-контуром, цвет по типу урона
// (см. docs/mechanics/damage-and-defense.md). 10 статусов: ignite/freeze/shock/
// stun/bleed/sunder/poison/curse/weaken/wither.
//
// Запуск из корня репозитория:
//
//	go run ./tools/statusgen assets/sprites/status
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

// line рисует толстую линию (thick=радиус) цветом c.
func (g *grid) line(x0, y0, x1, y1, thick int, c color.RGBA) {
	steps := int(math.Max(math.Abs(float64(x1-x0)), math.Abs(float64(y1-y0)))) + 1
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := int(math.Round(float64(x0) + t*float64(x1-x0)))
		y := int(math.Round(float64(y0) + t*float64(y1-y0)))
		g.disc(x, y, thick, c)
	}
}

// triUp — треугольник-остриё вверх: от базы (y0, x0..x1) к вершине (cx, ytip).
func (g *grid) triUp(cx, x0, x1, y0, ytip int, c color.RGBA) {
	for y := ytip; y <= y0; y++ {
		t := float64(y-ytip) / float64(y0-ytip)
		hw := t * float64(x1-x0) / 2
		l := int(math.Round(float64(cx) - hw))
		r := int(math.Round(float64(cx) + hw))
		for x := l; x <= r; x++ {
			g.set(x, y, c)
		}
	}
}

// triDown — треугольник-остриё вниз: от базы (ytop, x0..x1) к вершине (cx, ytip).
func (g *grid) triDown(cx, x0, x1, ytop, ytip int, c color.RGBA) {
	for y := ytop; y <= ytip; y++ {
		t := float64(ytip-y) / float64(ytip-ytop)
		hw := t * float64(x1-x0) / 2
		l := int(math.Round(float64(cx) - hw))
		r := int(math.Round(float64(cx) + hw))
		for x := l; x <= r; x++ {
			g.set(x, y, c)
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

var OUT = hx("#14121a")

// --- Иконки ---

func ignite(g *grid) { // пламя
	body, core, edge := hx("#ff8c28"), hx("#ffd24a"), hx("#e0401a")
	g.disc(16, 19, 6, body)
	g.triUp(16, 10, 22, 20, 5, body) // язык вверх
	g.rect(11, 22, 21, 24, edge)     // низ горячее
	g.disc(16, 20, 3, core)          // ядро
	g.triUp(16, 13, 19, 20, 9, core)
	g.set(19, 12, body) // всполох сбоку
	g.set(13, 14, body)
	g.outline(OUT)
}

func freeze(g *grid) { // снежинка
	c, core := hx("#9fe8ff"), hx("#ffffff")
	cx, cy := 16, 16
	for _, a := range []float64{90, 30, 150} { // 3 оси = 6 лучей
		rad := a * math.Pi / 180
		dx, dy := math.Cos(rad), math.Sin(rad)
		x1, y1 := cx+int(math.Round(dx*9)), cy+int(math.Round(dy*9))
		x2, y2 := cx-int(math.Round(dx*9)), cy-int(math.Round(dy*9))
		g.line(x1, y1, x2, y2, 0, c)
		// веточки на концах
		for _, e := range [][2]int{{x1, y1}, {x2, y2}} {
			bx, by := int(float64(e[0])-dx*3), int(float64(e[1])-dy*3)
			g.line(e[0], e[1], bx+int(dy*3), by-int(dx*3), 0, c)
			g.line(e[0], e[1], bx-int(dy*3), by+int(dx*3), 0, c)
		}
	}
	g.disc(cx, cy, 1, core)
	g.outline(OUT)
}

func shock(g *grid) { // молния
	c, core := hx("#ffe23a"), hx("#fff6b0")
	pts := [][2]int{{19, 4}, {12, 15}, {17, 15}, {11, 28}}
	for i := 0; i < len(pts)-1; i++ {
		g.line(pts[i][0], pts[i][1], pts[i+1][0], pts[i+1][1], 1, c)
	}
	for i := 0; i < len(pts)-1; i++ {
		g.line(pts[i][0], pts[i][1], pts[i+1][0], pts[i+1][1], 0, core)
	}
	g.outline(OUT)
}

func stun(g *grid) { // звёздочки-«оглушение»
	c := hx("#ffe23a")
	star := func(cx, cy, r int) {
		g.line(cx-r, cy, cx+r, cy, 0, c)
		g.line(cx, cy-r, cx, cy+r, 0, c)
		g.set(cx, cy, hx("#fff6b0"))
	}
	star(10, 10, 4)
	star(21, 8, 3)
	star(17, 15, 2)
	g.outline(OUT)
}

func bleed(g *grid) { // капля крови
	body, hl := hx("#e0303a"), hx("#ff8080")
	g.disc(16, 20, 5, body)
	g.triUp(16, 11, 21, 21, 8, body)
	g.disc(14, 19, 1, hl) // блик
	g.outline(OUT)
}

func sunder(g *grid) { // треснувший щит
	steel, shd, dark := hx("#8a94a4"), hx("#5f6676"), hx("#2e3340")
	g.rect(9, 6, 22, 16, steel)         // верх щита
	g.triDown(15, 9, 22, 16, 26, steel) // низ к острию
	g.rect(9, 6, 10, 16, shd)           // тень слева
	g.triDown(15, 12, 15, 16, 24, shd)
	// широкая трещина-зигзаг
	g.line(15, 6, 13, 12, 1, dark)
	g.line(13, 12, 18, 16, 1, dark)
	g.line(18, 16, 14, 24, 1, dark)
	g.outline(OUT)
}

func poison(g *grid) { // ядовитая капля + пузыри
	body, hl := hx("#46c84a"), hx("#a8ff8a")
	g.disc(15, 20, 5, body)
	g.triUp(15, 10, 20, 21, 9, body)
	g.disc(13, 19, 1, hl)
	g.disc(22, 13, 2, body) // пузырь
	g.disc(24, 19, 1, body)
	g.outline(OUT)
}

func curse(g *grid) { // череп-проклятие
	bone, dark := hx("#c8b8e8"), hx("#3a2a5a")
	g.disc(16, 14, 6, bone)      // черепная коробка
	g.rect(13, 18, 19, 22, bone) // челюсть
	g.set(13, 15, dark)          // глазницы
	g.set(14, 15, dark)
	g.set(18, 15, dark)
	g.set(19, 15, dark)
	g.rect(15, 18, 17, 19, dark) // нос
	g.set(14, 21, dark)          // зубы
	g.set(16, 21, dark)
	g.set(18, 21, dark)
	g.outline(OUT)
}

func weaken(g *grid) { // стрелка вниз (ослабление)
	c, hl := hx("#9a8ab0"), hx("#c8bce0")
	g.rect(14, 5, 17, 17, c)        // древко
	g.triDown(15, 9, 22, 16, 26, c) // остриё вниз
	g.rect(14, 5, 14, 17, hl)       // блик
	g.outline(OUT)
}

func wither(g *grid) { // поникший (увядший) цветок
	stem, head, dead := hx("#6a7a3a"), hx("#8a7aa0"), hx("#4a3a5a")
	// стебель снизу вверх, затем согнутая «шея» влево
	g.line(16, 27, 16, 15, 0, stem)
	g.line(16, 15, 12, 11, 0, stem)
	// поникшая головка (смотрит вниз), увядшая
	g.disc(11, 11, 3, head)
	g.disc(11, 12, 2, dead) // тёмная сердцевина/гниль
	g.set(9, 13, dead)
	g.set(13, 13, dead)
	// опавший лепесток/лист
	g.line(18, 18, 21, 22, 0, stem)
	g.disc(21, 22, 1, dead)
	g.outline(OUT)
}

var icons = map[string]func(*grid){
	"ignite": ignite, "freeze": freeze, "shock": shock, "stun": stun,
	"bleed": bleed, "sunder": sunder, "poison": poison, "curse": curse,
	"weaken": weaken, "wither": wither,
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

func main() {
	out := os.Args[1]
	order := []string{"ignite", "freeze", "shock", "stun", "bleed", "sunder", "poison", "curse", "weaken", "wither"}
	for _, n := range order {
		g := &grid{}
		icons[n](g)
		os.MkdirAll(out, 0o755)
		f, _ := os.Create(filepath.Join(out, n+".png"))
		png.Encode(f, upscale(g))
		f.Close()
	}
}
