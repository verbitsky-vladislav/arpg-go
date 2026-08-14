package mob

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"math/rand/v2"
)

// Правила заселения карты врагами: assets/mobs/enemies/spawn.json.
// Формат: docs/mobs/enemies_spawn.md.
//
// Отличие от животных одно, но определяющее: население меряется не головами, а
// опасностью. Один демон и восемь крыс — не одно и то же, и «сорок особей на
// карту» означало бы то легкую прогулку, то бойню.

// EnemySpawnConfig — конфиг заселения врагами.
type EnemySpawnConfig struct {
	Version int `json:"version"`

	Radius struct {
		SpawnMin float64 `json:"spawn_min"`
		SpawnMax float64 `json:"spawn_max"`
		Keep     float64 `json:"keep"`
		Despawn  float64 `json:"despawn"`
	} `json:"radius"`

	Safe struct {
		// Radius — радиус тишины вокруг точки старта.
		Radius float64 `json:"radius"`
	} `json:"safe"`

	Danger struct {
		Budget     float64 `json:"budget"`
		NearBudget float64 `json:"near_budget"`
		NightScale float64 `json:"night_scale"`
	} `json:"danger"`

	Limits struct {
		Global    int            `json:"global"`
		Near      int            `json:"near"`
		PerType   int            `json:"per_type"`
		Overrides map[string]int `json:"per_type_overrides"`
	} `json:"limits"`

	Tiers struct {
		Weights     map[string]int     `json:"weights"`
		BiomeShift  map[string]float64 `json:"biome_shift"`
		SlimeColors map[string]int     `json:"slime_colors"`
	} `json:"tiers"`

	Rate struct {
		IntervalTicks int     `json:"interval_ticks"`
		Attempts      int     `json:"attempts"`
		PlaceTries    int     `json:"place_tries"`
		GroupSpread   float64 `json:"group_spread"`
		MinGroupGap   float64 `json:"min_group_gap"`
	} `json:"rate"`

	Elite struct {
		Chance      float64 `json:"chance"`
		HPScale     float64 `json:"hp_scale"`
		DamageScale float64 `json:"damage_scale"`
		XPScale     float64 `json:"xp_scale"`
		SpeedScale  float64 `json:"speed_scale"`
		StealBonus  float64 `json:"steal_bonus"`
	} `json:"elite"`

	InitialFill            float64 `json:"initial_fill"`
	DespawnChance          float64 `json:"despawn_chance"`
	DespawnIntervalTicks   int     `json:"despawn_interval_ticks"`
	ActivityMismatchWeight float64 `json:"activity_mismatch_weight"`
}

// LoadEnemySpawn читает конфиг заселения врагами.
func LoadEnemySpawn(fsys fs.FS, p string) (*EnemySpawnConfig, error) {
	b, err := fs.ReadFile(fsys, p)
	if err != nil {
		return nil, fmt.Errorf("mob: чтение %q: %w", p, err)
	}
	var c EnemySpawnConfig
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("mob: разбор %q: %w", p, err)
	}
	return &c, c.Validate()
}

