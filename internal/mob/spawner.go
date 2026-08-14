package mob

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"math/rand/v2"
	"slices"

	"github.com/vladislav/game/internal/engine"
	"github.com/vladislav/game/internal/physics"
	"github.com/vladislav/game/internal/sprite"
)

// SpawnConfig — правила заселения карты (assets/mobs/animals/spawn.json).
// Отвечает на «сколько и где», тогда как «кто» задаётся весами биомов в
// species.json. Формат: docs/mobs/spawn.md.
type SpawnConfig struct {
	Version int `json:"version"`

	Radius struct {
		SpawnMin float64 `json:"spawn_min"` // ближе не появляются — игрок увидит
		SpawnMax float64 `json:"spawn_max"` // дальше не появляются — впустую
		Keep     float64 `json:"keep"`      // ближе этого не снимаем никогда
		Despawn  float64 `json:"despawn"`   // дальше — снимаем сразу
	} `json:"radius"`

	Limits struct {
		Global     int            `json:"global"`      // всего на карте
		Near       int            `json:"near"`        // в радиусе keep
		PerSpecies int            `json:"per_species"` // одного вида на карте
		Overrides  map[string]int `json:"per_species_overrides"`
	} `json:"limits"`

	Rate struct {
		IntervalTicks int     `json:"interval_ticks"`
		Attempts      int     `json:"attempts"`     // точек за цикл
		PlaceTries    int     `json:"place_tries"`  // попыток поставить одну особь
		GroupSpread   float64 `json:"group_spread"` // разброс внутри группы, px
	} `json:"rate"`

	InitialFill            float64 `json:"initial_fill"`             // доля global при заселении карты
	DespawnChance          float64 `json:"despawn_chance"`           // шанс снять особь за проверку
	DespawnIntervalTicks   int     `json:"despawn_interval_ticks"`   //
	ActivityMismatchWeight float64 `json:"activity_mismatch_weight"` // вес вида не в своё время суток
}

// LoadSpawnConfig читает конфиг заселения.
func LoadSpawnConfig(fsys fs.FS, p string) (*SpawnConfig, error) {
	b, err := fs.ReadFile(fsys, p)
	if err != nil {
		return nil, fmt.Errorf("mob: чтение %q: %w", p, err)
	}
	var c SpawnConfig
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("mob: разбор %q: %w", p, err)
	}
	return &c, c.Validate()
}

// Validate ловит конфиг, при котором спавн вообще не заработает: неверный
// порядок радиусов даёт пустое кольцо, нулевые лимиты — вечно пустую карту.
func (c *SpawnConfig) Validate() error {
	r := c.Radius
	switch {
	case r.SpawnMin <= 0 || r.SpawnMax <= r.SpawnMin:
		return fmt.Errorf("mob: кольцо спавна пустое: spawn_min=%.0f spawn_max=%.0f", r.SpawnMin, r.SpawnMax)
	case r.Keep < r.SpawnMax:
		return fmt.Errorf("mob: keep=%.0f меньше spawn_max=%.0f — звери будут таять сразу после появления", r.Keep, r.SpawnMax)
	case r.Despawn < r.Keep:
		return fmt.Errorf("mob: despawn=%.0f меньше keep=%.0f", r.Despawn, r.Keep)
	case c.Limits.Global <= 0 || c.Limits.Near <= 0 || c.Limits.PerSpecies <= 0:
		return fmt.Errorf("mob: нулевой лимит популяции")
	case c.Rate.IntervalTicks <= 0 || c.Rate.Attempts <= 0 || c.Rate.PlaceTries <= 0:
		return fmt.Errorf("mob: нулевой темп спавна")
	case c.InitialFill < 0 || c.InitialFill > 1:
		return fmt.Errorf("mob: initial_fill=%.2f вне 0..1", c.InitialFill)
	}
	return nil
}

// cap возвращает предел численности вида.
func (c *SpawnConfig) cap(id string) int {
	if n, ok := c.Limits.Overrides[id]; ok {
		return n
	}
	return c.Limits.PerSpecies
}

// SpawnWorld — что спавнеру нужно знать про карту.
type SpawnWorld interface {
	Field() *physics.Field        // сетка проходимости: где вообще можно стоять
	Biome() string                // id биома карты (ключ в habitat.biomes)
	Zone(p engine.Vec2) string    // water|shore|meadow|woods|trail|plateau|farmyard
	Size() (w float64, h float64) // размеры мира
}

