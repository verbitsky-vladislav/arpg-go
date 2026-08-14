package main

// Режим проверки: рисует рамки спрайтов из манифеста поверх листа и кладёт
// картинки в out/iconnorm/. Нарезка тут выводится из пикселей, поэтому
// глазами по контактному листу видно сразу, где она разъехалась.

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

const overlayScale = 2

func overlayPack(dir, outDir string) error {
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return err
	}
	var man Manifest
	if err := json.Unmarshal(raw, &man); err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	for _, key := range sortedSheets(man.Sheets) {
		sh := man.Sheets[key]
		src, err := decode(filepath.Join(dir, sh.File))
		if err != nil {
			return err
		}
		b := src.Bounds()
		w, h := b.Dx()*overlayScale, b.Dy()*overlayScale
		dst := image.NewRGBA(image.Rect(0, 0, w, h))
		// шахматка под прозрачностью, иначе рамок на пустом фоне не видно
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				c := color.RGBA{40, 40, 46, 255}
				if (x/8+y/8)%2 == 0 {
					c = color.RGBA{54, 54, 62, 255}
				}
				dst.Set(x, y, c)
			}
		}
		for y := 0; y < b.Dy(); y++ {
			for x := 0; x < b.Dx(); x++ {
				c := src.At(b.Min.X+x, b.Min.Y+y)
				if _, _, _, a := c.RGBA(); a == 0 {
					continue
				}
				draw.Draw(dst, image.Rect(x*overlayScale, y*overlayScale, (x+1)*overlayScale, (y+1)*overlayScale),
					&image.Uniform{c}, image.Point{}, draw.Over)
			}
		}
		// кадр анимации — жёлтая сетка, спрайты — зелёные рамки
		if sh.Anim != nil {
			for i := 1; i < sh.Anim.Frames; i++ {
				if sh.Anim.Frame.W < b.Dx() {
					vline(dst, i*sh.Anim.Frame.W*overlayScale, 0, h, color.RGBA{255, 210, 60, 255})
				} else {
					hline(dst, 0, w, i*sh.Anim.Frame.H*overlayScale, color.RGBA{255, 210, 60, 255})
				}
			}
		}
		for _, s := range sh.Sprites {
			frame(dst, s.X*overlayScale, s.Y*overlayScale, s.W*overlayScale, s.H*overlayScale,
				color.RGBA{80, 230, 120, 255})
		}
		name := fmt.Sprintf("%s_%s.png", man.ID, key)
		f, err := os.Create(filepath.Join(outDir, name))
		if err != nil {
			return err
		}
		err = png.Encode(f, dst)
		f.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func frame(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	hline(img, x, x+w, y, c)
	hline(img, x, x+w, y+h-1, c)
	vline(img, x, y, y+h, c)
	vline(img, x+w-1, y, y+h, c)
}

func hline(img *image.RGBA, x0, x1, y int, c color.RGBA) {
	for x := x0; x < x1; x++ {
		img.SetRGBA(x, y, c)
	}
}

func vline(img *image.RGBA, x, y0, y1 int, c color.RGBA) {
	for y := y0; y < y1; y++ {
		img.SetRGBA(x, y, c)
	}
}

func sortedSheets(m map[string]*Sheet) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	for i := 1; i < len(ks); i++ {
		for j := i; j > 0 && strings.ToLower(ks[j]) < strings.ToLower(ks[j-1]); j-- {
			ks[j], ks[j-1] = ks[j-1], ks[j]
		}
	}
	return ks
}
