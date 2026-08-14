package main

// export_png.go — отладочные PNG: поля высоты/влажности в оттенках серого и
// карта уровней в цвете. Без них не подобрать параметры шума (worldgen.spec §9.2).

import (
	"image"
	"image/color"
	"image/png"
	"os"
)

// writePNG кодирует изображение в файл.
func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// grayField рендерит float-поле [0,1] в серый PNG с масштабом scale (пиксель→блок).
func grayField(g *Grid[float64], scale int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, g.W*scale, g.H*scale))
	for y := 0; y < g.H; y++ {
		for x := 0; x < g.W; x++ {
			v := g.At(x, y)
			if v < 0 {
				v = 0
			}
			if v > 1 {
				v = 1
			}
			c := color.RGBA{uint8(v * 255), uint8(v * 255), uint8(v * 255), 255}
			fillBlock(img, x*scale, y*scale, scale, c)
		}
	}
	return img
}

// levelColors — цвета уровней для отладочной карты.
var levelColors = map[Level]color.RGBA{
	LiquidDeep:    {28, 52, 104, 255},  // тёмно-синий
	LiquidShallow: {70, 120, 168, 255}, // светло-синий
	Ground:        {96, 148, 74, 255},  // зелёный
	Plateau:       {150, 176, 96, 255}, // светлее (выше)
}

// levelField рендерит карту уровней в цвет с масштабом scale.
func levelField(g *Grid[Level], scale int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, g.W*scale, g.H*scale))
	for y := 0; y < g.H; y++ {
		for x := 0; x < g.W; x++ {
			c := levelColors[g.At(x, y)]
			fillBlock(img, x*scale, y*scale, scale, c)
		}
	}
	return img
}

func fillBlock(img *image.RGBA, px, py, size int, c color.RGBA) {
	for dy := 0; dy < size; dy++ {
		for dx := 0; dx < size; dx++ {
			img.SetRGBA(px+dx, py+dy, c)
		}
	}
}
