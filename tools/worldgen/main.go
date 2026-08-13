// Инструмент worldgen — общий процедурный генератор карт биомов.
// Читает manifest.json биома, прогоняет пайплайн шума→уровней→тайлов→объектов
// (worldgen.spec §5) и композитит результат в PNG на настоящем арте.
// Только stdlib, не зависит от игрового бинарника.
//
// Запуск:
//
//	go run ./tools/worldgen <biome-dir> [-seeds 6] [-size 256] [-out dir]
//	go run ./tools/worldgen audit        # проверить все биомы (роли/файлы)
//	go run ./tools/worldgen index <biome-dir> <sheet>  # атлас с наложенными id
//	go run ./tools/worldgen tmxhint <biome-dir>        # черновик blob-наборов из .tmx
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "audit":
		runAudit(os.Args[2:])
	case "index":
		runIndex(os.Args[2:])
	case "tmxhint":
		runTMXHint(os.Args[2:])
	case "plateautest":
		runPlateauTest(os.Args[2:])
	case "ref":
		runRef(os.Args[2:])
	case "tiles":
		runTilesDump(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		runGenerate(os.Args[1:])
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `worldgen — генератор карт биомов

  worldgen <biome-dir> [-seeds N] [-size N] [-out dir]
      сгенерировать карты биома (по умолчанию 6 сидов, размер 256, out=out/worldgen/<id>)
  worldgen audit
      проверить все assets/biomes/* на наличие манифеста и обязательных ролей/файлов
  worldgen index <biome-dir> <sheet>
      выгрузить атлас листа с наложенными номерами тайлов (authoring-помощник)
  worldgen tmxhint <biome-dir>
      напечатать черновик blob/edge-наборов, снятый с примера .tmx (подсказка)
`)
}

// runGenerate — основной режим: сгенерировать N карт биома.
func runGenerate(args []string) {
	fs := flag.NewFlagSet("generate", flag.ExitOnError)
	seeds := fs.Int("seeds", 6, "число сидов (карт)")
	size := fs.Int("size", 256, "сторона карты в тайлах")
	out := fs.String("out", "", "каталог вывода (по умолчанию out/worldgen/<id>)")
	scale := fs.Int("scale", 0, "масштаб рендера (0 = render_scale из манифеста)")
	// flag.Parse останавливается на первом позиционном аргументе, поэтому
	// сначала вытаскиваем каталог биома, а флаги (в любом порядке) — из остатка.
	biomeDir, rest := splitPositional(args)
	if biomeDir == "" {
		fmt.Fprintln(os.Stderr, "укажите каталог биома, напр. assets/biomes/forest")
		os.Exit(2)
	}
	_ = fs.Parse(rest)

	m, err := LoadManifest(biomeDir)
	if err != nil {
		fatal(err)
	}
	if gaps := m.validateRoles(); len(gaps) > 0 {
		fmt.Fprintf(os.Stderr, "манифест %s неполон:\n", m.ID)
		for _, g := range gaps {
			fmt.Fprintln(os.Stderr, "  -", g)
		}
		os.Exit(1)
	}

	outDir := *out
	if outDir == "" {
		outDir = filepath.Join("out", "worldgen", m.ID)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fatal(err)
	}

	rscale := m.RenderScale
	if *scale > 0 {
		rscale = *scale
	}

	atlas, err := NewAtlasSet(m)
	if err != nil {
		fatal(err)
	}

	p := defaultParams(*size, *size)
	p.EdgeMode = m.EdgeMode

	var allChecks [][]checkResult
	var seedIDs []int64
	for i := 1; i <= *seeds; i++ {
		seed := uint64(i) * 0x9E3779B97F4A7C15
		g := NewGenerator(p, seed, m)
		g.Run()

		// отладочные поля — только для первого сида, чтобы не плодить файлы
		if i == 1 {
			_ = writePNG(filepath.Join(outDir, "debug_height.png"), grayField(g.Height, 2))
			_ = writePNG(filepath.Join(outDir, "debug_moisture.png"), grayField(g.Moisture, 2))
			_ = writePNG(filepath.Join(outDir, "debug_levels.png"), levelField(g.Level, 2))
		}

		mp := g.ToMapV1(int64(i))
		img := RenderMap(mp, atlas, rscale)
		name := filepath.Join(outDir, fmt.Sprintf("seed_%d.png", i))
		if err := writePNG(name, img); err != nil {
			fatal(err)
		}
		// map.json — для первого сида (образец формата)
		if i == 1 {
			if err := writeMapJSON(filepath.Join(outDir, "map.json"), mp); err != nil {
				fatal(err)
			}
		}
		allChecks = append(allChecks, g.validate(mp))
		seedIDs = append(seedIDs, int64(i))
		fmt.Printf("seed %d → %s (%dx%d тайлов, пропсов %d)\n", i, name, mp.Width, mp.Height, len(mp.Props))
	}
	if err := writeReport(filepath.Join(outDir, "report.txt"), m.ID, allChecks, seedIDs); err != nil {
		fatal(err)
	}
	fmt.Printf("готово: %d карт в %s (map.json, report.txt рядом)\n", *seeds, outDir)
}

// splitPositional вынимает первый непфлаговый аргумент (каталог биома), возвращая
// его и остальные аргументы (флаги в любом порядке относительно позиционного).
func splitPositional(args []string) (string, []string) {
	pos := ""
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		if pos == "" && len(a) > 0 && a[0] != '-' {
			pos = a
			continue
		}
		rest = append(rest, a)
	}
	return pos, rest
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "ошибка:", err)
	os.Exit(1)
}
