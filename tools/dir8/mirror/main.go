package main

// mirror отражает кадр по горизонтали внутри его 64x64 рамки.
// Отражение идёт по x -> 63-x: у пака левый и правый ряды связаны
// именно так, рамка персонажа при этом ложится сама на себя.
// usage: go run -tags mirror mirror.go <in.png> <out.png>

import (
	"image"
	"image/draw"
	"image/png"
	"os"
)

func main() {
	src := loadPNG(os.Args[1])
	b := src.Bounds()
	out := image.NewNRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			out.SetNRGBA(b.Max.X-1-x, y, src.NRGBAAt(x, y))
		}
	}
	writePNG(os.Args[2], out)
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

func must(err error) {
	if err != nil {
		panic(err)
	}
}
