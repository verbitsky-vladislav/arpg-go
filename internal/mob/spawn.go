package mob

import (
	"math/rand/v2"
	"slices"
)

// Table — таблица спавна одного биома: виды с весами, взятыми из
// habitat.biomes. Строится один раз на карту, дальше только броски.
//
// Самостоятельно спавнятся только те, у кого spawn.with пуст: детёныши
// приходят вместе со взрослым (см. RollGroup), а не сами по себе.
type Table struct {
	Biome   string
	entries []tableEntry
	total   int
}

type tableEntry struct {
	sp     *Species
	weight int
}

// Group — одна пачка на спавн: сколько взрослых и какой при них выводок.
type Group struct {
	Species    *Species
	Count      int
	Young      *Species // nil, если детёныша у вида нет
	YoungCount int
}

// BuildTable собирает таблицу спавна для биома.
func BuildTable(c *Catalog, biome string) *Table {
	t := &Table{Biome: biome}
	for _, id := range c.IDs() {
		sp := c.Species[id]
		w := sp.Habitat.Biomes[biome]
		if w <= 0 || sp.Spawn.With != "" {
			continue
		}
		t.entries = append(t.entries, tableEntry{sp: sp, weight: w})
		t.total += w
	}
	return t
}

// Empty — есть ли кого спавнить.
func (t *Table) Empty() bool { return t.total == 0 }

// Species — виды таблицы (для отладки и проверок).
func (t *Table) Species() []*Species {
	out := make([]*Species, 0, len(t.entries))
	for _, e := range t.entries {
		out = append(out, e.sp)
	}
	return out
}

// Roll выбирает вид по весам. nil — таблица пуста.
func (t *Table) Roll(rng *rand.Rand) *Species {
	if t.total == 0 {
		return nil
	}
	n := rng.IntN(t.total)
	for _, e := range t.entries {
		if n < e.weight {
			return e.sp
		}
		n -= e.weight
	}
	return t.entries[len(t.entries)-1].sp
}

// RollZone выбирает вид, которому подходит конкретная точка карты: сначала
// точка, потом жилец — как в майнкрафте, где условия спавна проверяются для
// уже выбранного места. Вид, активный не в это время суток, не запрещён, а
// придавлен весом mismatch (0 — запрещён совсем).
func (t *Table) RollZone(rng *rand.Rand, zone string, night bool, mismatch float64) *Species {
	var total float64
	weights := make([]float64, len(t.entries))
	for i, e := range t.entries {
		if !Fits(e.sp, zone) {
			continue
		}
		w := float64(e.weight)
		if !ActiveAt(e.sp, night) {
			w *= mismatch
		}
		if w <= 0 {
			continue
		}
		weights[i] = w
		total += w
	}
	if total <= 0 {
		return nil
	}
	n := rng.Float64() * total
	for i, w := range weights {
		if w == 0 {
			continue
		}
		if n < w {
			return t.entries[i].sp
		}
		n -= w
	}
	return nil
}

// RollGroup выбирает вид, размер группы и выводок при нём. Стадность живёт в
// данных (spawn.group), поэтому овцы приходят по шесть, а лиса одна.
func (t *Table) RollGroup(c *Catalog, rng *rand.Rand) *Group {
	return t.GroupFor(c, rng, t.Roll(rng))
}

// GroupFor собирает пачку вокруг уже выбранного вида.
func (t *Table) GroupFor(c *Catalog, rng *rand.Rand, sp *Species) *Group {
	if sp == nil {
		return nil
	}
	g := &Group{Species: sp, Count: rollRange(rng, sp.Spawn.Group.Min, sp.Spawn.Group.Max)}
	if y := c.Get(sp.YoungForm); y != nil && y.Spawn.With == sp.ID && y.Habitat.Biomes[t.Biome] > 0 {
		if rng.IntN(2) == 0 { // выводок при взрослом — не каждый раз
			g.Young = y
			g.YoungCount = rollRange(rng, y.Spawn.Group.Min, y.Spawn.Group.Max)
		}
	}
	return g
}

func rollRange(rng *rand.Rand, lo, hi int) int {
	if hi < lo {
		lo, hi = hi, lo
	}
	if lo < 1 {
		lo = 1
	}
	if hi <= lo {
		return lo
	}
	return lo + rng.IntN(hi-lo+1)
}

// AllowsZone — можно ли спавнить вид в зоне карты (water, shore, meadow,
// woods, trail, plateau, farmyard). Зоны совпадают со слоями worldgen, так что
// спавнеру не нужен собственный разбор карты.
func (s *Species) AllowsZone(zone string) bool {
	if slices.Contains(s.Habitat.Avoid, zone) {
		return false
	}
	return slices.Contains(s.Habitat.Zones, zone)
}

// Fits — полное условие места для вида: зона плюс правило поселения. Домашние
// не заводятся в чистом поле, только во дворе; пока worldgen не размечает зону
// farmyard, в дикой карте появляются одни дикие — это ожидаемо, а не поломка.
func Fits(s *Species, zone string) bool {
	if s.Habitat.NeedsSettlement && zone != "farmyard" {
		return false
	}
	return s.AllowsZone(zone)
}

// ActiveAt — бодрствует ли вид в это время суток.
func ActiveAt(s *Species, night bool) bool {
	switch s.Activity {
	case "day":
		return !night
	case "night":
		return night
	default:
		return true
	}
}