// Validate ловит конфиг, при котором заселение не заработает.
func (c *EnemySpawnConfig) Validate() error {
	r := c.Radius
	switch {
	case r.SpawnMin <= 0 || r.SpawnMax <= r.SpawnMin:
		return fmt.Errorf("mob: кольцо спавна пустое: spawn_min=%.0f spawn_max=%.0f", r.SpawnMin, r.SpawnMax)
	case r.Keep < r.SpawnMax:
		return fmt.Errorf("mob: keep=%.0f меньше spawn_max=%.0f — враги будут таять сразу после появления", r.Keep, r.SpawnMax)
	case r.Despawn < r.Keep:
		return fmt.Errorf("mob: despawn=%.0f меньше keep=%.0f", r.Despawn, r.Keep)
	case c.Danger.Budget <= 0 || c.Danger.NearBudget <= 0:
		return fmt.Errorf("mob: нулевой бюджет опасности")
	case c.Danger.NearBudget > c.Danger.Budget:
		return fmt.Errorf("mob: near_budget=%.0f больше общего бюджета %.0f", c.Danger.NearBudget, c.Danger.Budget)
	case c.Danger.NightScale <= 0:
		return fmt.Errorf("mob: night_scale=%.2f — ночью мир исчезнет", c.Danger.NightScale)
	case c.Limits.Global <= 0 || c.Limits.Near <= 0 || c.Limits.PerType <= 0:
		return fmt.Errorf("mob: нулевой лимит популяции")
	case len(c.Tiers.Weights) == 0:
		return fmt.Errorf("mob: не заданы веса тиров")
	case c.Rate.IntervalTicks <= 0 || c.Rate.Attempts <= 0 || c.Rate.PlaceTries <= 0:
		return fmt.Errorf("mob: нулевой темп спавна")
	case c.InitialFill < 0 || c.InitialFill > 1:
		return fmt.Errorf("mob: initial_fill=%.2f вне 0..1", c.InitialFill)
	case c.Elite.Chance < 0 || c.Elite.Chance > 1:
		return fmt.Errorf("mob: elite.chance=%.2f вне 0..1", c.Elite.Chance)
	case c.Elite.HPScale < 1 || c.Elite.DamageScale < 1:
		return fmt.Errorf("mob: элита слабее обычной особи")
	case c.DespawnChance < 0 || c.DespawnChance > 1:
		return fmt.Errorf("mob: despawn_chance=%.2f вне 0..1", c.DespawnChance)
	case c.Safe.Radius < 0:
		return fmt.Errorf("mob: safe.radius отрицателен")
	case c.Safe.Radius >= r.SpawnMin:
		// Иначе тишина съедает кольцо подсева целиком, и стоящему на старте
		// игроку не подсеется никто и никогда.
		return fmt.Errorf("mob: safe.radius=%.0f не меньше spawn_min=%.0f", c.Safe.Radius, r.SpawnMin)
	}
	for id, n := range c.Limits.Overrides {
		if n <= 0 {
			return fmt.Errorf("mob: лимит %q равен %d", id, n)
		}
	}
	for b, s := range c.Tiers.BiomeShift {
		if s < 0 || s > 1 {
			return fmt.Errorf("mob: сдвиг тиров биома %q равен %.2f, а должен быть 0..1", b, s)
		}
	}
	return nil
}

// TypeCap — предел численности типа.
func (c *EnemySpawnConfig) TypeCap(typeID string) int {
	if n, ok := c.Limits.Overrides[typeID]; ok {
		return n
	}
	return c.Limits.PerType
}

// Budget — бюджет опасности на карту с поправкой на время суток.
func (c *EnemySpawnConfig) Budget(night bool) float64 {
	if night {
		return c.Danger.Budget * c.Danger.NightScale
	}
	return c.Danger.Budget
}

// RollTier выбирает тир (или цвет — у слизней) для типа t в биоме biome.
//
// Сдвиг биома поднимает шанс старших тиров, не выкидывая младшие совсем:
// в подземелье третьего круга крысы всё ещё встречаются, просто демоны там
// не редкость. Реализовано как перевес: вес тира умножается на (1+shift)^ранг.
func (c *EnemySpawnConfig) RollTier(rng *rand.Rand, t *EnemyType, biome string) *Tier {
	ids := t.TierIDs()
	if len(ids) == 0 {
		return nil
	}
	shift := c.Tiers.BiomeShift[biome]
	weights := make([]float64, len(ids))
	total := 0.0
	for i, id := range ids {
		w := float64(c.tierWeight(id))
		if w <= 0 {
			continue
		}
		w *= math.Pow(1+shift*3, float64(rankOf(id)))
		weights[i] = w
		total += w
	}
	if total <= 0 {
		return t.Tiers[ids[0]]
	}
	n := rng.Float64() * total
	for i, w := range weights {
		if w == 0 {
			continue
		}
		if n < w {
			return t.Tiers[ids[i]]
		}
		n -= w
	}
	return t.Tiers[ids[len(ids)-1]]
}

// tierWeight — вес тира или цвета слизня. Незнакомый ключ весит как самый
// слабый: новый вариант должен появляться редко, а не пропадать молча.
func (c *EnemySpawnConfig) tierWeight(id string) int {
	if w, ok := c.Tiers.Weights[id]; ok {
		return w
	}
	if w, ok := c.Tiers.SlimeColors[id]; ok {
		return w
	}
	return 1
}

// rankOf — порядковый номер силы: t1 → 0, t2 → 1, t3 → 2. У цветов слизней
// ранга нет, поэтому сдвиг биома их не касается.
func rankOf(tierID string) int {
	switch tierID {
	case "t1":
		return 0
	case "t2":
		return 1
	case "t3":
		return 2
	}
	return 0
}
