package main

// tmxexport.go — режим `worldgen tmx <biome-dir> [seed] [size]`: выгрузить
// сгенерированную карту в .tmx, чтобы открыть её в Tiled поверх тех же .tsx и
// посмотреть на каждый конкретный тайл. Нужен для разбора претензий вида «вот
// этот кадр встаёт не туда»: на PNG видно только результат, а здесь — id тайла,
// слой и лист, из которого он взят.
//
// Разрежённые слои (обрыв, лестницы, берег) кладут в одну клетку несколько
// тайлов друг на друга — в Tiled так нельзя, поэтому такой слой раскладывается
// на несколько «этажей» в порядке отрисовки: cliff, cliff_2, cliff_3…

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func runTMXExport(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "worldgen tmx <biome-dir> [seed] [size]")
		os.Exit(2)
	}
	biomeDir := args[0]
	seed, size := 1, 160
	if len(args) > 1 {
		if v, ok := atoi(args[1]); ok {
			seed = v
		}
	}
	if len(args) > 2 {
		if v, ok := atoi(args[2]); ok {
			size = v
		}
	}
	m, err := LoadManifest(biomeDir)
	if err != nil {
		fatal(err)
	}
	// сид считается так же, как в основном прогоне (runGenerate), иначе
	// выгрузка окажется другой картой, чем seed_N.png
	g := NewGenerator(defaultParams(size, size), uint64(seed)*0x9E3779B97F4A7C15, m)
	g.Run()
	mp := g.ToMapV1(int64(seed))

	outDir := filepath.Join("out", "worldgen", m.ID)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fatal(err)
	}
	name := filepath.Join(outDir, fmt.Sprintf("seed_%d.tmx", seed))
	if err := writeTMX(name, mp, m, biomeDir, outDir); err != nil {
		fatal(err)
	}
	fmt.Printf("tmx → %s (сид %d, %dx%d)\n", name, seed, mp.Width, mp.Height)
}

// tmxOutLayer — один слой на выгрузку: плотный массив gid по клеткам.
type tmxOutLayer struct {
	name string
	data []uint32
}

func writeTMX(path string, mp *MapV1, m *Manifest, biomeDir, outDir string) error {
	W, H := mp.Width, mp.Height
	// gid листа: firstgid уже посчитан в sheetRefs, локальный id прибавляется.
	first := map[string]int{}
	byIndex := make([]string, len(mp.Sheets))
	for i, s := range mp.Sheets {
		first[s.Name] = s.Firstgid
		byIndex[i] = s.Name
	}

	var layers []tmxOutLayer
	// плотный слой: локальные id листа роли, 0 = пусто
	dense := func(name, role string, data []uint16) {
		t, ok := m.Terrains[role]
		if !ok || data == nil {
			return
		}
		base := first[t.Sheet]
		out := make([]uint32, W*H)
		for i, v := range data {
			if v != 0 {
				out[i] = uint32(base + int(v) - 1)
			}
		}
		layers = append(layers, tmxOutLayer{name, out})
	}
	// разрежённый слой: раскладываем по этажам, чтобы ничего не потерять
	sparse := func(name string, tiles []SparseTile) {
		var floors [][]uint32
		for _, st := range tiles {
			if st.X < 0 || st.Y < 0 || st.X >= W || st.Y >= H {
				continue
			}
			sheet := ""
			if int(st.Sheet) < len(byIndex) {
				sheet = byIndex[st.Sheet]
			}
			gid := uint32(first[sheet] + int(st.Tile))
			idx := st.Y*W + st.X
			placed := false
			for f := range floors {
				if floors[f][idx] == 0 {
					floors[f][idx] = gid
					placed = true
					break
				}
			}
			if !placed {
				nf := make([]uint32, W*H)
				nf[idx] = gid
				floors = append(floors, nf)
			}
		}
		for f, data := range floors {
			n := name
			if f > 0 {
				n = fmt.Sprintf("%s_%d", name, f+1)
			}
			layers = append(layers, tmxOutLayer{n, data})
		}
	}

	// порядок как в рендере, снизу вверх
	dense("mud", "mud", mp.Layers.Mud)
	dense("ground", "ground", mp.Layers.Ground)
	sparse("coast", mp.Layers.Coast)
	sparse("plateau_shadow", mp.Layers.PlateauShadow)
	dense("plateau", "plateau", mp.Layers.Plateau)
	sparse("cliff", mp.Layers.Cliff)
	sparse("stairs", mp.Layers.Stairs)

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	fmt.Fprintf(&b, `<map version="1.10" tiledversion="1.12.2" orientation="orthogonal" `+
		`renderorder="right-down" width="%d" height="%d" tilewidth="%d" tileheight="%d" `+
		`infinite="0" nextlayerid="%d" nextobjectid="1">`+"\n",
		W, H, mp.TileSize, mp.TileSize, len(layers)+1)
	for _, s := range mp.Sheets {
		// путь к .tsx относительно каталога, куда кладём .tmx
		tsx := strings.TrimSuffix(s.File, filepath.Ext(s.File)) + ".tsx"
		rel, err := filepath.Rel(outDir, filepath.Join(biomeDir, tsx))
		if err != nil {
			rel = filepath.Join(biomeDir, tsx)
		}
		fmt.Fprintf(&b, ` <tileset firstgid="%d" source="%s"/>`+"\n", s.Firstgid, rel)
	}
	for i, l := range layers {
		fmt.Fprintf(&b, ` <layer id="%d" name="%s" width="%d" height="%d">`+"\n", i+1, l.name, W, H)
		b.WriteString(`  <data encoding="csv">` + "\n")
		for y := 0; y < H; y++ {
			row := make([]string, W)
			for x := 0; x < W; x++ {
				row[x] = itoa(int(l.data[y*W+x]))
			}
			b.WriteString(strings.Join(row, ","))
			if y < H-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.WriteString("</data>\n </layer>\n")
	}
	b.WriteString("</map>\n")
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
