// Package save — персистентность персонажа: уровень/опыт/прокачка на диск.
// Best-effort: ошибки чтения/записи не валят игру (просто нет сейва).
// Файлы: <os.UserConfigDir>/pixel-arpg/chars/<heroID>.json, один на героя.
package save

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Character — сериализуемое состояние персонажа.
type Character struct {
	Level int      `json:"level"`
	XP    int      `json:"xp"`
	Nodes []string `json:"nodes"`
}

func dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	d := filepath.Join(base, "pixel-arpg", "chars")
	if err := os.MkdirAll(d, 0o755); err != nil {
		return "", err
	}
	return d, nil
}

func path(heroID string) (string, bool) {
	d, err := dir()
	if err != nil {
		return "", false
	}
	return filepath.Join(d, heroID+".json"), true
}

// Load читает сохранение героя (ok=false — нет/битое).
func Load(heroID string) (Character, bool) {
	p, ok := path(heroID)
	if !ok {
		return Character{}, false
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return Character{}, false
	}
	var c Character
	if json.Unmarshal(b, &c) != nil {
		return Character{}, false
	}
	return c, true
}

// Store пишет сохранение героя (тихо игнорирует ошибки).
func Store(heroID string, c Character) {
	p, ok := path(heroID)
	if !ok {
		return
	}
	if b, err := json.MarshalIndent(c, "", " "); err == nil {
		_ = os.WriteFile(p, b, 0o644)
	}
}