// PackFunc отдаёт спрайт-пак по имени каталога (art вида).
type PackFunc func(art string) (*sprite.Pack, error)

// Spawner держит популяцию животных вокруг игрока: подсевает в кольце за
// пределами обзора и снимает тех, кто остался далеко позади. Схема майнкрафта:
// ближе spawn_min не появляется никто (иначе видно появление из воздуха),
// дальше despawn не живёт никто (иначе карта копит зверей, которых никто не
// видит).
type Spawner struct {
	cfg   *SpawnConfig
	cat   *Catalog
	world SpawnWorld
	table *Table
	packs PackFunc
	rng   *rand.Rand

	animals []*Animal
	counts  map[string]int // живых по видам
	tick    int
	errs    []string // виды, чьи паки не загрузились (сообщается один раз)
}

// NewSpawner собирает спавнер для карты w.
func NewSpawner(cfg *SpawnConfig, cat *Catalog, w SpawnWorld, packs PackFunc, rng *rand.Rand) *Spawner {
	return &Spawner{
		cfg: cfg, cat: cat, world: w, packs: packs, rng: rng,
		table:  BuildTable(cat, w.Biome()),
		counts: map[string]int{},
	}
}

// Animals — живая популяция (для отрисовки и попаданий).
func (s *Spawner) Animals() []*Animal { return s.animals }

// Errors — виды, которых не удалось загрузить.
func (s *Spawner) Errors() []string { return s.errs }

// Populate заселяет карту целиком при её создании — как в майнкрафте, где
// мирные звери расставляются при генерации, а не подсеваются потом. Без этого
// игрок стартует на пустой карте и первые полминуты видит, как она наполняется.
func (s *Spawner) Populate() {
	target := int(float64(s.cfg.Limits.Global) * s.cfg.InitialFill)
	w, h := s.world.Size()
	for tries := 0; len(s.animals) < target && tries < target*40; tries++ {
		p := engine.Vec2{X: s.rng.Float64() * w, Y: s.rng.Float64() * h}
		s.trySpawnAt(p, false)
	}
}

// Update — один тик: спавн по расписанию, снятие далёких, обновление всех.
// night приходит снаружи: часов в игре пока нет, а поведение видов от времени
// суток уже зависит (species.activity).
func (s *Spawner) Update(player engine.Vec2, hasPlayer, night bool) {
	s.tick++

	if hasPlayer && s.tick%s.cfg.Rate.IntervalTicks == 0 {
		s.spawnCycle(player, night)
	}
	if hasPlayer && s.tick%max(s.cfg.DespawnIntervalTicks, 1) == 0 {
		s.despawnFar(player)
	}

	ctx := Ctx{Field: s.world.Field(), Threat: player, HasThreat: hasPlayer}
	live := s.animals[:0]
	for _, a := range s.animals {
		ctx.Prey = s.nearestPrey(a)
		a.Update(ctx)
		if a.Gone() {
			s.counts[a.Species.ID]--
			continue
		}
		live = append(live, a)
	}
	s.animals = live
}

// spawnCycle — цикл подсева: несколько случайных точек в кольце вокруг игрока.
func (s *Spawner) spawnCycle(player engine.Vec2, night bool) {
	if len(s.animals) >= s.cfg.Limits.Global {
		return
	}
	if s.countNear(player) >= s.cfg.Limits.Near {
		return
	}
	r := s.cfg.Radius
	for range s.cfg.Rate.Attempts {
		ang := s.rng.Float64() * 2 * math.Pi
		dist := r.SpawnMin + s.rng.Float64()*(r.SpawnMax-r.SpawnMin)
		p := player.Add(engine.Vec2{X: math.Cos(ang) * dist, Y: math.Sin(ang) * dist})
		if s.trySpawnAt(p, night) {
			return // одна группа за цикл — иначе карта наполняется рывком
		}
	}
}

// trySpawnAt пытается высадить группу в точке p. Порядок как в майнкрафте:
// сначала место, потом житель — вид подбирается под зону, а не наоборот.
func (s *Spawner) trySpawnAt(p engine.Vec2, night bool) bool {
	if !s.placeable(p) {
		return false
	}
	sp := s.table.RollZone(s.rng, s.world.Zone(p), night, s.cfg.ActivityMismatchWeight)
	if sp == nil || !s.hasRoom(sp) {
		return false
	}
	g := s.table.GroupFor(s.cat, s.rng, sp)
	if g == nil {
		return false
	}
	n := s.placeGroup(g.Species, g.Count, p)
	if g.Young != nil {
		n += s.placeGroup(g.Young, g.YoungCount, p)
	}
	return n > 0
}

