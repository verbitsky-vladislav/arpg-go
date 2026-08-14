package main

// topaint переводит кусок PNG в текстовую сетку палитры paint.go,
// чтобы можно было начать рисовать поверх готового кадра.
// usage: go run -tags topaint topaint.go <png> <x> <y> <w> <h>

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"strconv"

	"github.com/vladislav/game/tools/dir8/internal/grid"
)

func main() {
	src := loadPNG(os.Args[1])
	x0, y0 := atoi(os.Args[2]), atoi(os.Args[3])
	w, h := atoi(os.Args[4]), atoi(os.Args[5])
	fmt.Printf("@ %d %d\n", x0, y0)
	for y := 0; y < h; y++ {
		line := make([]byte, w)
		for x := 0; x < w; x++ {
			line[x] = glyph(src.NRGBAAt(x0+x, y0+y))
		}
		fmt.Printf("%s   # %2d\n", line, y)
	}
}

// glyph — ближайший цвет палитры; '?' если цвет вне пака.
func glyph(c color.NRGBA) byte {
	if c.A == 0 {
		return '.'
	}
	best, bestD := byte('?'), 1<<30
	for g, p := range grid.Palette {
		if (p.A > 200) != (c.A > 200) {
			continue
		}
		dr, dg, db := int(p.R)-int(c.R), int(p.G)-int(c.G), int(p.B)-int(c.B)
		if d := dr*dr + dg*dg + db*db; d < bestD {
			best, bestD = g, d
		}
	}
	return best
}

func loadPNG(p string) *image.NRGBA {
	f, err := os.Open(p)
	must(err)
	defer f.Close()
	s, err := png.Decode(f)
	must(err)
	d := image.NewNRGBA(s.Bounds())
	draw.Draw(d, s.Bounds(), s, s.Bounds().Min, draw.Src)
	return d
}

func atoi(s string) int { n, err := strconv.Atoi(s); must(err); return n }

func must(err error) {
	if err != nil {
		panic(err)
	}
}
