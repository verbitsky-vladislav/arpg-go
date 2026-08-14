package worldgen

// plateautest.go — отладочный режим: одно чистое мини-плато на травяном поле,
// чтобы быстро итерировать раскладку скального обрыва (углы, стенка, подножие),
// не гоняя всю карту. Запуск: worldgen plateautest [biome-dir].

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
)

func RunPlateauTest(args []string) {
	biomeDir := "assets/biomes/forest"
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		biomeDir = args[0]
	}
	m, err := LoadManifest(biomeDir)
	if err != nil {
		fatal(err)
	}
	W, H := 22, 16
	p := defaultParams(W, H)
	g := NewGenerator(p, 1, m)
	g.Level = NewGrid[Level](W, H)
	g.Moisture = NewGrid[float64](W, H)
	for i := range g.Level.Data {
		g.Level.Data[i] = Ground
	}
	// плато с ПЛОСКИМ низом (чистая южная лента обрыва): скруглённый верх +
	// ровная нижняя кромка + вертикальные бока, со ступенчатым заходом снизу,
	// чтобы проверить правило подножия (ровно / ступень / конец линии).
	top, bot := 5, 10 // ряды верх..низ плато
	for y := top; y <= bot; y++ {
		// горизонтальный размах: узкий сверху (скругление), полный ниже
		inset := 0
		if y == top {
			inset = 4
		} else if y == top+1 {
			inset = 2
		}
		// ступеньки на нижней кромке (проверка углов подножия)
		l, r := 4+inset, W-5-inset
		if y == bot {
			l, r = 6, W-7 // низ у́же → ступенька вверх по бокам
		}
		for x := l; x <= r; x++ {
			g.Level.Set(x, y, Plateau)
		}
	}
	_ = math.Sqrt

	g.stagePlateauApron()
	g.Trail = map[[2]int]bool{}
	g.GroundLayer = NewGrid[uint16](W, H)
	g.MudLayer = NewGrid[uint16](W, H)
	isLand := func(x, y int) bool { return g.Level.In(x, y) && g.Level.At(x, y).isLand() }
	g.paintLand(isLand, "ground", "ground", "grass_water", "coast", g.GroundLayer)
	g.stagePlateau()
	g.stagePlateauShadow()

	mp := g.ToMapV1(1)
	atlas, err := NewAtlasSet(m)
	if err != nil {
		fatal(err)
	}
	outDir := filepath.Join("out", "worldgen", m.ID)
	_ = os.MkdirAll(outDir, 0o755)
	name := filepath.Join(outDir, "plateautest.png")
	if err := writePNG(name, RenderMap(mp, atlas, m.RenderScale*2)); err != nil {
		fatal(err)
	}
	fmt.Printf("мини-плато → %s (%dx%d)\n", name, W, H)
}
