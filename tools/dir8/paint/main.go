// paint собирает один 64x64 кадр из текстовых сеток — этим удобно
// смотреть правку, не пересобирая весь лист.
//
// Сетки рисуются по порядку, поверх друг друга; каждой можно задать
// сдвиг. Так голова пишется один раз на весь цикл, а по кадрам
// меняются только тело с мечом.
//
// usage: go run ./tools/dir8/paint <out.png> <grid.txt[:dx,dy]>...
package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"strings"

	"github.com/vladislav/game/tools/dir8/internal/grid"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: paint <out.png> <grid.txt[:dx,dy]>...")
		os.Exit(2)
	}
	out := image.NewNRGBA(image.Rect(0, 0, grid.FrameSize, grid.FrameSize))
	for _, spec := range os.Args[2:] {
		path, shift := parseSpec(spec)
		l, err := grid.Read(path)
		check(err)
		check(l.Draw(out, shift, path))
	}
	f, err := os.Create(os.Args[1])
	check(err)
	defer f.Close()
	check(png.Encode(f, out))
}

// parseSpec разбирает "grid.txt" или "grid.txt:dx,dy".
func parseSpec(s string) (string, image.Point) {
	i := strings.LastIndex(s, ":")
	if i < 0 {
		return s, image.Point{}
	}
	var dx, dy int
	if _, err := fmt.Sscanf(s[i+1:], "%d,%d", &dx, &dy); err != nil {
		return s, image.Point{}
	}
	return s[:i], image.Pt(dx, dy)
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "paint:", err)
		os.Exit(1)
	}
}
