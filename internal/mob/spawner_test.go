package mob_test

import (
	"math"
	"os"
	"slices"
	"testing"

	"github.com/vladislav/game/internal/assets"
	"github.com/vladislav/game/internal/engine"
	"github.com/vladislav/game/internal/mob"
	"github.com/vladislav/game/internal/sprite"
)

const worldSide = 2048

// forestWorld — карта под тесты: остров травы с озером в правом верхнем углу и
// полосой леса слева. Зоны те же, что размечает worldgen.
type forestWorld struct{ biome string }

func (forestWorld) Walkable(p engine.Vec2) bool {
	return p.X >= 0 && p.Y >= 0 && p.X < worldSide && p.Y < worldSide
}

func (forestWorld) Water(p engine.Vec2) bool {
	return p.X > 1600 && p.Y < 400
}

func (w forestWorld) Zone(p engine.Vec2) string {
	switch {
	case w.Water(p):
		return "water"
	case p.X > 1500 && p.Y < 500:
		return "shore"
	case p.X < 500:
		return "woods"
	default:
		return "meadow"
	}
}

func (w forestWorld) Biome() string          { return w.biome }
func (forestWorld) Size() (float64, float64) { return worldSide, worldSide }

func spawner(t *testing.T, biome string) (*mob.Spawner, *mob.SpawnConfig) {
	t.Helper()
	l := assets.NewLoader(os.DirFS(assetsRoot))
	cat, err := mob.LoadSpecies(l.FS(), animalsDir+"/species.json")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := mob.LoadSpawnConfig(l.FS(), animalsDir+"/spawn.json")
	if err != nil {
		t.Fatal(err)
	}
	packs := func(art string) (*sprite.Pack, error) { return sprite.Load(l, animalsDir+"/"+art) }
	return mob.NewSpawner(cfg, cat, forestWorld{biome: biome}, packs, newRNG()), cfg
}

// TestSpawnConfigValid — конфиг из ассетов проходит собственные проверки.
func TestSpawnConfigValid(t *testing.T) {
	_, cfg := spawner(t, "forest")
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	// Кольцо спавна обязано лежать за пределами обзора, иначе игрок увидит
	// появление зверя из воздуха. Полудиагональ экрана 640x360.
	view := math.Hypot(640, 360) / 2
	if cfg.Radius.SpawnMin <= view {
		t.Errorf("spawn_min=%.0f не больше полудиагонали обзора %.0f", cfg.Radius.SpawnMin, view)
	}
}

// TestPopulateFillsMap — начальное заселение доводит карту до доли global и не
// превышает ни один лимит.
func TestPopulateFillsMap(t *testing.T) {
	s, cfg := spawner(t, "forest")
	s.Populate()
	got := len(s.Animals())
	want := int(float64(cfg.Limits.Global) * cfg.InitialFill)
	if got == 0 {
		t.Fatalf("карта осталась пустой (ошибки: %v)", s.Errors())
	}
	if got > cfg.Limits.Global {
		t.Errorf("заселено %d при лимите %d", got, cfg.Limits.Global)
	}
	if got < want/2 {
		t.Errorf("заселено %d, ожидалось около %d", got, want)
	}
	perSpecies := map[string]int{}
	for _, a := range s.Animals() {
		perSpecies[a.Species.ID]++
	}
	for id, n := range perSpecies {
		if lim := cfgCap(cfg, id); n > lim {
			t.Errorf("%s: %d особей при лимите %d", id, n, lim)
		}
	}
}

func cfgCap(cfg *mob.SpawnConfig, id string) int {
	if n, ok := cfg.Limits.Overrides[id]; ok {
		return n
	}
	return cfg.Limits.PerSpecies
}

// TestNoSettlementAnimalsInWild — вид, которому нужен двор, в дикой карте не
// заводится: зону farmyard worldgen пока не размечает. Домашние без этого
// требования (утки на лесном озере) появляться могут — это задумано.
func TestNoSettlementAnimalsInWild(t *testing.T) {
	s, _ := spawner(t, "forest")
	s.Populate()
	wild := 0
	for _, a := range s.Animals() {
		if a.Species.Habitat.NeedsSettlement {
			t.Errorf("%s: требует поселения, но завёлся в лесу", a.Species.ID)
		}
		if a.Species.Wild() {
			wild++
		}
	}
	if wild == 0 {
		t.Error("в лесу не завелось ни одного дикого вида")
	}
}

