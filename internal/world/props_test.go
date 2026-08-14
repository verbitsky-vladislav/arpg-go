package world

import (
	"os"
	"testing"

	"github.com/vladislav/game/internal/assets"
	"github.com/vladislav/game/internal/engine"
	"github.com/vladislav/game/internal/physics"
	"github.com/vladislav/game/internal/worldgen"
)

// TestPropsLoadedAndSorted — объекты карты доезжают до рантайма и готовы к
// отрисовке. Проверка нужна ровно потому, что этого шага долго не было: объекты
// стояли в сетке физики (игрок в них упирался), а нарисовать их было нечем.
func TestPropsLoadedAndSorted(t *testing.T) {
	l := assets.NewLoader(os.DirFS("../../assets"))
	man, err := worldgen.LoadManifestFS(l.FS(), "biomes/forest")
	if err != nil {
		t.Skipf("нет биома forest: %v", err)
	}
	mv := worldgen.Generate(man, 1, 96)
	if len(mv.Props) == 0 {
		t.Skip("генератор не поставил объектов")
	}
	m, err := New(l, mv)
	if err != nil {
		t.Fatalf("карта не собралась: %v", err)
	}
	if m.PropCount() != len(mv.Props) {
		t.Errorf("загружено %d объектов из %d — часть спрайтов не нашлась",
			m.PropCount(), len(mv.Props))
	}
	// порядок отрисовки: по возрастанию глубины, иначе дальнее перекроет ближнее
	for i := 1; i < m.PropCount(); i++ {
		if m.PropDepth(i) < m.PropDepth(i-1) {
			t.Fatalf("объекты не отсортированы по глубине на позиции %d", i)
		}
	}
	// среди объектов forest есть покачивающиеся деревья — их полосы кадров
	// обязаны читаться как обычные PNG
	anim := 0
	for _, p := range mv.Props {
		if p.Frames > 1 {
			anim++
		}
	}
	t.Logf("объектов %d, из них анимированных %d", m.PropCount(), anim)
}

// TestPropBarrierUnderSprite — барьер объекта стоит ТАМ ЖЕ, где нарисован его
// ствол, а не рядом.
//
// Проверка появилась по живому багу: объект приходит из карты в клеточных
// координатах, а содержимое клетки нарисовано со сдвигом на полтайла (он запечён
// в сетку физики). Без поправки сквозь пень можно было пройти насквозь, зато
// справа-снизу от него стояла невидимая стена, касавшаяся спрайта углом.
func TestPropBarrierUnderSprite(t *testing.T) {
	l := assets.NewLoader(os.DirFS("../../assets"))
	man, err := worldgen.LoadManifestFS(l.FS(), "biomes/forest")
	if err != nil {
		t.Skipf("нет биома forest: %v", err)
	}
	mv := worldgen.Generate(man, 3, 96)
	m, err := New(l, mv)
	if err != nil {
		t.Fatalf("карта не собралась: %v", err)
	}
	ts := float64(mv.TileSize)
	checked, bad := 0, 0
	for i, p := range mv.Props {
		if !p.Collides {
			continue
		}
		checked++
		// середина нарисованной клетки якоря — то, чем объект стоит на земле
		base := engine.Vec2{
			X: m.PropX(i) + float64(p.W)*ts/2,
			Y: m.PropDepth(i) - ts/2,
		}
		// барьер обязан накрывать не точку, а ширину тела: проверяем центр и обе
		// стороны на трети полуширины — иначе «стена в одну клетку» под деревом
		// снова пройдёт проверку, а игрок будет обходить ствол по краю
		off := float64(p.Body) / 2 / 3
		for _, dx := range []float64{-off, 0, off} {
			q := engine.Vec2{X: base.X + dx, Y: base.Y}
			if m.Field().CellAt(q) != physics.Solid {
				bad++
				if bad <= 3 {
					t.Errorf("%s (тело %d px): в (%.0f,%.0f) не стена, а %v",
						p.ID, p.Body, q.X, q.Y, m.Field().CellAt(q))
				}
				break
			}
		}
	}
	if checked == 0 {
		t.Skip("непроходимых объектов на карте нет")
	}
	if bad != 0 {
		t.Fatalf("барьер разошёлся со спрайтом у %d объектов из %d", bad, checked)
	}
	t.Logf("сверено объектов: %d", checked)
}
