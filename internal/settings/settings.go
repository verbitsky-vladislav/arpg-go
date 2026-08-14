// Package settings — пользовательские настройки, сохраняемые между запусками.
// Единый источник правды: load → apply (ebiten/ui) → save при каждом изменении.
// Файл лежит в пользовательском конфиг-каталоге ОС (fallback — текущий каталог).
package settings

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/vladislav/game/internal/audio"
	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/ui"
)

// Settings — сохраняемые параметры.
type Settings struct {
	Fullscreen  bool `json:"fullscreen"`
	VSync       bool `json:"vsync"`
	WindowScale int  `json:"window_scale"` // множитель логического разрешения (2..4)
	ShowFPS     bool `json:"show_fps"`
	// Громкость в процентах: общая и отдельно эффекты. Ноль — законное
	// значение («выключить»), поэтому отличить его от «в файле поля нет»
	// обычным int нельзя — и не нужно: load разбирает JSON поверх дефолтов,
	// так что старый файл настроек сохраняет громкость по умолчанию.
	Volume      int `json:"volume"`
	SFXVolume   int `json:"sfx_volume"`
	MusicVolume int `json:"music_volume"`
	// BarStyle — вид полосы здоровья (id из assets/ui/bars/bars.json). Пустая
	// строка и неизвестный id разрешаются в стиль по умолчанию, поэтому старый
	// файл настроек и подчищенный список стилей обходятся без миграции.
	BarStyle string `json:"bar_style"`
	// Keys — привязки клавиш: действие (см. keys.go) → клавиша. Отсутствующее
	// действие берёт клавишу по умолчанию, поэтому старый файл настроек и новое
	// действие обходятся без миграции.
	Keys map[string]ebiten.Key `json:"keys"`
}

func defaults() Settings {
	return Settings{
		Fullscreen:  false,
		VSync:       true,
		WindowScale: config.WindowW / config.ScreenW, // как в config (2×)
		ShowFPS:     false,
		// Не на максимум: игра — не единственное, что звучит у игрока, и
		// стартовать громче, чем нужно, неприятнее, чем тише.
		Volume:    70,
		SFXVolume: 100,
		// Музыка тише эффектов: она фон, а не событие, и перекрывать ею удары
		// и шаги нельзя.
		MusicVolume: 70,
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
	s.Volume = clampVolume(s.Volume, defaults().Volume)
	s.SFXVolume = clampVolume(s.SFXVolume, defaults().SFXVolume)
	s.MusicVolume = clampVolume(s.MusicVolume, defaults().MusicVolume)
	// Неизвестные действия из файла игнорируем: список действий задаёт код.
	for id := range s.Keys {
		if !knownAction(id) {
			delete(s.Keys, id)
		}
	}
	dropDuplicateKeys(s.Keys)
	return s
}

// dropDuplicateKeys снимает сохранённые привязки, столкнувшиеся с чужой
// клавишей. Столкнуться они могут не по вине игрока: клавиша по умолчанию у
// нового действия способна совпасть с той, которую он когда-то назначил
// другому (так Q, E, R ушли под умения). Одна клавиша на два действия хуже, чем
// потерянная привязка: нажатие сработало бы дважды, и понять это по игре
// нельзя. Проигрывает более поздняя привязка — код перечисляет действия в
// порядке важности.
func dropDuplicateKeys(keys map[string]ebiten.Key) {
	if len(keys) == 0 {
		return
	}
	used := map[ebiten.Key]bool{}
	for a := Action(0); a < ActCount; a++ {
		id := actionID[a]
		k, bound := keys[id]
		if !bound {
			k = actionKey[a]
		}
		if used[k] && bound {
			delete(keys, id)
			k = actionKey[a]
		}
		used[k] = true
	}
}

func save() {
	p := filePath()
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	if b, err := json.MarshalIndent(current, "", "  "); err == nil {
		_ = os.WriteFile(p, b, 0o644)
	}
}

// clampVolume приводит громкость к 0..100, а мусор — к дефолту. Битое
// значение отличается от «выключено» тем, что выключил игрок, а мусор в файле
// не выбирал никто.
func clampVolume(v, def int) int {
	if v < 0 || v > 100 {
		return def
	}
	return v
}

// apply проталкивает текущие настройки в движок и UI.
func apply() {
	ebiten.SetFullscreen(current.Fullscreen)
	ebiten.SetVsyncEnabled(current.VSync)
	if !current.Fullscreen {
		ebiten.SetWindowSize(current.WindowScale*config.ScreenW, current.WindowScale*config.ScreenH)
	}
	ui.ShowFPS = current.ShowFPS
	audio.SetVolume(current.Volume, current.SFXVolume, current.MusicVolume)
}

// Шаг громкости: 10% — столько, чтобы прогнать всю шкалу десятком нажатий и
// при этом слышать разницу между соседними значениями.
const volumeStep = 10

// CycleVolume прибавляет шаг общей громкости, за 100% переходя в 0.
func CycleVolume() {
	current.Volume = nextVolume(current.Volume)
	apply()
	save()
}

// CycleSFXVolume — то же для громкости эффектов.
func CycleSFXVolume() {
	current.SFXVolume = nextVolume(current.SFXVolume)
	apply()
	save()
}

// CycleMusicVolume — то же для громкости музыки.
func CycleMusicVolume() {
	current.MusicVolume = nextVolume(current.MusicVolume)
	apply()
	save()
}

func nextVolume(v int) int {
	v += volumeStep
	if v > 100 {
		v = 0
	}
	return v
}

// --- переключатели (применяют и сохраняют) ---

func ToggleFullscreen() { current.Fullscreen = !current.Fullscreen; apply(); save() }
func ToggleVSync()      { current.VSync = !current.VSync; apply(); save() }
func ToggleFPS()        { current.ShowFPS = !current.ShowFPS; apply(); save() }

// SetBarStyle запоминает вид полосы здоровья. Какие стили есть и какой из них
// сейчас показан, знает ui — он их грузит и умеет разрешать пустое значение в
// стиль по умолчанию; здесь только хранение.
func SetBarStyle(id string) {
	current.BarStyle = id
	save()
}

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
