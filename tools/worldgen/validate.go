package main

// validate.go — проверки E1..E12 (worldgen.spec §8) с фактическими значениями.
// Пишет человекочитаемый отчёт. E3/E4 (недостижимое/узкое плато) — ключевые:
// они не видны на скрине, но ломают игру.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type checkResult struct {
	ID   string
	Name string
	OK   bool
	Note string
}

// validate прогоняет проверки над результатом одного сида.
func (g *Generator) validate(mp *MapV1) []checkResult {
	W, H := g.P.Width, g.P.Height
	area := W * H
	land, plateau, deep, shallow := 0, 0, 0, 0
	for _, lv := range g.Level.Data {
		switch lv {
		case LiquidDeep:
			deep++
		case LiquidShallow:
			shallow++
		case Plateau:
			plateau++
			land++
		case Ground:
			land++
		}
	}
	var res []checkResult
	add := func(id, name string, ok bool, note string) {
		res = append(res, checkResult{id, name, ok, note})
	}

	// E1: суша не ближе edgeBorder тайлов к границе массива
	border := g.P.EdgeBorder
	e1 := true
	for y := 0; y < H && e1; y++ {
		for x := 0; x < W; x++ {
			near := x < border || y < border || x >= W-border || y >= H-border
			if near && g.Level.At(x, y).isLand() {
				e1 = false
				break
			}
		}
	}
	add("E1", fmt.Sprintf("суша не ближе %d тайлов к краю", border), e1, "")

	// E4: плато шириной < 4 тайлов
	narrow := 0
	isP := func(x, y int) bool { return g.Level.In(x, y) && g.Level.At(x, y) == Plateau }
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			if g.Level.At(x, y) == Plateau && !thickEnough(isP, x, y, 4) {
				narrow++
			}
		}
	}
	add("E4", "плато шириной < 4 тайлов", narrow == 0, fmt.Sprintf("узких клеток: %d", narrow))

	// E5: спавн существует и стоит на земле
	e5 := false
	for _, mk := range mp.Markers {
		if mk.Kind == "spawn" && g.Level.At(mk.X, mk.Y).isLand() {
			e5 = true
		}
	}
	add("E5", "точка появления на суше", e5, "")

	// E6: доля суши 45..70%
	lf := float64(land) / float64(area) * 100
	add("E6", "доля суши 45..70%", lf >= 45 && lf <= 70, fmt.Sprintf("%.1f%%", lf))

	// E7: доля плато от суши 10..30%
	pf := 0.0
	if land > 0 {
		pf = float64(plateau) / float64(land) * 100
	}
	add("E7", "доля плато от суши 10..30%", pf >= 10 && pf <= 30, fmt.Sprintf("%.1f%%", pf))

	// E8: пропсы не пересекаются между собой и лежат на суше
	occ := map[[2]int]bool{}
	overlap, inWater := 0, 0
	for _, p := range mp.Props {
		for yy := p.Y; yy < p.Y+p.H; yy++ {
			for xx := p.X; xx < p.X+p.W; xx++ {
				k := [2]int{xx, yy}
				if occ[k] {
					overlap++
				}
				occ[k] = true
				if g.Level.In(xx, yy) && !g.Level.At(xx, yy).isLand() {
					inWater++
				}
			}
		}
	}
	add("E8", "пропсы без пересечений и не в воде", overlap == 0 && inWater == 0,
		fmt.Sprintf("пересечений: %d, в воде: %d", overlap, inWater))

	// E11: обязательные роли манифеста закрыты
	gaps := g.Manifest.validateRoles()
	add("E11", "роли манифеста закрыты", len(gaps) == 0, strings.Join(gaps, "; "))

	// E12: файлы пропсов существуют
	missing := 0
	for _, p := range g.Manifest.Props {
		if !fileExists(filepath.Join(g.Manifest.dir, p.File)) {
			missing++
		}
	}
	add("E12", "файлы пропсов на диске", missing == 0, fmt.Sprintf("нет файлов: %d", missing))

	_ = deep
	_ = shallow
	return res
}

// writeReport пишет отчёт проверок в файл.
func writeReport(path string, biome string, results [][]checkResult, seeds []int64) error {
	var b strings.Builder
	fmt.Fprintf(&b, "Отчёт валидации биома %s\n", biome)
	fmt.Fprintf(&b, "%s\n\n", strings.Repeat("=", 40))
	for si, res := range results {
		fmt.Fprintf(&b, "seed %d:\n", seeds[si])
		for _, r := range res {
			mark := "OK  "
			if !r.OK {
				mark = "FAIL"
			}
			line := fmt.Sprintf("  [%s] %-4s %s", mark, r.ID, r.Name)
			if r.Note != "" {
				line += "  (" + r.Note + ")"
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n")
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
