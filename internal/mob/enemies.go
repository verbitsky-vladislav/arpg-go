package mob

import (
	"encoding/json"
	"fmt"
	"image/color"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
)

// Таблица враждебных мобов: assets/mobs/enemies/enemies.json и, тем же
// форматом, assets/mobs/bosses/bosses.json. Формат: docs/mobs/enemies.md.
//
// От животных (species.json) отличается одним: у врага два уровня. Тип — это
// смысл (кто это, где водится, чем бьёт), тир — числа конкретной особи. Каждый
// тип нарисован в трёх силах подряд, и без такого деления смысловые поля
// пришлось бы повторять по три раза на все шестьдесят вариантов.

// Title — отображаемое имя на двух языках.
type Title struct {
	RU string `json:"ru"`
	EN string `json:"en"`
}

// PowerAttack — удар украденной силы уже в руках героя. Поля совпадают с
// loadouts.*.attack из character/character.json намеренно: сила становится
// альтернативным ударом персонажа, а не отдельной механикой.
type PowerAttack struct {
	Damage        int     `json:"damage"`
	Reach         float64 `json:"reach"`
	Arc           float64 `json:"arc"`
	SwingTicks    int     `json:"swing_ticks"`
	CooldownTicks int     `json:"cooldown_ticks"`
	Knockback     float64 `json:"knockback"`
	OnMove        bool    `json:"on_move"`
	// HitAt — кадр слоя-эффекта, на котором наносится урон.
	HitAt int `json:"hit_at"`
}

// Power — что достаётся игроку с убитого моба.
//
// Layer — суффикс файла в <пак>/parts: у врага атака нарисована послойно, и
// эффект (огонь, магия, дым, взмах) лежит отдельным PNG. Именно он переносится
// на героя, поэтому герой остаётся собой, а бьёт чужим.
type Power struct {
	Layer   string `json:"layer"`
	Element string `json:"element"`
	// Tint — цвет, в который перекрашивается слой при ударе героя ("#rrggbb").
	// Нужен рубящим силам: след клинка у всех белый, и без окраски украденный
	// удар не отличить от собственного взмаха героя. У стихийных сил пусто —
	// огонь и дым и так говорят за себя.
	Tint        string  `json:"tint"`
	StealChance float64 `json:"steal_chance"`
	// Зарядов у силы здесь нет: сколько раз ею ударят, зависит не от типа, а от
	// того, с кого её сняли, — см. PowerCharges (steal.go).
	Attack PowerAttack `json:"attack"`
}

// TintColor — разобранный Tint. ok=false, если цвет не задан.
func (p *Power) TintColor() (color.RGBA, bool) {
	c, err := parseHex(p.Tint)
	return c, err == nil
}

// parseHex разбирает "#rrggbb".
func parseHex(s string) (color.RGBA, error) {
	if len(s) != 7 || s[0] != '#' {
		return color.RGBA{}, fmt.Errorf("mob: цвет %q не в виде #rrggbb", s)
	}
	v, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil {
		return color.RGBA{}, fmt.Errorf("mob: цвет %q: %w", s, err)
	}
	return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 0xff}, nil
}

// Tier — одна сила типа: числа и имя. У слизней вместо t1..t3 цвета, и тогда
// у тира есть своя стихия.
type Tier struct {
	Title   Title  `json:"title"`
	Element string `json:"element"`
	HP      int    `json:"hp"`
	Damage  int    `json:"damage"`
	Speed   struct {
		Walk float64 `json:"walk"`
		Run  float64 `json:"run"`
	} `json:"speed"`
	XP        XPRange `json:"xp"`
	DropRolls int     `json:"drop_rolls"`

	ID   string     `json:"-"` // ключ записи (t1, green, ...)
	Type *EnemyType `json:"-"`
}

// Phase — фаза боя босса.
type Phase struct {
	AtHP       float64 `json:"at_hp"`
	Attack     string  `json:"attack"`
	SpeedScale float64 `json:"speed_scale"`
	Notes      string  `json:"notes"`
}

// Arena — как босс держится карты.
type Arena struct {
	Unique     bool `json:"unique"`     // больше одного на карте не бывает
	Persistent bool `json:"persistent"` // не снимается спавнером по удалению игрока
}

// EnemyType — тип врага: всё, что одинаково у его тиров.
type EnemyType struct {
	Art        string `json:"art"` // каталог со спрайтами (== ключ записи)
	Title      Title  `json:"title"`
	Family     string `json:"family"`
	SizeClass  string `json:"size_class"`
	Temper     string `json:"temper"`   // aggressive|territorial|ambusher|ranged
	Activity   string `json:"activity"` // day|night|any
	Locomotion struct {
		Ground bool `json:"ground"`
		Air    bool `json:"air"`
	} `json:"locomotion"`
	Threat struct {
		Attacks    bool    `json:"attacks"`
		Unprovoked bool    `json:"unprovoked"`
		Sight      float64 `json:"sight"`
		Reach      float64 `json:"reach"`
		FleeHP     float64 `json:"flee_hp"`
	} `json:"threat"`
	Habitat struct {
		Biomes map[string]int `json:"biomes"`
		Zones  []string       `json:"zones"`
		Group  struct {
			Min int `json:"min"`
			Max int `json:"max"`
		} `json:"group"`
	} `json:"habitat"`
	Drops  []Drop           `json:"drops"`
	Power  *Power           `json:"power"` // null — отбирать нечего
	Arena  *Arena           `json:"arena"` // только у боссов
	Phases []Phase          `json:"phases"`
	Notes  string           `json:"notes"`
	Tiers  map[string]*Tier `json:"tiers"`

	ID string `json:"-"` // ключ записи
}

