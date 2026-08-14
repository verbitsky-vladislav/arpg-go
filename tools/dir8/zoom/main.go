package main

// zoom увеличивает кусок PNG с пиксельной сеткой и линейкой,
// чтобы можно было целиться в конкретный пиксель.
// usage: go run -tags zoom zoom.go <png> <x> <y> <w> <h> <scale> <out.png>

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"strconv"
)

func main() {
	src := loadPNG(os.Args[1])
	x0, y0 := atoi(os.Args[2]), atoi(os.Args[3])
	w, h := atoi(os.Args[4]), atoi(os.Args[5])
	s := atoi(os.Args[6])

	pad := s // поле под линейку
	out := image.NewNRGBA(image.Rect(0, 0, w*s+pad, h*s+pad))
	draw.Draw(out, out.Bounds(), &image.Uniform{color.NRGBA{0x2a, 0x2a, 0x38, 0xff}}, image.Point{}, draw.Src)

	// шахматка под прозрачностью
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			bg := color.NRGBA{0x3a, 0x3a, 0x4e, 0xff}
			if (x+y)%2 == 1 {
				bg = color.NRGBA{0x44, 0x44, 0x58, 0xff}
			}
			cell := image.Rect(pad+x*s, pad+y*s, pad+(x+1)*s, pad+(y+1)*s)
			draw.Draw(out, cell, &image.Uniform{bg}, image.Point{}, draw.Src)
			c := src.NRGBAAt(x0+x, y0+y)
			if c.A > 0 {
				draw.Draw(out, cell, &image.Uniform{c}, image.Point{}, draw.Over)
			}
		}
	}

	// сетка: каждый пиксель тонко, каждые 5 — ярко
	for x := 0; x <= w; x++ {
		c := color.NRGBA{0x55, 0x55, 0x6a, 0xff}
		if x%5 == 0 {
			c = color.NRGBA{0x88, 0x88, 0xaa, 0xff}
		}
		vline(out, pad+x*s, 0, h*s+pad, c)
	}
	for y := 0; y <= h; y++ {
		c := color.NRGBA{0x55, 0x55, 0x6a, 0xff}
		if y%5 == 0 {
			c = color.NRGBA{0x88, 0x88, 0xaa, 0xff}
		}
		hline(out, 0, pad+y*s, w*s+pad, c)
	}

	// метки на линейке: столбик точек = номер по модулю 5 нам не нужен,
	// достаточно жирной риски каждые 5 клеток, подписи читаем по ней.
	for x := 0; x < w; x += 5 {
		draw.Draw(out, image.Rect(pad+x*s, 0, pad+x*s+s/3, pad), &image.Uniform{color.NRGBA{0xdd, 0xdd, 0xee, 0xff}}, image.Point{}, draw.Src)
	}
	for y := 0; y < h; y += 5 {
		draw.Draw(out, image.Rect(0, pad+y*s, pad, pad+y*s+s/3), &image.Uniform{color.NRGBA{0xdd, 0xdd, 0xee, 0xff}}, image.Point{}, draw.Src)
	}
	writePNG(os.Args[7], out)
}

func vline(im *image.NRGBA, x, y0, y1 int, c color.NRGBA) {
	for y := y0; y < y1; y++ {
		im.SetNRGBA(x, y, c)
	}
}

func hline(im *image.NRGBA, x0, y, x1 int, c color.NRGBA) {
	for x := x0; x < x1; x++ {
		im.SetNRGBA(x, y, c)
	}
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

func writePNG(p string, im image.Image) {
	f, err := os.Create(p)
	must(err)
	defer f.Close()
	must(png.Encode(f, im))
}

func atoi(s string) int { n, err := strconv.Atoi(s); must(err); return n }

func must(err error) {
	if err != nil {
		panic(err)
	}
}
