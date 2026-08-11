// Package hero — предметная модель героев: их список, имена и наборы анимаций.
// Кадры грузятся лениво через assets.Loader и кэшируются в самом герое, а
// тайминги действий заданы в одном месте (actionDefs), чтобы одинаково
// использоваться и в меню выбора героя, и позже в самой игре.
package hero

import (
	"fmt"

	"github.com/vladislav/game/internal/anim"
	"github.com/vladislav/game/internal/assets"
)

// spritesRoot — корень спрайтов героев внутри assets-FS.
// Структура: sprites/heroes/<id>/<facing>/<action>/frame_00.png ...
const spritesRoot = "sprites/heroes"

// Facing — направление вида. Спрайты есть для Down (фронт), Up (спина) и Side
// (профиль лицом вправо). Взгляд влево — это Side, отражённый по горизонтали
// при отрисовке (folder тот же "side"). Итого 4 направления из 3 наборов.
type Facing int

const (
	Down Facing = iota // к камере (фронт)
	Up                 // от камеры (спина)
	Side               // профиль (в бок)
)

// folder — каталог вида в ассетах.
func (f Facing) folder() string {
	switch f {
	case Up:
		return "up"
	case Side:
		return "side"
	default:
		return "down"
	}
}

// ID — идентификатор героя; совпадает с именем каталога спрайтов.
type ID string

const (
	Assassin  ID = "assassin"
	Barbarian ID = "barbarian"
	Engineer  ID = "engineer"
	Knight    ID = "knight"
	Mage      ID = "mage"
	Ranger    ID = "ranger"
)

// order — порядок героев в меню.
var order = []ID{Assassin, Barbarian, Engineer, Knight, Mage, Ranger}

var names = map[ID]string{
	Assassin:  "Assassin",
	Barbarian: "Barbarian",
	Engineer:  "Engineer",
	Knight:    "Knight",
	Mage:      "Mage",
	Ranger:    "Ranger",
}

// Action — вид анимации героя. Значение маппится на каталог с кадрами.
type Action int

const (
	Idle Action = iota
	Walk
	Attack
	Punch
	TakeDamage
	Loot
)

// Actions — все действия в порядке показа в галерее.
var Actions = []Action{Idle, Walk, Attack, Punch, TakeDamage, Loot}

type actionDef struct {
	dir        string // имя каталога кадров
	title      string // человекочитаемое имя
	frameTicks int    // тиков на кадр
	loop       bool   // зациклено ли действие
}

// actionDefs — единый источник правды по действиям: каталог, тайминг,
// зацикленность. Кадров 16, поэтому frameTicks вдвое меньше «покадровой»
// интуиции — длительность цикла сохраняется, а движение плавнее.
var actionDefs = map[Action]actionDef{
	Idle:       {"idle", "Idle", 4, true},
	Walk:       {"walk", "Walk", 3, true},
	Attack:     {"attack", "Attack", 2, false},
	Punch:      {"punch", "Punch", 2, false},
	TakeDamage: {"take_damage", "Take Damage", 2, false},
	Loot:       {"loot", "Loot", 3, false},
}

// Dir — имя каталога кадров действия.
func (a Action) Dir() string { return actionDefs[a].dir }

// Title — человекочитаемое имя действия (для UI).
func (a Action) Title() string { return actionDefs[a].title }

// Hero — герой со своим набором анимаций. Клипы грузятся лениво и кэшируются
// по паре (вид, действие).
type Hero struct {
	ID    ID
	Name  string
	l     *assets.Loader
	clips map[string]*anim.Clip
}

// Clip лениво загружает и кэширует клип действия a для вида f. При ошибке
// загрузки возвращает nil (и запоминает это) — вызывающий код безопасно
// работает с nil через anim.Clip.Valid / anim.Player.
func (h *Hero) Clip(a Action, f Facing) *anim.Clip {
	def := actionDefs[a]
	dir := fmt.Sprintf("%s/%s/%s/%s", spritesRoot, h.ID, f.folder(), def.dir)
	if c, ok := h.clips[dir]; ok {
		return c
	}
	frames, err := h.l.Frames(dir)
	if err != nil {
		h.clips[dir] = nil // не пробовать повторно
		return nil
	}
	c := &anim.Clip{Frames: frames, FrameTicks: def.frameTicks, Loop: def.loop}
	h.clips[dir] = c
	return c
}

// Registry — реестр всех героев поверх одного загрузчика ассетов.
type Registry struct {
	heroes []*Hero
	byID   map[ID]*Hero
}

// NewRegistry строит реестр героев (сами кадры пока не грузятся — лениво).
func NewRegistry(l *assets.Loader) *Registry {
	r := &Registry{byID: make(map[ID]*Hero, len(order))}
	for _, id := range order {
		h := &Hero{ID: id, Name: names[id], l: l, clips: map[string]*anim.Clip{}}
		r.heroes = append(r.heroes, h)
		r.byID[id] = h
	}
	return r
}

// Heroes — все герои в порядке отображения.
func (r *Registry) Heroes() []*Hero { return r.heroes }

// Get — герой по ID (nil, если нет).
func (r *Registry) Get(id ID) *Hero { return r.byID[id] }
