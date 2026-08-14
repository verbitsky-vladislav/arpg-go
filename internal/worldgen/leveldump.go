package worldgen

// leveldump.go — режим `worldgen level <biome-dir> [seed] [size]`: выгрузить
// СЕТКУ УРОВНЕЙ карты текстом, не трогая .tmx. Нужен, когда карту в Tiled уже
// правят руками: по .tmx форму макушки не восстановить (dual-grid рисует тайлы
// на клетку шире региона), а для разбора раскладки обрыва она обязательна.
//
// Обозначения: P — макушка плато, r — клетка тела обрыва, . — нижняя земля,
// ~ — мель, # — глубина, s — лестница.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func RunLevelDump(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "worldgen level <biome-dir> [seed] [size]")
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
	// сид считается так же, как в основном прогоне (RunGenerate), иначе
	// выгрузка окажется другой картой, чем seed_N.png
	g := NewGenerator(defaultParams(size, size), uint64(seed)*0x9E3779B97F4A7C15, m)
	g.Run()

	var b strings.Builder
	fmt.Fprintf(&b, "# уровни карты %s, сид %d, %dx%d\n", m.ID, seed, size, size)
	b.WriteString("# P макушка, r тело обрыва, s лестница, . земля, ~ мель, # глубина\n")
	for y := 0; y < g.P.Height; y++ {
		for x := 0; x < g.P.Width; x++ {
			ch := '.'
			switch {
			case g.Stair[[2]int{x, y}]:
				ch = 's'
			case g.Cliff[[2]int{x, y}]:
				ch = 'r'
			case g.Level.At(x, y) == Plateau:
				ch = 'P'
			case g.Level.At(x, y) == LiquidShallow:
				ch = '~'
			case g.Level.At(x, y) == LiquidDeep:
				ch = '#'
			}
			b.WriteRune(ch)
		}
		b.WriteByte('\n')
	}
	outDir := filepath.Join("out", "worldgen", m.ID)
	_ = os.MkdirAll(outDir, 0o755)
	name := filepath.Join(outDir, fmt.Sprintf("seed_%d.level.txt", seed))
	if err := os.WriteFile(name, []byte(b.String()), 0o644); err != nil {
		fatal(err)
	}
	fmt.Printf("уровни → %s\n", name)
}
