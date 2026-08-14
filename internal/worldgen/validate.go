package worldgen

// validate.go — проверки E1..E12 (worldgen.spec §8) с фактическими значениями.
// Пишет человекочитаемый отчёт. E3/E4 (недостижимое/узкое плато) — ключевые:
// они не видны на скрине, но ломают игру.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

	// разбор возвышенностей по кускам: тип, размер макушки, наличие подъёма
	pl, pn := components(g.Level, func(l Level) bool { return l == Plateau })
	psize := componentSizes(pl, pn)
	pkind := make([]plateauKind, pn+1)
	for i, id := range pl.Data {
		if id > 0 {
			pkind[id] = plateauKind(g.Kind.Data[i])
		}
	}
	reach := make([]bool, pn+1)
	for c := range g.Stair {
		// лестница стоит НИЖЕ кромки, поэтому кусок ищем над её верхним рядом
		for dy := 1; dy <= maxCliffDepth+1; dy++ {
			if id := pl.AtOr(c[0], c[1]-dy, 0); id > 0 {
				reach[id] = true
				break
			}
		}
	}

	// E3: подъём есть ровно там, где его требует тип. Без лестницы верх —
	// декорация: тело обрыва в nav стоит стеной, обойти его негде; лишняя
	// лестница на декоративном высоком плато — тоже ошибка.
	noStairs, extraStairs := 0, 0
	for id := 1; id <= pn; id++ {
		switch want := specOf(pkind[id]).stairs; {
		case want && !reach[id]:
			noStairs++
		case !want && reach[id]:
			extraStairs++
		}
	}
	add("E3", "подъёмы по типу плато", noStairs == 0 && extraStairs == 0,
		fmt.Sprintf("без нужного подъёма: %d, лишних: %d, кусков: %d", noStairs, extraStairs, pn))

	// E14: типы возвышенностей соответствуют спецификации — количество кусков в
	// заданном коридоре, макушка не больше предела типа.
	var gaps []string
	for _, sp := range plateauSpecs {
		cnt, over := 0, 0
		for id := 1; id <= pn; id++ {
			if pkind[id] != sp.kind {
				continue
			}
			cnt++
			if psize[id] > sp.capCells {
				over++
			}
		}
		if cnt < sp.min || cnt > sp.max {
			gaps = append(gaps, fmt.Sprintf("%s: %d шт (нужно %d..%d)", sp.name, cnt, sp.min, sp.max))
		}
		if over > 0 {
			gaps = append(gaps, fmt.Sprintf("%s: %d макушек больше %d клеток", sp.name, over, sp.capCells))
		}
	}
	add("E14", "типы плато по спецификации", len(gaps) == 0, strings.Join(gaps, "; "))

	// E4: на макушке нет лент и усов тоньше трёх клеток. Прямоугольность здесь
	// не проверяется намеренно: силуэт должен быть округлым и неровным, важно
	// лишь отсутствие полосок, на которых не встаёт ни обрыв, ни автотайл.
	narrow := plateauThin(g.Level)
	add("E4", "макушка без лент тоньше 3 тайлов", narrow == 0, fmt.Sprintf("узких клеток: %d", narrow))

	// E5: спавн существует и стоит на земле
	e5 := false
	for _, mk := range mp.Markers {
		if mk.Kind == "spawn" && g.Level.At(mk.X, mk.Y).isLand() {
			e5 = true
		}
	}
	add("E5", "точка появления на суше", e5, "")

	// E6: доля суши 45..70% от полезной площади. Считаем не от всего массива:
	// островная маска высоты держит сушу в круге, и от массива физический
	// максимум — π/4≈78.5%, порог был недостижим по построению.
	lf := float64(land) / g.usableArea() * 100
	add("E6", "доля суши 45..70% (от площади острова)", lf >= 45 && lf <= 70, fmt.Sprintf("%.1f%%", lf))

	// E7: доля плато от суши — теперь справочная величина, а не порог. Раньше
	// требовалось 10..30%, но при явном размещении кусков площадь возвышенностей
	// задана лимитами типов (E14), и старый коридор ей противоречит.
	pf := 0.0
	if land > 0 {
		pf = float64(plateau) / float64(land) * 100
	}
	add("E7", "доля плато от суши (справочно)", true, fmt.Sprintf("%.1f%%", pf))

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

	// E9: тело обрыва целиком лежит на нижней земле (иначе скала висит над
	// водой или над другим плато и линия обрыва рвётся)
	badApron := 0
	for c := range g.Cliff {
		if !g.Level.In(c[0], c[1]) || g.Level.At(c[0], c[1]) != Ground {
			badApron++
		}
	}
	add("E9", "обрыв стоит на нижней земле", badApron == 0, fmt.Sprintf("висящих клеток: %d", badApron))

	// E13: покрытие угловых наборов — сколько раз точного ключа не нашлось и
	// пришлось ставить ближайший тайл. Это и есть «кривые стыки» на картинке.
	miss, keys := 0, make([]string, 0, len(g.cornerMiss))
	for set, n := range g.cornerMiss {
		miss += n
		keys = append(keys, fmt.Sprintf("%s×%d", set, n))
	}
	sort.Strings(keys)
	add("E13", "угловые наборы покрывают все ключи", miss == 0, strings.Join(keys, ", "))

	// E15: макушки, на которые положен подъём, связаны с точкой появления. E3
	// проверяет, что лестница построена, а эта — что по ней действительно
	// доходишь: тем же обходом и по тем же правилам, что у физики в игре.
	unreach, plateauCells := g.navReach(mp)
	add("E15", "на плато со ступенями можно подняться", unreach == 0,
		fmt.Sprintf("отрезано клеток: %d из %d", unreach, plateauCells))

	// E11: обязательные роли манифеста закрыты
	roleGaps := g.Manifest.validateRoles()
	add("E11", "роли манифеста закрыты", len(roleGaps) == 0, strings.Join(roleGaps, "; "))

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
