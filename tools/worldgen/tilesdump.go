package main

// tilesdump.go — режим `worldgen tiles <biome-dir> <sheet> <id,id,...>`:
// выложить перечисленные тайлы в ряд крупно, с номерами. Нужен, чтобы глазами
// сверить, что именно лежит под ключом углового набора: атлас целиком слишком
// мелкий, чтобы отличить «трава на скале» от «травяная полка».

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"
	"path/filepath"
	"strings"
)

func runTilesDump(args []string) {
	if len(args) < 3 {
		fmt.Fprintln(os.Stderr, "worldgen tiles <biome-dir> <sheet> <id,id,...>")
		os.Exit(2)
	}
	biomeDir, sheet := args[0], args[1]
	var ids []int
	for _, s := range strings.Split(args[2], ",") {
		if v, ok := atoi(strings.TrimSpace(s)); ok {
			ids = append(ids, v)
		}
	}
	m, err := LoadManifest(biomeDir)
	if err != nil {
		fatal(err)
	}
	atlas, err := NewAtlasSet(m)
	if err != nil {
		fatal(err)
	}
	const scale, pad = 8, 4
	cell := m.TileSize*scale + pad*2
	out := image.NewRGBA(image.Rect(0, 0, cell*len(ids), cell))
	draw.Draw(out, out.Bounds(), &image.Uniform{color.RGBA{120, 170, 90, 255}}, image.Point{}, draw.Src)
	for i, id := range ids {
		t := atlas.Tile(sheet, id)
		if t == nil {
			continue
		}
		drawTileScaled(out, t, i*cell+pad, pad, scale)
	}
	outDir := filepath.Join("out", "worldgen", m.ID)
	_ = os.MkdirAll(outDir, 0o755)
	name := filepath.Join(outDir, "tiles_"+sheet+".png")
	if err := writePNG(name, out); err != nil {
		fatal(err)
	}
	fmt.Printf("%s %v → %s (слева направо в порядке аргумента)\n", sheet, ids, name)
}