// placeGroup расставляет count особей вида вокруг центра.
func (s *Spawner) placeGroup(sp *Species, count int, center engine.Vec2) int {
	pack, err := s.packs(sp.Art)
	if err != nil {
		s.noteErr(sp.ID, err)
		return 0
	}
	spread := s.cfg.Rate.GroupSpread
	placed := 0
	for range count {
		if !s.hasRoom(sp) || len(s.animals) >= s.cfg.Limits.Global {
			break
		}
		for range s.cfg.Rate.PlaceTries {
			p := center.Add(engine.Vec2{
				X: (s.rng.Float64()*2 - 1) * spread,
				Y: (s.rng.Float64()*2 - 1) * spread,
			})
			if !s.placeable(p) || !Fits(sp, s.world.Zone(p)) {
				continue
			}
			// Тело должно поместиться целиком: иначе зверь родится по пояс в
			// воде или в скале и первым же тиком начнёт выдавливаться наружу.
			f := s.world.Field()
			if !f.Fits(p, bodyOf(sp, pack, f.CellAt(p).Floor())) {
				continue
			}
			a := NewAnimal(sp, pack, p, s.rng)
			a.Land(f)
			s.animals = append(s.animals, a)
			s.counts[sp.ID]++
			placed++
			break
		}
	}
	return placed
}

// placeable — годится ли точка под подсев вообще: она в границах мира и не в
// скале. Поместится ли туда конкретный вид — решают Fits (зона обитания) и
// поле (тело целиком), уже с видом и паком на руках.
func (s *Spawner) placeable(p engine.Vec2) bool {
	w, h := s.world.Size()
	if p.X < 0 || p.Y < 0 || p.X >= w || p.Y >= h {
		return false
	}
	return s.world.Field().CellAt(p) != physics.Solid
}

func (s *Spawner) hasRoom(sp *Species) bool {
	return s.counts[sp.ID] < s.cfg.cap(sp.ID)
}

func (s *Spawner) countNear(player engine.Vec2) int {
	n := 0
	for _, a := range s.animals {
		if engine.Dist(a.Pos, player) <= s.cfg.Radius.Keep {
			n++
		}
	}
	return n
}

// despawnFar снимает тех, кто остался далеко: за despawn — сразу, между keep и
// despawn — по шансу, чтобы популяция рассасывалась постепенно, а не хлопком.
// Прирученные и помеченные не снимаются никогда.
func (s *Spawner) despawnFar(player engine.Vec2) {
	live := s.animals[:0]
	for _, a := range s.animals {
		d := engine.Dist(a.Pos, player)
		drop := false
		switch {
		case a.Persistent || d <= s.cfg.Radius.Keep:
		case d > s.cfg.Radius.Despawn:
			drop = true
		default:
			drop = s.rng.Float64() < s.cfg.DespawnChance
		}
		if drop {
			s.counts[a.Species.ID]--
			continue
		}
		live = append(live, a)
	}
	s.animals = live
}

// nearestPrey ищет хищнику ближайшую жертву из его списка prey. Знание про
// соседей живёт здесь: сам зверь про популяцию ничего не знает.
func (s *Spawner) nearestPrey(a *Animal) *Animal {
	if len(a.Species.Threat.Prey) == 0 || !a.Alive() {
		return nil
	}
	var best *Animal
	bestD := a.Species.Threat.Sight
	for _, o := range s.animals {
		if o == a || !o.Alive() {
			continue
		}
		if !slices.Contains(a.Species.Threat.Prey, o.Species.ID) {
			continue
		}
		if o.Floor() != a.Floor() {
			continue // добыча на другом этаже: до неё не добраться
		}
		if d := engine.Dist(a.Pos, o.Pos); d < bestD {
			best, bestD = o, d
		}
	}
	return best
}

func (s *Spawner) noteErr(id string, err error) {
	msg := fmt.Sprintf("%s: %v", id, err)
	if !slices.Contains(s.errs, msg) {
		s.errs = append(s.errs, msg)
	}
}
