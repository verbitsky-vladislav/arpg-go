package worldgen

import "testing"

// TestPropsPlacementRules — правила размещения объектов держатся на всех сидах:
// якорь не в воде, спрайт крупного не лезет на тропу, ничего не жмётся к кромке
// плато, и непроходимое не стоит у лестницы — ни телом, ни кроной.
// Проверка изнутри пакета: снаружи, по map.json, тропу и кромку не увидеть.
func TestPropsPlacementRules(t *testing.T) {
	m, err := LoadManifest("../../assets/biomes/forest")
	if err != nil {
		t.Skipf("нет манифеста forest: %v", err)
	}
	if len(m.Props) == 0 {
		t.Skip("в манифесте нет объектов")
	}
	total := 0
	for seed := uint64(1); seed <= 8; seed++ {
		p := defaultParams(128, 128)
		p.EdgeMode = m.EdgeMode
		g := NewGenerator(p, seed, m)
		g.Run()
		water, trail, edge, stair := g.auditProps()
		total += len(g.Props)
		if water != 0 || trail != 0 || edge != 0 || stair != 0 {
			t.Errorf("сид %d: в воде %d, на тропе %d, у кромки плато %d, у лестницы %d",
				seed, water, trail, edge, stair)
		}
	}
	if total == 0 {
		t.Fatal("объекты не расставлены ни на одном сиде")
	}
	t.Logf("проверено объектов: %d", total)
}