// TestSpawnRing — подсев идёт строго в кольце вокруг игрока: ближе spawn_min
// (видно появление) и дальше spawn_max (впустую) никто не появляется.
func TestSpawnRing(t *testing.T) {
	s, cfg := spawner(t, "forest")
	player := engine.Vec2{X: worldSide / 2, Y: worldSide / 2}
	seen := map[*mob.Animal]bool{}
	spawned := 0
	// Разброс группы уводит особей от точки броска, поэтому допуск — spread.
	slack := cfg.Rate.GroupSpread + 1
	for range 4000 {
		s.Update(player, true, false)
		for _, a := range s.Animals() {
			if seen[a] {
				continue
			}
			seen[a] = true
			spawned++
			d := engine.Dist(a.Pos, player)
			if d < cfg.Radius.SpawnMin-slack || d > cfg.Radius.SpawnMax+slack {
				t.Fatalf("%s появился на %.0f, кольцо %.0f..%.0f",
					a.Species.ID, d, cfg.Radius.SpawnMin, cfg.Radius.SpawnMax)
			}
		}
	}
	if spawned == 0 {
		t.Fatal("за 4000 тиков никто не появился")
	}
}

// TestCapsHold — популяция не растёт бесконечно: лимиты держат и общий счёт, и
// число вокруг игрока.
func TestCapsHold(t *testing.T) {
	s, cfg := spawner(t, "forest")
	player := engine.Vec2{X: worldSide / 2, Y: worldSide / 2}
	s.Populate()
	maxNear := 0
	for range 8000 {
		s.Update(player, true, false)
		if n := len(s.Animals()); n > cfg.Limits.Global {
			t.Fatalf("популяция %d при лимите %d", n, cfg.Limits.Global)
		}
		near := 0
		for _, a := range s.Animals() {
			if engine.Dist(a.Pos, player) <= cfg.Radius.Keep {
				near++
			}
		}
		maxNear = max(maxNear, near)
	}
	// Цикл спавна останавливается на near, но уже живущие могут подойти сами —
	// поэтому проверяем не точный предел, а что он не превышен кратно.
	if maxNear > cfg.Limits.Near*2 {
		t.Errorf("вокруг игрока скопилось %d при лимите %d", maxNear, cfg.Limits.Near)
	}
}

// TestDespawnFar — ушедшие далеко снимаются, а помеченные Persistent остаются.
func TestDespawnFar(t *testing.T) {
	s, cfg := spawner(t, "forest")
	s.Populate()
	if len(s.Animals()) == 0 {
		t.Fatal("карта пуста")
	}
	s.Animals()[0].Persistent = true
	kept := s.Animals()[0]

	// Игрок в углу: почти вся карта оказывается дальше despawn.
	corner := engine.Vec2{X: 0, Y: 0}
	for range 6000 {
		s.Update(corner, true, false)
	}
	for _, a := range s.Animals() {
		d := engine.Dist(a.Pos, corner)
		if a != kept && d > cfg.Radius.Despawn {
			t.Errorf("%s остался на %.0f при despawn %.0f", a.Species.ID, d, cfg.Radius.Despawn)
		}
	}
	if !slices.Contains(s.Animals(), kept) {
		t.Error("особь с Persistent сняли с карты")
	}
}

// TestNightShiftsSpecies — ночью состав меняется: дневные виды придавлены
// весом, ночные выходят вперёд.
func TestNightShiftsSpecies(t *testing.T) {
	count := func(night bool) (day, nite int) {
		s, _ := spawner(t, "forest")
		player := engine.Vec2{X: worldSide / 2, Y: worldSide / 2}
		seen := map[*mob.Animal]bool{}
		for range 6000 {
			s.Update(player, true, night)
			for _, a := range s.Animals() {
				if seen[a] {
					continue
				}
				seen[a] = true
				switch a.Species.Activity {
				case "day":
					day++
				case "night":
					nite++
				}
			}
		}
		return day, nite
	}
	dDay, dNight := count(false)
	nDay, nNight := count(true)
	if dDay+dNight == 0 || nDay+nNight == 0 {
		t.Fatal("никто не появился")
	}
	dayShare := float64(dDay) / float64(dDay+dNight)
	nightShare := float64(nDay) / float64(nDay+nNight)
	if nightShare >= dayShare {
		t.Errorf("доля дневных видов ночью (%.2f) не меньше, чем днём (%.2f)", nightShare, dayShare)
	}
}
