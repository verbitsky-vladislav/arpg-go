// Package character — игровой персонаж: описательные данные о телах и
// лоадаутах и автомат состояний, который по вводу игрока крутит анимации.
//
// Устройство то же, что у животных (internal/mob): данные о смысле лежат в
// assets/character/character.json, графика — в manifest.json рядом со
// спрайтами и грузится sprite.Pack. Разница одна: намерения приходят не от ИИ,
// а от ввода (см. Input). Формат таблицы: docs/character/character.md.
//
// Вариант персонажа — это пара «тело × лоадаут»: тело задаёт внешность и
// небольшие поправки к балансу, лоадаут — оружие, набор анимаций и удар. Пак
// лежит в assets/character/<body>/<loadout>.
package character

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"sort"

	"github.com/vladislav/game/internal/assets"
	"github.com/vladislav/game/internal/sprite"
)

// Title — отображаемое имя на двух языках.
type Title struct {
	RU string `json:"ru"`
	EN string `json:"en"`
}

// Base — баланс, общий для всех вариантов персонажа. Тело и лоадаут его только
// домножают, поэтому «сколько у героя здоровья» — это одно число здесь, а не
// четыре по вариантам.
type Base struct {
	HP    int `json:"hp"`
	Speed struct {
		Walk float64 `json:"walk"`
		Run  float64 `json:"run"`
	} `json:"speed"`
	BodyRadius float64 `json:"body_radius"`
	Hurt       struct {
		LockTicks   int     `json:"lock_ticks"`   // тиков оцепенения после удара
		InvulnTicks int     `json:"invuln_ticks"` // тиков неуязвимости после удара
		Knockback   float64 `json:"knockback"`    // отброс, px/с
	} `json:"hurt"`
	Death struct {
		FadeTicks int `json:"fade_ticks"`
	} `json:"death"`
	AttackMoveScale float64 `json:"attack_move_scale"`
}

// Body — тело (внешность) персонажа.
type Body struct {
	Art            string  `json:"art"` // каталог со спрайтами (== ключ записи)
	Title          Title   `json:"title"`
	DefaultLoadout string  `json:"default_loadout"`
	HPScale        float64 `json:"hp_scale"`
	SpeedScale     float64 `json:"speed_scale"`
	Notes          string  `json:"notes"`

	ID string `json:"-"` // ключ записи, проставляется загрузчиком
}

// Attack — как бьёт лоадаут. Сектор перед персонажем: reach — его радиус, arc —
// раствор в градусах.
type Attack struct {
	Damage int     `json:"damage"`
	Reach  float64 `json:"reach"`
	Arc    float64 `json:"arc"`
	// SwingTicks — длительность замаха в тиках. Клип удара растягивается под
	// неё: fps в manifest.json один на весь пак (его ставит assetnorm) и к
	// скорости боя отношения не имеет.
	SwingTicks    int     `json:"swing_ticks"`
	CooldownTicks int     `json:"cooldown_ticks"`
	Knockback     float64 `json:"knockback"`
	// OnMove — умеет ли лоадаут бить на ходу (клипы walk_attack/run_attack).
	OnMove bool `json:"on_move"`
	// HitAt — на каком кадре клипа наносится урон. Клипа нет в таблице —
	// урон приходится на середину клипа (см. Player.hitFrame).
	HitAt map[string]int `json:"hit_at"`
}

// CanStrike — умеет ли лоадаут бить. Нулевой урон означает «оружия нет, драться
// нечем»: безоружный герой не атакует вообще, а не бьёт слабо. Это свойство
// данных, а не кода, — вооружившись, тот же герой начнёт бить без правок.
func (l *Loadout) CanStrike() bool { return l != nil && l.Attack.Damage > 0 }

// Loadout — чем персонаж вооружён: подкаталог тела со своим набором анимаций.
type Loadout struct {
	Art        string  `json:"art"` // подкаталог (== ключ записи)
	Title      Title   `json:"title"`
	SpeedScale float64 `json:"speed_scale"`
	Attack     Attack  `json:"attack"`
	Notes      string  `json:"notes"`

	ID string `json:"-"`
}

// Catalog — таблица персонажа целиком.
type Catalog struct {
	Version  int                 `json:"version"`
	Vocab    map[string]any      `json:"vocab"`
	Base     Base                `json:"base"`
	Bodies   map[string]*Body    `json:"bodies"`
	Loadouts map[string]*Loadout `json:"loadouts"`
}

// Load читает таблицу персонажа из fsys по пути p
// (обычно "character/character.json").
func Load(fsys fs.FS, p string) (*Catalog, error) {
	b, err := fs.ReadFile(fsys, p)
	if err != nil {
		return nil, fmt.Errorf("character: чтение %q: %w", p, err)
	}
	var c Catalog
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("character: разбор %q: %w", p, err)
	}
	for id, b := range c.Bodies {
		b.ID = id
	}
	for id, l := range c.Loadouts {
		l.ID = id
	}
	return &c, nil
}

// BodyIDs — тела в стабильном (алфавитном) порядке.
func (c *Catalog) BodyIDs() []string { return sortedKeys(c.Bodies) }

// LoadoutIDs — лоадауты в стабильном (алфавитном) порядке.
func (c *Catalog) LoadoutIDs() []string { return sortedKeys(c.Loadouts) }

// Body — тело по идентификатору (nil, если нет).
func (c *Catalog) Body(id string) *Body { return c.Bodies[id] }

// Loadout — лоадаут по идентификатору (nil, если нет).
func (c *Catalog) Loadout(id string) *Loadout { return c.Loadouts[id] }

