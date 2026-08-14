package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/vladislav/game/internal/config"
)

// TestKeepCursorHiddenLeavesForeignCursor — пока окна нет (или оно не в
// фокусе), курсор чужой: игра его не трогает. Проверка именно этой сдержанности
// — прятать системный курсор поверх других окон нельзя.
func TestKeepCursorHiddenLeavesForeignCursor(t *testing.T) {
	ebiten.SetCursorMode(ebiten.CursorModeVisible)
	for range 5 * config.TPS { // дольше разогрева
		KeepCursorHidden(config.ScreenW, config.ScreenH)
	}
	if got := ebiten.CursorMode(); got != ebiten.CursorModeVisible {
		t.Errorf("режим курсора стал %v, а окна в фокусе нет — трогать его было нечего", got)
	}
}