// EnemyCatalog — таблица целиком (враги или боссы: формат один).
type EnemyCatalog struct {
	Version int                   `json:"version"`
	Vocab   map[string]any        `json:"vocab"`
	Types   map[string]*EnemyType `json:"types"`
}

// LoadEnemies читает таблицу из fsys по пути p
// ("mobs/enemies/enemies.json" или "mobs/bosses/bosses.json").
func LoadEnemies(fsys fs.FS, p string) (*EnemyCatalog, error) {
	b, err := fs.ReadFile(fsys, p)
	if err != nil {
		return nil, fmt.Errorf("mob: чтение %q: %w", p, err)
	}
	var c EnemyCatalog
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("mob: разбор %q: %w", p, err)
	}
	for id, t := range c.Types {
		t.ID = id
		for tid, tier := range t.Tiers {
			tier.ID, tier.Type = tid, t
		}
	}
	return &c, nil
}

// TypeIDs — типы в стабильном (алфавитном) порядке.
func (c *EnemyCatalog) TypeIDs() []string {
	ids := make([]string, 0, len(c.Types))
	for id := range c.Types {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// IDs — все варианты вида "demon_t1" в стабильном порядке. Совпадает с полем
// id манифеста пака, поэтому годится как сквозной идентификатор особи.
func (c *EnemyCatalog) IDs() []string {
	var ids []string
	for _, tid := range c.TypeIDs() {
		for _, tier := range c.Types[tid].TierIDs() {
			ids = append(ids, tid+"_"+tier)
		}
	}
	return ids
}

// TierIDs — тиры типа в стабильном порядке.
func (t *EnemyType) TierIDs() []string {
	ids := make([]string, 0, len(t.Tiers))
	for id := range t.Tiers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Get — вариант по сквозному идентификатору ("demon_t1"). nil, если такого нет.
//
// Разбор идёт справа налево по последнему подчёркиванию: в именах типов оно
// встречается (slime_boss), в именах тиров — нет.
func (c *EnemyCatalog) Get(id string) *Tier {
	i := strings.LastIndex(id, "_")
	if i < 0 {
		return nil
	}
	t := c.Types[id[:i]]
	if t == nil {
		return nil
	}
	return t.Tiers[id[i+1:]]
}

// PackDir — каталог спрайт-пака варианта внутри fs. root — корень раздела
// ("mobs/enemies" или "mobs/bosses").
func (t *Tier) PackDir(root string) string { return path.Join(root, t.Type.Art, t.ID) }

// Elem — стихия варианта: своя у слизней (цвет), иначе стихия силы типа.
func (t *Tier) Elem() string {
	if t.Element != "" {
		return t.Element
	}
	if t.Type != nil && t.Type.Power != nil {
		return t.Type.Power.Element
	}
	return ""
}

// Validate проверяет внутреннюю связность таблицы и возвращает список проблем
// (пустой — всё сошлось). Графику не трогает: существование слоя-эффекта и
// сходимость hit_at с длиной клипа проверяет отдельная сверка с пакетом sprite
// (см. тесты).
func (c *EnemyCatalog) Validate() []string {
	var probs []string
	add := func(f string, a ...any) { probs = append(probs, fmt.Sprintf(f, a...)) }

	for _, id := range c.TypeIDs() {
		t := c.Types[id]
		if t.Art != id {
			add("%s: art=%q не совпадает с ключом", id, t.Art)
		}
		if len(t.Tiers) == 0 {
			add("%s: ни одного тира", id)
		}
		if len(t.Habitat.Biomes) == 0 {
			add("%s: не задан ни один биом", id)
		}
		if len(t.Habitat.Zones) == 0 {
			add("%s: не заданы зоны", id)
		}
		if t.Habitat.Group.Min > t.Habitat.Group.Max || t.Habitat.Group.Min < 1 {
			add("%s: habitat.group %d..%d", id, t.Habitat.Group.Min, t.Habitat.Group.Max)
		}
		if t.Threat.Sight <= 0 {
			add("%s: нулевой радиус обнаружения", id)
		}
		if t.Threat.Reach <= 0 {
			add("%s: нулевая дистанция удара", id)
		}
		if t.Threat.FleeHP < 0 || t.Threat.FleeHP > 1 {
			add("%s: flee_hp=%.2f вне 0..1", id, t.Threat.FleeHP)
		}
		if !t.Locomotion.Ground && !t.Locomotion.Air {
			add("%s: не умеет ни ходить, ни летать", id)
		}
		for _, p := range dropProblems(t.Drops) {
			add("%s: %s", id, p)
		}

		for _, tid := range t.TierIDs() {
			tr := t.Tiers[tid]
			who := id + "_" + tid
			switch {
			case tr.HP <= 0:
				add("%s: hp <= 0", who)
			case tr.Damage <= 0 && t.Threat.Attacks:
				add("%s: враг без урона, хотя attacks=true", who)
			case tr.Speed.Walk <= 0:
				add("%s: нулевая скорость шага", who)
			case tr.Speed.Run < tr.Speed.Walk:
				add("%s: бег медленнее шага", who)
			case tr.XP.Min <= 0 || tr.XP.Max < tr.XP.Min:
				add("%s: полоса опыта %d..%d", who, tr.XP.Min, tr.XP.Max)
			case tr.DropRolls < 0:
				add("%s: отрицательное число бросков дропа", who)
			}
			if tr.Title.RU == "" || tr.Title.EN == "" {
				add("%s: пустое имя", who)
			}
		}

		if p := t.Power; p != nil {
			switch {
			case p.Layer == "":
				add("%s: power без слоя", id)
			case p.Element == "":
				add("%s: power без стихии", id)
			case p.StealChance <= 0 || p.StealChance > 1:
				add("%s: power.steal_chance=%.2f вне 0..1", id, p.StealChance)
			case p.Attack.Damage <= 0:
				add("%s: power.attack.damage <= 0", id)
			case p.Attack.Reach <= 0:
				add("%s: power.attack.reach <= 0", id)
			case p.Attack.Arc <= 0 || p.Attack.Arc > 180:
				add("%s: power.attack.arc=%.0f вне 0..180 (сектор за спину не бьёт)", id, p.Attack.Arc)
			case p.Attack.SwingTicks <= 0:
				add("%s: power.attack.swing_ticks <= 0", id)
			case p.Attack.CooldownTicks <= 0:
				add("%s: power.attack.cooldown_ticks <= 0", id)
			case p.Attack.HitAt < 0:
				add("%s: power.attack.hit_at отрицателен", id)
			}
			if p.Tint != "" {
				if _, err := parseHex(p.Tint); err != nil {
					add("%s: power.tint: %v", id, err)
				}
			}
			// Рубящий след у всех белый: без окраски украденный удар
			// неотличим от собственного взмаха героя.
			if p.Element == "slash" && p.Tint == "" {
				add("%s: рубящая сила без tint — на экране сольётся со взмахом героя", id)
			}
		}

		// Боссовые поля — только у боссов, и наоборот: фазы без арены означают,
		// что запись попала не в тот файл.
		if len(t.Phases) > 0 && t.Arena == nil {
			add("%s: фазы заданы, а arena — нет", id)
		}
		for i, ph := range t.Phases {
			if ph.AtHP <= 0 || ph.AtHP > 1 {
				add("%s: фаза %d: at_hp=%.2f вне 0..1", id, i, ph.AtHP)
			}
			if ph.Attack == "" {
				add("%s: фаза %d: не сказано, чем бьёт", id, i)
			}
			if i > 0 && ph.AtHP >= t.Phases[i-1].AtHP {
				add("%s: фаза %d начинается не ниже предыдущей", id, i)
			}
		}
	}
	return probs
}

// LayerFile ищет в паке файл слоя-эффекта. В таблице записан смысловой суффикс
// ("attack_fire"), а в parts/ имена как получилось у художника:
//
//	demon1_attack_fire.png      — префикс пака с номером тира
//	attack6_swing.png           — без префикса, зато с номером слоя (goblin/t1)
//	attack_swing.png            — тот же слой у соседнего тира (goblin/t2)
//
// Поэтому сравниваются имена без цифр, а варианты удара на ходу отсеиваются:
// без этого "attack_swing" поймал бы orc1_run_attack_swing.png.
func LayerFile(fsys fs.FS, packDir, layer string) (string, error) {
	dir := path.Join(packDir, "parts")
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return "", fmt.Errorf("mob: чтение %q: %w", dir, err)
	}
	want := undigit(layer)
	moving := strings.Contains(want, "run_attack") || strings.Contains(want, "walk_attack")
	for _, e := range entries {
		got := undigit(strings.TrimSuffix(e.Name(), ".png"))
		if !moving && (strings.Contains(got, "run_attack") || strings.Contains(got, "walk_attack")) {
			continue
		}
		if got == want || strings.HasSuffix(got, "_"+want) {
			return path.Join(dir, e.Name()), nil
		}
	}
	return "", fmt.Errorf("mob: в %q нет слоя %q", dir, layer)
}

// undigit убирает цифры из имени: они означают либо номер тира в префиксе, либо
// номер слоя в исходнике, и к смыслу слоя отношения не имеют.
func undigit(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return -1
		}
		return r
	}, s)
}
