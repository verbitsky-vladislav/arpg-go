package mob

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

// Профили поведения врагов: assets/mobs/enemies/behavior.json.
// Формат и смысл полей: docs/mobs/enemies_ai.md.
//
// Профиль выбирается по temper типа, поверх ложится поправка по family. Слияние
// идёт по «сырому» JSON, а не по структурам с указателями: в файле у профиля
// стоят только те поля, которые он меняет, и различать «ноль» от «не задано»
// иначе пришлось бы в каждом поле.

// Perception — чем враг замечает игрока и как долго его помнит.
type Perception struct {
	FOVDeg         float64 `json:"fov_deg"`
	Hearing        float64 `json:"hearing"`
	ReactionTicks  int     `json:"reaction_ticks"`
	SuspicionTicks int     `json:"suspicion_ticks"`
	LoseTicks      int     `json:"lose_ticks"`
	MemoryTicks    int     `json:"memory_ticks"`
	SearchTicks    int     `json:"search_ticks"`
}

// CombatBehavior — как враг ведёт себя, когда цель найдена.
type CombatBehavior struct {
	PreferRange  float64 `json:"prefer_range"`
	KeepBand     float64 `json:"keep_band"`
	Strafe       float64 `json:"strafe"`
	CommitTicks  int     `json:"commit_ticks"`
	AttackSlots  int     `json:"attack_slots"`
	Flinch       bool    `json:"flinch"`
	RegroupTicks int     `json:"regroup_ticks"`
	// Dodge — шанс отскочить из-под занесённого оружия, DodgeTicks — длина
	// отскока. Ноль означает «не умеет»: голем стоит под ударом.
	Dodge      float64 `json:"dodge"`
	DodgeTicks int     `json:"dodge_ticks"`
	// Lead — доля упреждения при замахе: 0 — бить в текущую точку цели, 1 — в
	// ту, где она окажется к кадру попадания.
	Lead float64 `json:"lead"`
	// Dash — рывок за отрывающейся целью: шанс, длина и множитель скорости.
	Dash      float64 `json:"dash"`
	DashTicks int     `json:"dash_ticks"`
	DashScale float64 `json:"dash_scale"`
	// FinishHP — доля здоровья цели, ниже которой перестают осторожничать.
	FinishHP float64 `json:"finish_hp"`
	// Ambush — ждёт неподвижно, пока не заметит цель.
	Ambush bool `json:"ambush"`
}

// GroupBehavior — как враги договариваются между собой.
type GroupBehavior struct {
	CallRadius     float64 `json:"call_radius"`
	CallDelayTicks int     `json:"call_delay_ticks"`
	Spacing        float64 `json:"spacing"`
	Flank          float64 `json:"flank"`
	SurroundSlots  int     `json:"surround_slots"`
	// Cutoff — умеет ли родня отрезать отход (0 или 1).
	Cutoff int `json:"cutoff"`
}

// PatrolBehavior — что враг делает, пока никого не видит.
type PatrolBehavior struct {
	Radius    float64 `json:"radius"`
	WaitTicks [2]int  `json:"wait_ticks"`
	Leash     float64 `json:"leash"`
}

// Behavior — собранный профиль одного типа врага.
type Behavior struct {
	Perception Perception     `json:"perception"`
	Combat     CombatBehavior `json:"combat"`
	Group      GroupBehavior  `json:"group"`
	Patrol     PatrolBehavior `json:"patrol"`
}

// BehaviorSet — таблица профилей целиком.
type BehaviorSet struct {
	Version  int            `json:"version"`
	Defaults map[string]any `json:"defaults"`
	// Профили и семьи держатся как any, а не map[string]map[string]any:
	// рядом с ними в файле лежат ключи-комментарии со строками, и строгий тип
	// спотыкался бы о пояснение для человека.
	Profiles map[string]any `json:"profiles"`
	Families map[string]any `json:"families"`

	cache map[string]Behavior
}

// LoadBehavior читает профили поведения (обычно
// "mobs/enemies/behavior.json").
func LoadBehavior(fsys fs.FS, p string) (*BehaviorSet, error) {
	b, err := fs.ReadFile(fsys, p)
	if err != nil {
		return nil, fmt.Errorf("mob: чтение %q: %w", p, err)
	}
	var s BehaviorSet
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("mob: разбор %q: %w", p, err)
	}
	s.cache = map[string]Behavior{}
	return &s, nil
}

// For — профиль для характера temper с поправкой семьи family. Неизвестные
// имена не ошибка: берётся то, что есть, вплоть до одних defaults. Пропущенный
// профиль лучше странного поведения, чем паника посреди боя.
func (s *BehaviorSet) For(temper, family string) Behavior {
	key := temper + "/" + family
	if b, ok := s.cache[key]; ok {
		return b
	}
	merged := deepCopy(s.Defaults)
	if m, ok := s.Profiles[temper].(map[string]any); ok {
		mergeInto(merged, m)
	}
	if m, ok := s.Families[family].(map[string]any); ok {
		mergeInto(merged, m)
	}

	var out Behavior
	raw, err := json.Marshal(strip(merged))
	if err == nil {
		_ = json.Unmarshal(raw, &out)
	}
	s.cache[key] = out
	return out
}