// PackDir — каталог спрайт-пака пары «тело × лоадаут» внутри fs загрузчика.
// root — корень раздела персонажа (обычно "character").
func PackDir(root string, b *Body, l *Loadout) string {
	return path.Join(root, b.Art, l.Art)
}

// LoadPack грузит пак пары.
func LoadPack(ld *assets.Loader, root string, b *Body, l *Loadout) (*sprite.Pack, error) {
	return sprite.Load(ld, PackDir(root, b, l))
}

// LoadPacks грузит паки всех пар «тело × лоадаут» разом. Ключ — "тело/лоадаут".
// Нужен там, где лоадаут меняется на лету (смена оружия) и загрузка посреди боя
// нежелательна, и просмотрщику, которому нужны все варианты сразу.
func LoadPacks(ld *assets.Loader, root string, c *Catalog) (map[string]*sprite.Pack, error) {
	out := make(map[string]*sprite.Pack, len(c.Bodies)*len(c.Loadouts))
	for _, bid := range c.BodyIDs() {
		for _, lid := range c.LoadoutIDs() {
			p, err := LoadPack(ld, root, c.Bodies[bid], c.Loadouts[lid])
			if err != nil {
				return nil, err
			}
			out[bid+"/"+lid] = p
		}
	}
	return out, nil
}

// Validate проверяет внутреннюю связность таблицы и возвращает список проблем
// (пустой — всё сошлось). Графику не трогает: сходятся ли hit_at с реальной
// длиной клипов, проверяет отдельная сверка с sprite.Pack (см. тесты).
func (c *Catalog) Validate() []string {
	var probs []string
	add := func(f string, a ...any) { probs = append(probs, fmt.Sprintf(f, a...)) }

	if c.Base.HP <= 0 {
		add("base.hp <= 0")
	}
	if c.Base.Speed.Walk <= 0 || c.Base.Speed.Run < c.Base.Speed.Walk {
		add("base.speed: walk=%.0f run=%.0f — бег должен быть не медленнее шага",
			c.Base.Speed.Walk, c.Base.Speed.Run)
	}
	if c.Base.BodyRadius <= 0 {
		add("base.body_radius <= 0")
	}
	if c.Base.Hurt.LockTicks <= 0 {
		add("base.hurt.lock_ticks <= 0")
	}
	if c.Base.Death.FadeTicks <= 0 {
		add("base.death.fade_ticks <= 0")
	}
	if c.Base.AttackMoveScale <= 0 || c.Base.AttackMoveScale > 1 {
		add("base.attack_move_scale=%.2f вне (0..1]", c.Base.AttackMoveScale)
	}
	if len(c.Bodies) == 0 {
		add("не задано ни одного тела")
	}
	if len(c.Loadouts) == 0 {
		add("не задано ни одного лоадаута")
	}

	for _, id := range c.BodyIDs() {
		b := c.Bodies[id]
		if b.Art != id {
			add("%s: art=%q не совпадает с ключом", id, b.Art)
		}
		if c.Loadouts[b.DefaultLoadout] == nil {
			add("%s: default_loadout=%q — неизвестный лоадаут", id, b.DefaultLoadout)
		}
		if b.HPScale <= 0 {
			add("%s: hp_scale <= 0", id)
		}
		if b.SpeedScale <= 0 {
			add("%s: speed_scale <= 0", id)
		}
	}

	for _, id := range c.LoadoutIDs() {
		l := c.Loadouts[id]
		if l.Art != id {
			add("%s: art=%q не совпадает с ключом", id, l.Art)
		}
		if l.SpeedScale <= 0 {
			add("%s: speed_scale <= 0", id)
		}
		a := l.Attack
		switch {
		case a.Damage < 0:
			add("%s: attack.damage=%d отрицателен", id, a.Damage)
		case !l.CanStrike():
			// Лоадаут без удара — законное состояние (безоружный). Тогда и
			// остального удара быть не должно: половина заполненных полей
			// означала бы, что кто-то выключил урон и забыл про прочее.
			if a.Reach != 0 || a.Arc != 0 || a.SwingTicks != 0 ||
				a.CooldownTicks != 0 || a.Knockback != 0 || a.OnMove || len(a.HitAt) > 0 {
				add("%s: attack.damage=0 (лоадаут не бьёт), но остальные поля удара заполнены", id)
			}
		case a.Reach <= 0:
			add("%s: attack.reach <= 0", id)
		case a.Arc <= 0 || a.Arc > 360:
			add("%s: attack.arc=%.0f вне 0..360", id, a.Arc)
		case a.SwingTicks <= 0:
			add("%s: attack.swing_ticks <= 0 — замах нулевой длины", id)
		case a.CooldownTicks <= 0:
			add("%s: attack.cooldown_ticks <= 0 — удар без перезарядки", id)
		}
		for clip, f := range a.HitAt {
			if f < 0 {
				add("%s: attack.hit_at[%q]=%d отрицателен", id, clip, f)
			}
			if !attackClips[clip] {
				add("%s: attack.hit_at[%q] — не ударный клип", id, clip)
			}
		}
		// Бить на ходу нечем, если у лоадаута нет ни одного клипа удара в
		// движении: hit_at — единственное место, где такие клипы объявлены.
		_, walkAtk := a.HitAt["walk_attack"]
		_, runAtk := a.HitAt["run_attack"]
		if a.OnMove && !walkAtk && !runAtk {
			add("%s: attack.on_move=true, но клипы walk_attack/run_attack не объявлены в hit_at", id)
		}
	}
	return probs
}

// attackClips — клипы, на которых вообще бывает попадание.
var attackClips = map[string]bool{"attack": true, "walk_attack": true, "run_attack": true}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
