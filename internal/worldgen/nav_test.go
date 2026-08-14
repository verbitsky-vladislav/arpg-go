package worldgen

// nav_test.go — сетка физики против готовой карты. Проверяется главное её
// свойство: она описывает то, что НАРИСОВАНО, а не то, что лежит в сетке
// уровней. Эти две вещи разъезжаются на полтайла (dual-grid), и именно из-за
// этого расхождения звери ходили по воде.

import (
	"testing"

	"github.com/vladislav/game/internal/physics"
)

const biomeDir = "../../assets/biomes/forest"

// testSide — сторона тестовых карт в тайлах. Меньше боевых 128, чтобы тесты шли
// быстро, но достаточно, чтобы на карте были и вода, и плато с лестницами.
const testSide = 96

// testMap — карта биома forest на сиде seed вместе с генератором, который её
// сделал: navReach смотрит в g.Kind, чтобы отличить декоративную скалу от той,
// на которую обязан быть подъём.
func testMap(t *testing.T, seed uint64) (*Generator, *MapV1) {
	t.Helper()
	m, err := LoadManifest(biomeDir)
	if err != nil {
		t.Skipf("нет биома forest: %v", err)
	}
	p := defaultParams(testSide, testSide)
	p.EdgeMode = m.EdgeMode
	g := NewGenerator(p, seed, m)
	g.Run()
	return g, g.ToMapV1(int64(seed))
}

// navAt — клетка физики под под-клеткой (sx,sy).
func navAt(mp *MapV1, sx, sy int) physics.Cell {
	return physics.Cell(mp.Nav.Cells[sy*mp.Nav.Width+sx])
}

// TestNavMatchesDrawnTiles — под каждой под-клеткой лежит то, что в этом месте
// нарисовано. Проверка через слои тайлов, а не через g.Level: слой — это уже
// картинка, и сойтись с ним сетка может, только если сдвиг учтён.
//
// Тайл, накрывающий под-клетку, — это `sx/scale`. Если под-клетка размечена как
// макушка, в этом тайле обязан быть тайл слоя plateau: клетка-макушка приходится
// ему углом, а значит, кусок плато он рисует. При старом (несдвинутом) чтении
// это ломалось по всей кромке возвышенности.
func TestNavMatchesDrawnTiles(t *testing.T) {
	_, mp := testMap(t, 3)
	n := mp.Nav
	if n.Scale != navScale || n.Width != mp.Width*navScale {
		t.Fatalf("сетка %dx%d scale %d при карте %dx%d", n.Width, n.Height, n.Scale, mp.Width, mp.Height)
	}

	// Берег рисуется не плотным слоем, а разрежённым coast (переходные тайлы
	// уезжают на лист Water_coasts), поэтому клетки суши у воды надо искать там.
	coast := map[int]bool{}
	for _, c := range mp.Layers.Coast {
		coast[c.Y*mp.Width+c.X] = true
	}

	plateauMiss, groundMiss, checked := 0, 0, 0
	for sy := range n.Height {
		for sx := range n.Width {
			tx, ty := sx/n.Scale, sy/n.Scale
			ti := ty*mp.Width + tx
			switch navAt(mp, sx, sy) {
			case physics.Plateau:
				checked++
				if mp.Layers.Plateau[ti] == 0 {
					plateauMiss++
				}
			case physics.Ground:
				checked++
				// Земля нарисована травой или тропой; под кромкой плато лежит
				// его тень, а сам тайл земли там всё равно есть.
				if mp.Layers.Ground[ti] == 0 && mp.Layers.Mud[ti] == 0 && !coast[ti] {
					groundMiss++
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("на карте не оказалось ни земли, ни плато")
	}
	if plateauMiss > 0 || groundMiss > 0 {
		t.Errorf("сетка разошлась с картинкой: макушек мимо тайла %d, земли мимо тайла %d (из %d)",
			plateauMiss, groundMiss, checked)
	}
}

// TestNavStairsConnectFloors — каждая лестница связывает этажи: сверху к ней
// примыкает макушка, снизу — нижняя земля. Лестница, которая никуда не ведёт,
// в отчёте не видна (E3 проверяет, что она построена, а не что по ней доходишь).
func TestNavStairsConnectFloors(t *testing.T) {
	for _, seed := range []uint64{1, 2, 3} {
		_, mp := testMap(t, seed)
		n := mp.Nav
		var ramps, connected int
		seen := map[[2]int]bool{}
		for sy := range n.Height {
			for sx := range n.Width {
				if navAt(mp, sx, sy) != physics.Ramp || seen[[2]int{sx / navScale, sy / navScale}] {
					continue
				}
				seen[[2]int{sx / navScale, sy / navScale}] = true
				ramps++
				up, down := false, false
				for d := 1; d <= 4*navScale && !(up && down); d++ {
					if sy-d >= 0 {
						switch navAt(mp, sx, sy-d) {
						case physics.Plateau:
							up = true
						case physics.Ramp: // тело лестницы, идём дальше
						default:
							d = 99
						}
					}
				}
				for d := 1; d <= 4*navScale && !down; d++ {
					if sy+d < n.Height {
						switch navAt(mp, sx, sy+d) {
						case physics.Ground, physics.Shallow:
							down = true
						case physics.Ramp:
						default:
							d = 99
						}
					}
				}
				if up && down {
					connected++
				}
			}
		}
		if ramps == 0 {
			t.Errorf("сид %d: на карте нет ни одной лестницы", seed)
		}
		if connected != ramps {
			t.Errorf("сид %d: лестниц %d, связывают этажи %d", seed, ramps, connected)
		}
	}
}

// TestNavPlateauReachable — на каждую макушку, которой положен подъём, можно
// попасть от точки появления. Та же проверка, что E15 в отчёте: она ловит и
// пропс, севший на нижнюю ступень, и лестницу, упёршуюся в воду.
func TestNavPlateauReachable(t *testing.T) {
	for _, seed := range []uint64{1, 2, 3, 4, 5} {
		g, mp := testMap(t, seed)
		un, total := g.navReach(mp)
		if total == 0 {
			continue // на карте нет плато со ступенями
		}
		if un > 0 {
			t.Errorf("сид %d: отрезано %d клеток макушек из %d", seed, un, total)
		}
	}
}
