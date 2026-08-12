// Package settings — пользовательские настройки, сохраняемые между запусками.
// Единый источник правды: load → apply (ebiten/ui) → save при каждом изменении.
// Файл лежит в пользовательском конфиг-каталоге ОС (fallback — текущий каталог).
package settings

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/ui"
)

// Settings — сохраняемые параметры.
type Settings struct {
	Fullscreen  bool `json:"fullscreen"`
	VSync       bool `json:"vsync"`
	WindowScale int  `json:"window_scale"` // множитель логического разрешения (2..4)
	ShowFPS     bool `json:"show_fps"`
}

func defaults() Settings {
	return Settings{
		Fullscreen:  false,
		VSync:       true,
		WindowScale: config.WindowW / config.ScreenW, // как в config (2×)
		ShowFPS:     false,
	}
}

var current = defaults()

// Get — копия текущих настроек (для отображения в UI).
func Get() Settings { return current }

// filePath — путь к файлу настроек.
func filePath() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		return "settings.json" // fallback — рядом с бинарником
	}
	return filepath.Join(dir, "pixel-arpg", "settings.json")
}

// Init загружает настройки с диска (или дефолты) и применяет их. Вызывать один
// раз при старте (после создания окна).
func Init() {
	current = load()
	apply()
}

func load() Settings {
	s := defaults()
	b, err := os.ReadFile(filePath())
	if err != nil {
		return s // нет файла → дефолты
	}
	_ = json.Unmarshal(b, &s) // битый JSON → оставляем дефолты
	if s.WindowScale < 1 || s.WindowScale > 4 {
		s.WindowScale = defaults().WindowScale
	}
	return s
}

func save() {
	p := filePath()
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	if b, err := json.MarshalIndent(current, "", "  "); err == nil {
		_ = os.WriteFile(p, b, 0o644)
	}
}

// apply проталкивает текущие настройки в движок и UI.
func apply() {
	ebiten.SetFullscreen(current.Fullscreen)
	ebiten.SetVsyncEnabled(current.VSync)
	if !current.Fullscreen {
		ebiten.SetWindowSize(current.WindowScale*config.ScreenW, current.WindowScale*config.ScreenH)
	}
	ui.ShowFPS = current.ShowFPS
}

// --- переключатели (применяют и сохраняют) ---

func ToggleFullscreen() { current.Fullscreen = !current.Fullscreen; apply(); save() }
func ToggleVSync()      { current.VSync = !current.VSync; apply(); save() }
func ToggleFPS()        { current.ShowFPS = !current.ShowFPS; apply(); save() }

// CycleWindowScale переключает масштаб окна 2→3→4→2 (в оконном режиме).
func CycleWindowScale() {
	if current.Fullscreen {
		return
	}
	current.WindowScale++
	if current.WindowScale > 4 {
		current.WindowScale = 2
	}
	apply()
	save()
}