// TemperIDs — характеры в стабильном порядке.
func (s *BehaviorSet) TemperIDs() []string { return sortedKeysAny(s.Profiles) }

// FamilyIDs — семьи в стабильном порядке.
func (s *BehaviorSet) FamilyIDs() []string { return sortedKeysAny(s.Families) }

// Validate ловит профиль, при котором ИИ работать не сможет: слепой враг,
// нулевые тайминги, отрицательные доли.
func (s *BehaviorSet) Validate() []string {
	var probs []string
	add := func(f string, a ...any) { probs = append(probs, fmt.Sprintf(f, a...)) }
	if len(s.Defaults) == 0 {
		add("нет блока defaults — сливать нечего")
	}
	if len(s.Profiles) == 0 {
		add("нет ни одного профиля")
	}
	for _, temper := range s.TemperIDs() {
		for _, family := range append([]string{""}, s.FamilyIDs()...) {
			b := s.For(temper, family)
			who := temper
			if family != "" {
				who += "/" + family
			}
			switch {
			case b.Perception.FOVDeg <= 0 || b.Perception.FOVDeg > 360:
				add("%s: fov_deg=%.0f вне 0..360", who, b.Perception.FOVDeg)
			case b.Perception.ReactionTicks < 0:
				add("%s: отрицательная задержка реакции", who)
			case b.Perception.LoseTicks <= 0:
				add("%s: lose_ticks=0 — цель теряется мгновенно, бой можно обнулять углом", who)
			case b.Perception.MemoryTicks < b.Perception.LoseTicks:
				add("%s: memory_ticks меньше lose_ticks — враг забывает раньше, чем перестаёт видеть", who)
			case b.Combat.Strafe < 0 || b.Combat.Strafe > 1:
				add("%s: strafe=%.2f вне 0..1", who, b.Combat.Strafe)
			case b.Combat.AttackSlots < 1:
				add("%s: attack_slots < 1 — бить не будет никто", who)
			case b.Combat.CommitTicks <= 0:
				add("%s: commit_ticks=0 — решение будет дрожать каждый тик", who)
			case b.Group.Flank < 0 || b.Group.Flank > 1:
				add("%s: flank=%.2f вне 0..1", who, b.Group.Flank)
			case b.Group.SurroundSlots < 1:
				add("%s: surround_slots < 1", who)
			case b.Group.Spacing <= 0:
				add("%s: spacing=0 — свои слипнутся в одну точку", who)
			case b.Patrol.Leash <= 0:
				add("%s: leash=0 — враг не вернётся домой никогда", who)
			case b.Patrol.WaitTicks[0] > b.Patrol.WaitTicks[1]:
				add("%s: wait_ticks min > max", who)
			case b.Combat.PreferRange < 0:
				add("%s: prefer_range отрицателен", who)
			case b.Combat.Dodge < 0 || b.Combat.Dodge > 1:
				add("%s: dodge=%.2f вне 0..1", who, b.Combat.Dodge)
			case b.Combat.Dodge > 0 && b.Combat.DodgeTicks <= 0:
				add("%s: умеет отскакивать, но длина отскока нулевая", who)
			case b.Combat.Lead < 0 || b.Combat.Lead > 1:
				add("%s: lead=%.2f вне 0..1", who, b.Combat.Lead)
			case b.Combat.Dash < 0 || b.Combat.Dash > 1:
				add("%s: dash=%.2f вне 0..1", who, b.Combat.Dash)
			case b.Combat.Dash > 0 && (b.Combat.DashTicks <= 0 || b.Combat.DashScale <= 1):
				add("%s: умеет рывок, но он не быстрее обычного бега", who)
			case b.Combat.FinishHP < 0 || b.Combat.FinishHP > 1:
				add("%s: finish_hp=%.2f вне 0..1", who, b.Combat.FinishHP)
			}
		}
	}
	return probs
}

// mergeInto накладывает src поверх dst: вложенные объекты сливаются, остальное
// заменяется целиком.
func mergeInto(dst, src map[string]any) {
	for k, v := range src {
		sub, isMap := v.(map[string]any)
		cur, hadMap := dst[k].(map[string]any)
		if isMap && hadMap {
			mergeInto(cur, sub)
			continue
		}
		if isMap {
			dst[k] = deepCopy(sub)
			continue
		}
		dst[k] = v
	}
}

func deepCopy(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if sub, ok := v.(map[string]any); ok {
			out[k] = deepCopy(sub)
			continue
		}
		out[k] = v
	}
	return out
}

// strip выбрасывает ключи-комментарии ("_", "_fov_deg"): в файле они несут
// смысл для человека, а в структуру только мешают попасть.
func strip(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if strings.HasPrefix(k, "_") {
			continue
		}
		if sub, ok := v.(map[string]any); ok {
			out[k] = strip(sub)
			continue
		}
		out[k] = v
	}
	return out
}

func sortedKeysAny[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		if strings.HasPrefix(k, "_") {
			continue
		}
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
