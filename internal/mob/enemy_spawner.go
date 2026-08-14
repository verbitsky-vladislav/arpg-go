package mob

import (
	"fmt"
	"math"
	"math/rand/v2"
	"slices"

	"github.com/vladislav/game/internal/engine"
	"github.com/vladislav/game/internal/physics"
)

// navStep — сторона клетки навигации в пикселях. Грубее физической (8 px):
// огибать стену надо целиком, а вдоль неё ведёт физика.
//
// Шестнадцать, а не тридцать два: на грубой сетке центр клетки не попадает в
// узкий проход, и тело, которое физически в него пролезает, навигация считает
// запертым. Перестройка всех трёх полос карты 2048×2048 стоит 1.2 мс раз в 20
// тиков (см. BenchmarkNavRebuild) — за это можно и заплатить.
const navStep = 16

// navRebuildTicks — как часто разливать волну от игрока. За треть секунды цель
// далеко не уходит, а обход сетки стоит дороже одного шага врага.
const navRebuildTicks = 20

// EnemyTable — типы врагов биома с весами из habitat.biomes.
type EnemyTable struct {
	Biome   string
	entries []enemyEntry
}

type enemyEntry struct {
	t *EnemyType
	w int
}

// BuildEnemyTable собирает таблицу типов для биома.
func BuildEnemyTable(c *EnemyCatalog, biome string) *EnemyTable {
	t := &EnemyTable{Biome: biome}
	for _, id := range c.TypeIDs() {
		ty := c.Types[id]
		if w := ty.Habitat.Biomes[biome]; w > 0 {
			t.entries = append(t.entries, enemyEntry{t: ty, w: w})
		}
	}
	return t
}

// Empty — есть ли кого селить в этом биоме.
func (t *EnemyTable) Empty() bool { return len(t.entries) == 0 }

// Types — типы таблицы (для отладки и проверок).
func (t *EnemyTable) Types() []*EnemyType {
	out := make([]*EnemyType, 0, len(t.entries))
	for _, e := range t.entries {
		out = append(out, e.t)
	}
	return out
}

// Roll выбирает тип под зону и время суток. allow отсекает тех, кому уже тесно
// (лимит по типу): фильтр приходит снаружи, чтобы таблица не знала про счёт.
func (t *EnemyTable) Roll(rng *rand.Rand, zone string, night bool, mismatch float64, allow func(*EnemyType) bool) *EnemyType {
	weights := make([]float64, len(t.entries))
	total := 0.0
	for i, e := range t.entries {
		if !EnemyFitsZone(e.t, zone) || (allow != nil && !allow(e.t)) {
			continue
		}
		w := float64(e.w)
		if !EnemyActiveAt(e.t, night) {
			w *= mismatch
		}
		if w <= 0 {
			continue
		}
		weights[i], total = w, total+w
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
			return t.entries[i].t
		}
		n -= w
	}
	return nil
}

// EnemyFitsZone — водится ли тип в такой зоне карты.
func EnemyFitsZone(t *EnemyType, zone string) bool {
	return slices.Contains(t.Habitat.Zones, zone)
}

// EnemyActiveAt — бодрствует ли тип в это время суток.
func EnemyActiveAt(t *EnemyType, night bool) bool {
	switch t.Activity {
	case "day":
		return !night
	case "night":
		return night
	}
	return true
}

// EnemySpawner держит население врагов вокруг игрока.
//
// От спавнера зверей отличается тем, что считает не головы, а опасность: место
// на карте занимает не «одна особь», а её опыт. Восемь крыс и один демон крови
// стоят одинаково, и карта не превращается ни в тир, ни в бойню.
//
// Он же держит общий отряд (перекличка, очередь удара, роли) и общую карту
// навигации: и то и другое имеет смысл только для всех врагов сразу.
type EnemySpawner struct {
	cfg   *EnemySpawnConfig
	cat   *EnemyCatalog
	bhv   *BehaviorSet
	world SpawnWorld
	table *EnemyTable
	packs PackFunc
	rng   *rand.Rand

	squad  *Squad
	nav    *NavField
	safe   engine.Vec2 // точка старта: вокруг неё врагов не заводим
	guard  bool
	counts map[string]int // живых по типам
	danger float64        // занятая опасность
	tick   int
	errs   []string
}

// NewEnemySpawner собирает спавнер врагов для карты w.
func NewEnemySpawner(cfg *EnemySpawnConfig, cat *EnemyCatalog, bhv *BehaviorSet,
	w SpawnWorld, packs PackFunc, rng *rand.Rand) *EnemySpawner {
	return &EnemySpawner{
		cfg: cfg, cat: cat, bhv: bhv, world: w, packs: packs, rng: rng,
		table:  BuildEnemyTable(cat, w.Biome()),
		squad:  NewSquad(),
		nav:    NewNav(w.Field(), navStep),
		counts: map[string]int{},
	}
}

