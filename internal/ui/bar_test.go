package ui_test

import (
	"os"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/vladislav/game/internal/assets"
	"github.com/vladislav/game/internal/ui"
)

func loadBars(t *testing.T) {
	t.Helper()
	l := assets.NewLoader(os.DirFS("../../assets"))
	if err := ui.InitBars(l, "ui/bars"); err != nil {
		t.Fatalf("полосы не грузятся: %v", err)
	}
}

// TestBarsLoad — разметка полос размечена руками (автоматика склеивает рамку с
// заливкой), поэтому ошибиться в координатах легко, а видно это только в игре.
// Проверяем то, что можно проверить без глаз: стили есть, слот лежит внутри
// рамки, рисование не падает на краях диапазона.
func TestBarsLoad(t *testing.T) {
	loadBars(t)
	ids := ui.BarIDs()
	if len(ids) == 0 {
		t.Fatal("ни одного стиля полосы")
	}
	dst := ebiten.NewImage(320, 64)
	for _, id := range ids {
		w, h := ui.BarSize(id)
		if w <= 0 || h <= 0 {
			t.Errorf("%s: пустой размер рамки %dx%d", id, w, h)
		}
		if ui.BarTitle(id) == "" {
			t.Errorf("%s: нет подписи для настроек", id)
		}
		dx, dy, sw, sh, ok := ui.BarSlot(id)
		if !ok {
			t.Errorf("%s: нет слота под заливку", id)
			continue
		}
		if dx < 0 || dy < 0 || dx+sw > w || dy+sh > h {
			t.Errorf("%s: слот (%d,%d %dx%d) вылезает за рамку %dx%d", id, dx, dy, sw, sh, w, h)
		}
		for _, frac := range []float64{-1, 0, 0.37, 1, 2} {
			if !ui.DrawBar(dst, id, 4, 4, frac) {
				t.Errorf("%s: DrawBar отказался рисовать при доле %v", id, frac)
			}
		}
	}
}

// TestBarUnknownStyleFallsBack — в настройках может лежать id из старой сборки
// или пустая строка у свежего профиля. Полоса здоровья не то место, где стоит
// пропадать: неизвестный стиль должен молча стать стилем по умолчанию.
func TestBarUnknownStyleFallsBack(t *testing.T) {
	loadBars(t)
	for _, id := range []string{"", "нет-такого-стиля"} {
		if w, _ := ui.BarSize(id); w <= 0 {
			t.Errorf("стиль %q не свёлся к дефолтному", id)
		}
		if !ui.DrawBar(ebiten.NewImage(320, 64), id, 0, 0, 0.5) {
			t.Errorf("стиль %q: нечем рисовать", id)
		}
	}
}
