package main

// islands считает связные куски непрозрачных пикселей в каждом кадре,
// не считая тени на земле. Оторванный от персонажа меч даёт второй
// кусок — так баг ловится без разглядывания.
import (
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
)

func main() {
	f, _ := os.Open(os.Args[1])
	src, err := png.Decode(f)
	if err != nil {
		panic(err)
	}
	im := image.NewNRGBA(src.Bounds())
	draw.Draw(im, src.Bounds(), src, src.Bounds().Min, draw.Src)
	b := im.Bounds()
	for row := 0; row*64 < b.Dy(); row++ {
		out := fmt.Sprintf("ряд %d:", row)
		for col := 0; col*64 < b.Dx(); col++ {
			out += fmt.Sprintf(" %d", count(im, col*64, row*64))
		}
		fmt.Println(out)
	}
}

func count(im *image.NRGBA, ox, oy int) int {
	solid := func(x, y int) bool {
		if x < 0 || y < 0 || x >= 64 || y >= 64 {
			return false
		}
		return im.NRGBAAt(ox+x, oy+y).A > 200 // тень полупрозрачная, её не берём
	}
	seen := map[[2]int]bool{}
	n := 0
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			if !solid(x, y) || seen[[2]int{x, y}] {
				continue
			}
			n++
			stack := [][2]int{{x, y}}
			for len(stack) > 0 {
				p := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				if seen[p] || !solid(p[0], p[1]) {
					continue
				}
				seen[p] = true
				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						stack = append(stack, [2]int{p[0] + dx, p[1] + dy})
					}
				}
			}
		}
	}
	return n
}