// Guard объявляет точку старта тихой: в радиусе safe.radius враги не заводятся
// ни при заселении, ни подсевом.
//
// Именно не заводятся, а не «не заходят»: враг, погнавшийся за игроком, добежит
// и до старта — это его работа, и мешать ей нельзя. Тишина здесь означает лишь
// то, что мир не встречает игрока толпой на первом же кадре.
func (s *EnemySpawner) Guard(p engine.Vec2) { s.safe, s.guard = p, true }

// Enemies — живое население (для отрисовки и попаданий).
func (s *EnemySpawner) Enemies() []*Enemy { return s.squad.Members() }

// Squad — общий штаб: игровому слою он нужен, чтобы показывать роли в отладке.
func (s *EnemySpawner) Squad() *Squad { return s.squad }

// Nav — общая карта навигации (отладка и подсказки).
func (s *EnemySpawner) Nav() *NavField { return s.nav }

// Danger — занятая сейчас опасность.
func (s *EnemySpawner) Danger() float64 { return s.danger }

// Errors — типы, чьи паки не загрузились.
func (s *EnemySpawner) Errors() []string { return s.errs }

// Populate заселяет карту при её создании: часть бюджета сразу, чтобы игрок не
// стартовал в пустом мире и не смотрел, как тот наполняется.
func (s *EnemySpawner) Populate(night bool) {
	want := s.cfg.Budget(night) * s.cfg.InitialFill
	w, h := s.world.Size()
	for tries := 0; s.danger < want && tries < 400; tries++ {
		p := engine.Vec2{X: s.rng.Float64() * w, Y: s.rng.Float64() * h}
		s.trySpawnAt(p, night)
	}
}

// Update — один тик населения: подсев, снятие далёких, общая карта навигации и
// обновление каждого врага.
func (s *EnemySpawner) Update(t Target, hasPlayer bool, playerFloor uint8, night bool) {
	s.tick++

	if hasPlayer && s.tick%s.interval(t.Pos) == 0 {
		s.spawnCycle(t.Pos, night)
	}
	if hasPlayer && s.tick%max(s.cfg.DespawnIntervalTicks, 1) == 0 {
		s.despawnFar(t.Pos)
	}
	// Волна от игрока: одна на всех и не каждый тик.
	if hasPlayer && s.tick%navRebuildTicks == 0 {
		s.nav.Rebuild(t.Pos, playerFloor)
	}

	ctx := EnemyCtx{
		Field:     s.world.Field(),
		Player:    t,
		HasPlayer: hasPlayer,
		Squad:     s.squad,
		Nav:       s.nav,
	}
	for _, e := range s.squad.Members() {
		e.Update(ctx)
	}
	s.collect()
}

// interval — через сколько тиков следующая попытка подсева.
//
// Там, где игрок всех вырезал, мир восстанавливается быстрее: иначе выгодной
// тактикой становится «выкосить пятачок и жить на нём», а зачищенная местность
// на полминуты превращается в безопасный коридор. Спокойный темп остаётся там,
// где и так есть кого встретить.
func (s *EnemySpawner) interval(player engine.Vec2) int {
	r := s.cfg.Rate
	if s.nearDanger(player) < s.cfg.Danger.NearBudget*r.EmptyShare {
		return max(1, r.EmptyIntervalTicks)
	}
	return max(1, r.IntervalTicks)
}

// collect убирает исчезнувших и освобождает их место в бюджете.
func (s *EnemySpawner) collect() {
	for _, e := range s.squad.Members() {
		if !e.Gone() {
			continue
		}
		s.counts[e.Type.ID]--
		s.danger -= e.Danger()
	}
	if s.danger < 0 {
		s.danger = 0
	}
	s.squad.Prune()
}

// spawnCycle — подсев в кольце за пределами обзора.
func (s *EnemySpawner) spawnCycle(player engine.Vec2, night bool) {
	if s.danger >= s.cfg.Budget(night) || len(s.squad.Members()) >= s.cfg.Limits.Global {
		return
	}
	if s.nearDanger(player) >= s.cfg.Danger.NearBudget || s.countNear(player) >= s.cfg.Limits.Near {
		return
	}
	r := s.cfg.Radius
	for range s.cfg.Rate.Attempts {
		ang := s.rng.Float64() * 2 * math.Pi
		dist := r.SpawnMin + s.rng.Float64()*(r.SpawnMax-r.SpawnMin)
		p := player.Add(engine.Vec2{X: math.Cos(ang) * dist, Y: math.Sin(ang) * dist})
		if s.trySpawnAt(p, night) {
			return // одна группа за цикл: иначе карта наполняется рывком
		}
	}
}

