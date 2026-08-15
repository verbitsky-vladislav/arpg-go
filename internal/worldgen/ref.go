package worldgen

// ref.go — режим `worldgen ref <biome-dir> <tmx>`: отрисовать карту художника
// НАШИМ атласом и рядом напечатать, какой роли (wangset'у манифеста) принадлежит
// каждый тайл. Это эталон: elevated_test.tsx.tmx — вручную собранное плато,
// по нему сверяется раскладка stage_plateau.

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// tileRole — обратный индекс «лист+локальный id → имя роли манифеста».
type tileRole map[string]map[int]string

func buildTileRoles(m *Manifest) tileRole {
	tr := tileRole{}
	for role, t := range m.Terrains {
		if t.Corner == nil {
			continue
		}
		if tr[t.Sheet] == nil {
			tr[t.Sheet] = map[int]string{}
		}
		for _, ids := range t.Corner {
			for _, id := range ids {
				if prev, ok := tr[t.Sheet][id]; ok && prev != role {
					tr[t.Sheet][id] = prev + "|" + role
					continue
				}
				tr[t.Sheet][id] = role
			}
		}
	}
	return tr
}

// cornerKeyOf — угловой ключ тайла в разметке (для печати эталона).
func cornerKeyOf(m *Manifest, sheet string, id int) string {
	for _, t := range m.Terrains {
		if t.Sheet != sheet {
			continue
		}
		for k, ids := range t.Corner {
			for _, v := range ids {
				if v == id {
					return k
				}
			}
		}
	}
	return "?"
}

func RunRef(args []string) {
	biomeDir := "assets/biomes/forest"
	tmxPath := ""
	for _, a := range args {
		if a == "" || a[0] == '-' {
			continue
		}
		if strings.HasSuffix(a, ".tmx") {
			tmxPath = a
		} else {
			biomeDir = a
		}
	}
	m, err := LoadManifest(biomeDir)
	if err != nil {
		fatal(err)
	}
	if tmxPath == "" {
		tmxPath = filepath.Join(biomeDir, "tiles", "elevated_test.tsx.tmx")
	}
	t, err := parseTMX(tmxPath)
	if err != nil {
		fatal(err)
	}
	atlas, err := NewAtlasSet(m)
	if err != nil {
		fatal(err)
	}
	roles := buildTileRoles(m)

	ts, scale := m.TileSize, m.RenderScale*2
	canvas := image.NewRGBA(image.Rect(0, 0, t.Width*ts*scale, t.Height*ts*scale))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{color.RGBA{40, 40, 48, 255}}, image.Point{}, draw.Src)

	// клетки слоёв в порядке файла (снизу вверх, как в Tiled)
	type cellInfo struct {
		sheet, role string
		id          int
	}
	info := map[[2]int][]cellInfo{}
	for _, l := range t.Layers {
		for _, c := range csvCells(l, t.Width) {
			sheet, id := sheetForGID(t.Tilesets, c.GID)
			tile := atlas.Tile(sheet, id)
			if tile != nil {
				drawTileScaled(canvas, tile, c.X*ts*scale, c.Y*ts*scale, scale, 0)
			}
			role := roles[sheet][id]
			if role == "" {
				role = "-"
			}
			info[[2]int{c.X, c.Y}] = append(info[[2]int{c.X, c.Y}], cellInfo{sheet, role, id})
		}
	}

	outDir := filepath.Join("out", "worldgen", m.ID)
	_ = os.MkdirAll(outDir, 0o755)
	name := filepath.Join(outDir, "ref_"+strings.TrimSuffix(filepath.Base(tmxPath), ".tmx")+".png")
	if err := writePNG(name, canvas); err != nil {
		fatal(err)
	}

	// текстовая раскладка ролей: по одной строке на клетку с непустым верхним слоем
	var keys [][2]int
	for k := range info {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i][1] != keys[j][1] {
			return keys[i][1] < keys[j][1]
		}
		return keys[i][0] < keys[j][0]
	})
	var b strings.Builder
	fmt.Fprintf(&b, "# эталон %s (%dx%d)\n# x,y : лист/локальный id : роль : угловой ключ\n", tmxPath, t.Width, t.Height)
	for _, k := range keys {
		for _, ci := range info[k] {
			fmt.Fprintf(&b, "%2d,%2d : %-12s %4d : %-12s : %s\n",
				k[0], k[1], ci.sheet, ci.id, ci.role, cornerKeyOf(m, ci.sheet, ci.id))
		}
	}
	dump := filepath.Join(outDir, "ref_roles.txt")
	_ = os.WriteFile(dump, []byte(b.String()), 0o644)
	fmt.Printf("эталон → %s\nроли    → %s\n", name, dump)
}

// csvCells распаковывает обычный (не-infinite) CSV-слой, зная ширину карты.
// tmxLayer.cells() рассчитан на чанки и для плоского CSV ширину не знает.
func csvCells(l tmxLayer, width int) []tmxCell {
	if len(l.Data.Chunks) > 0 {
		return l.cells()
	}
	nums := splitCSV(l.Data.Text)
	out := make([]tmxCell, 0, len(nums))
	for i, v := range nums {
		if v == 0 {
			continue
		}
		out = append(out, tmxCell{X: i % width, Y: i / width, GID: v & gidMask})
	}
	return out
}
