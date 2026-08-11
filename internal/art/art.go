// Package art — конкретные спрайты игры, нарисованные в коде из пиксельных
// карт (пакет gfx). Здесь «живёт» весь пиксель-арт. Каждый спрайт строится
// один раз и кэшируется. Добавляешь моба/оружие — рисуешь карту здесь.
package art

import (
	"github.com/hajimehoshi/ebiten/v2"

	"github.com/vladislav/game/internal/combat"
	"github.com/vladislav/game/internal/gfx"
)

const scale = 1 // 1 пиксель карты = 1 логический пиксель (окно масштабирует ×2)

var (
	playerImg *ebiten.Image
	mobImg    [3]*ebiten.Image
	projImg   = map[combat.DamageType]*ebiten.Image{}
)

// Player — спрайт игрока (маг в плаще).
func Player() *ebiten.Image {
	if playerImg == nil {
		pal := gfx.Palette{
			'X': gfx.Hex("#12203a"), // тёмный контур
			'S': gfx.Hex("#e8b48a"), // кожа
			'B': gfx.Hex("#3a6ea5"), // плащ
			'b': gfx.Hex("#28527a"), // тень плаща
			'E': gfx.Hex("#1a1a22"), // глаза
		}
		rows := []string{
			"...XXX...",
			"..XSSSX..",
			"..XEXEX..",
			"..XSSSX..",
			".XBBBBBX.",
			"XBBBBBBBX",
			"XBbBBBbBX",
			"XBBBBBBBX",
			"XBbBBBbBX",
			".XBBBBBX.",
			"..X.B.X..",
			"..X...X..",
			".XX...XX.",
		}
		playerImg = gfx.NewSprite(rows, pal, scale)
	}
	return playerImg
}

// Mob возвращает спрайт моба по варианту: 0 — grunt, 1 — brute, 2 — zapper.
func Mob(variant int) *ebiten.Image {
	if variant < 0 || variant > 2 {
		variant = 0
	}
	if mobImg[variant] == nil {
		mobImg[variant] = buildMob(variant)
	}
	return mobImg[variant]
}

func buildMob(variant int) *ebiten.Image {
	// Общая форма «клякса с глазами», меняем палитру и размер под тип.
	var body, dark, eye string
	switch variant {
	case 1: // brute — зелёный танк
		body, dark, eye = "#4a8f3c", "#2c5a24", "#ffe27a"
	case 2: // zapper — фиолетовый шустрый
		body, dark, eye = "#8a4fd0", "#5a2f8a", "#c8ff7a"
	default: // grunt — красный
		body, dark, eye = "#c03838", "#3a0f0f", "#ffd24d"
	}
	pal := gfx.Palette{
		'R': gfx.Hex(body),
		'X': gfx.Hex(dark),
		'Y': gfx.Hex(eye),
		'W': gfx.Hex("#2a0808"),
	}
	rows := []string{
		"..XXXX..",
		".XRRRRX.",
		"XRRRRRRX",
		"XRYRRYRX",
		"XRRRRRRX",
		"XRWWWWRX",
		".XRRRRX.",
		"..X..X..",
	}
	if variant == 1 { // brute крупнее
		return gfx.NewSprite(rows, pal, scale+1)
	}
	return gfx.NewSprite(rows, pal, scale)
}

// Projectile — спрайт снаряда, цвет зависит от типа урона.
func Projectile(t combat.DamageType) *ebiten.Image {
	if img, ok := projImg[t]; ok {
		return img
	}
	var core, edge string
	switch t {
	case combat.Fire:
		core, edge = "#fff1a0", "#ff6a1a"
	case combat.Cold:
		core, edge = "#dffcff", "#3aa0ff"
	case combat.Lightning:
		core, edge = "#ffffcc", "#ffe23a"
	case combat.Chaos:
		core, edge = "#f0c8ff", "#a83aff"
	default: // Physical
		core, edge = "#ffffff", "#a0a0a0"
	}
	pal := gfx.Palette{'c': gfx.Hex(core), 'e': gfx.Hex(edge)}
	rows := []string{
		".eee.",
		"eccce",
		"eccce",
		"eccce",
		".eee.",
	}
	img := gfx.NewSprite(rows, pal, scale)
	projImg[t] = img
	return img
}