// trySpawnAt пытается высадить группу в точке. Порядок как у зверей: сначала
// место, потом житель — тип подбирается под зону, а не наоборот.
func (s *EnemySpawner) trySpawnAt(p engine.Vec2, night bool) bool {
	if !s.placeable(p) || s.crowded(p) {
		return false
	}
	zone := s.world.Zone(p)
	ty := s.table.Roll(s.rng, zone, night, s.cfg.ActivityMismatchWeight, s.hasRoom)
	if ty == nil {
		return false
	}
	tier := s.cfg.RollTier(s.rng, ty, s.table.Biome)
	if tier == nil {
		return false
	}
	pack, err := s.packs(ty.Art + "/" + tier.ID)
	if err != nil {
		s.noteErr(ty.ID, err)
		return false
	}

	g := ty.Habitat.Group
	count := g.Min
	if g.Max > g.Min {
		count += s.rng.IntN(g.Max - g.Min + 1)
	}
	bhv := s.bhv.For(ty.Temper, ty.Family)

	placed := 0
	for range max(1, count) {
		if !s.hasRoom(ty) || s.danger >= s.cfg.Budget(night) {
			break
		}
		at, ok := s.spot(p)
		if !ok {
			continue
		}
		e := NewEnemy(tier, pack, bhv, at, s.rng)
		e.Land(s.world.Field())
		if s.rng.Float64() < s.cfg.Elite.Chance {
			e.MakeElite(s.cfg.Elite.HPScale, s.cfg.Elite.DamageScale, s.cfg.Elite.XPScale)
		}
		s.squad.Add(e)
		s.counts[ty.ID]++
		s.danger += e.Danger()
		placed++
	}
	return placed > 0
}

// spot ищет место для одной особи около центра группы.
func (s *EnemySpawner) spot(center engine.Vec2) (engine.Vec2, bool) {
	spread := s.cfg.Rate.GroupSpread
	for range s.cfg.Rate.PlaceTries {
		p := center.Add(engine.Vec2{
			X: (s.rng.Float64()*2 - 1) * spread,
			Y: (s.rng.Float64()*2 - 1) * spread,
		})
		if s.placeable(p) {
			return p, true
		}
	}
	return engine.Vec2{}, false
}

// placeable — можно ли вообще кого-то ставить в точку.
func (s *EnemySpawner) placeable(p engine.Vec2) bool {
	w, h := s.world.Size()
	if p.X < 0 || p.Y < 0 || p.X >= w || p.Y >= h {
		return false
	}
	if s.guard && engine.Dist(p, s.safe) < s.cfg.Safe.Radius {
		return false
	}
	f := s.world.Field()
	if f == nil {
		return true
	}
	_, ok := f.Place(p, s.walkerBody(p), 0)
	return ok
}

// walkerBody — типовое тело ходока для проверки места: спавнер выбирает точку
// до того, как особь создана, и мерить её собственным телом ещё нечем.
func (s *EnemySpawner) walkerBody(p engine.Vec2) physics.Body {
	return physics.Body{Radius: 8, Floor: s.world.Field().CellAt(p).Floor()}
}

// crowded — не слишком ли близко уже стоит другая группа. Без этого подсев
// лепит толпу в одном углу: игрок либо не встречает никого, либо всех сразу.
func (s *EnemySpawner) crowded(p engine.Vec2) bool {
	gap := s.cfg.Rate.MinGroupGap
	if gap <= 0 {
		return false
	}
	for _, e := range s.squad.Members() {
		if e.Alive() && engine.Dist(e.Pos, p) < gap {
			return true
		}
	}
	return false
}

func (s *EnemySpawner) hasRoom(t *EnemyType) bool {
	return s.counts[t.ID] < s.cfg.TypeCap(t.ID)
}

func (s *EnemySpawner) countNear(player engine.Vec2) int {
	n := 0
	for _, e := range s.squad.Members() {
		if e.Alive() && engine.Dist(e.Pos, player) <= s.cfg.Radius.Keep {
			n++
		}
	}
	return n
}

func (s *EnemySpawner) nearDanger(player engine.Vec2) float64 {
	var d float64
	for _, e := range s.squad.Members() {
		if e.Alive() && engine.Dist(e.Pos, player) <= s.cfg.Radius.Keep {
			d += e.Danger()
		}
	}
	return d
}

// despawnFar снимает тех, кто остался далеко: за despawn — сразу, между keep и
// despawn — по шансу, чтобы население рассасывалось, а не хлопком.
//
// Тот, кто уже дерётся, не снимается никогда: исчезнуть на глазах у игрока —
// худшее, что может сделать спавнер.
func (s *EnemySpawner) despawnFar(player engine.Vec2) {
	for _, e := range s.squad.Members() {
		if !e.Alive() || e.Engaged() {
			continue
		}
		d := engine.Dist(e.Pos, player)
		drop := false
		switch {
		case d <= s.cfg.Radius.Keep:
		case d > s.cfg.Radius.Despawn:
			drop = true
		default:
			drop = s.rng.Float64() < s.cfg.DespawnChance
		}
		if drop {
			e.Vanish()
		}
	}
	s.collect()
}

func (s *EnemySpawner) noteErr(id string, err error) {
	msg := fmt.Sprintf("%s: %v", id, err)
	if !slices.Contains(s.errs, msg) {
		s.errs = append(s.errs, msg)
	}
}
