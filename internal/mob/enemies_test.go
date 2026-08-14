package mob_test

import (
	"io/fs"
	"maps"
	"os"
	"slices"
	"testing"

	"github.com/vladislav/game/internal/assets"
	"github.com/vladislav/game/internal/mob"
	"github.com/vladislav/game/internal/sprite"
)

// Оба файла одного формата, поэтому и проверяются одним набором тестов.
var tables = []struct{ name, file, root string }{
	{"враги", "mobs/enemies/enemies.json", "mobs/enemies"},
	{"боссы", "mobs/bosses/bosses.json", "mobs/bosses"},
}

func enemies(t *testing.T, file string) (*assets.Loader, *mob.EnemyCatalog) {
	t.Helper()
	l := assets.NewLoader(os.DirFS(assetsRoot))
	c, err := mob.LoadEnemies(l.FS(), file)
	if err != nil {
		t.Fatal(err)
	}
	return l, c
}

// TestEnemiesValid — таблицы внутренне связны.
func TestEnemiesValid(t *testing.T) {
	for _, tab := range tables {
		t.Run(tab.name, func(t *testing.T) {
			_, c := enemies(t, tab.file)
			for _, p := range c.Validate() {
				t.Error(p)
			}
		})
	}
}

// TestEnemyPacksLoad — у каждого варианта есть спрайт-пак, он режется без
// ошибок, и его id в манифесте совпадает со сквозным id таблицы. Совпадение
// важно: по нему связываются данные и графика, и разойтись они могут молча.
func TestEnemyPacksLoad(t *testing.T) {
	for _, tab := range tables {
		t.Run(tab.name, func(t *testing.T) {
			l, c := enemies(t, tab.file)
			for _, id := range c.IDs() {
				tier := c.Get(id)
				if tier == nil {
					t.Fatalf("%s: Get не нашёл собственный id", id)
				}
				p, err := sprite.Load(l, tier.PackDir(tab.root))
				if err != nil {
					t.Errorf("%s: %v", id, err)
					continue
				}
				if p.ID != id {
					t.Errorf("%s: манифест пака зовётся %q", id, p.ID)
				}
				for _, n := range []string{"idle", "walk", "attack", "hurt", "death"} {
					if !p.Has(n) {
						t.Errorf("%s: нет обязательного клипа %q", id, n)
					}
				}
				for d := range sprite.Dir(sprite.DirCount) {
					if p.Clip("idle", d) == nil {
						t.Errorf("%s: нет idle для направления %v", id, d)
					}
				}
			}
		})
	}
}

// TestPowerLayers — сила, объявленная в таблице, существует в графике: слой
// лежит в parts у каждого тира, а кадр попадания есть в клипе атаки. Это
// единственная сверка данных с картинками — занизить hit_at безопасно, а
// завысить значит бить кадром, которого в клипе нет.
func TestPowerLayers(t *testing.T) {
	for _, tab := range tables {
		t.Run(tab.name, func(t *testing.T) {
			l, c := enemies(t, tab.file)
			powers := 0
			for _, tid := range c.TypeIDs() {
				ty := c.Types[tid]
				if ty.Power == nil {
					continue
				}
				powers++
				for _, tierID := range ty.TierIDs() {
					tier := ty.Tiers[tierID]
					dir := tier.PackDir(tab.root)
					if _, err := mob.LayerFile(l.FS(), dir, ty.Power.Layer); err != nil {
						t.Errorf("%s_%s: %v", tid, tierID, err)
						continue
					}
					p, err := sprite.Load(l, dir)
					if err != nil {
						t.Errorf("%s_%s: %v", tid, tierID, err)
						continue
					}
					atk, ok := p.Animations["attack"]
					if !ok {
						t.Errorf("%s_%s: сила есть, а клипа attack нет", tid, tierID)
						continue
					}
					if ty.Power.Attack.HitAt >= atk.Frames {
						t.Errorf("%s_%s: hit_at=%d, а кадров в attack %d",
							tid, tierID, ty.Power.Attack.HitAt, atk.Frames)
					}
				}
			}
			if tab.root == "mobs/enemies" && powers == 0 {
				t.Error("ни у одного врага нет силы — таблица потеряла блоки power")
			}
		})
	}
}

// TestBossPhases — фазы босса ссылаются на клипы, которые в паке есть.
func TestBossPhases(t *testing.T) {
	l, c := enemies(t, "mobs/bosses/bosses.json")
	for _, tid := range c.TypeIDs() {
		ty := c.Types[tid]
		for _, tierID := range ty.TierIDs() {
			p, err := sprite.Load(l, ty.Tiers[tierID].PackDir("mobs/bosses"))
			if err != nil {
				t.Fatalf("%s_%s: %v", tid, tierID, err)
			}
			for i, ph := range ty.Phases {
				if !p.Has(ph.Attack) {
					t.Errorf("%s_%s: фаза %d бьёт клипом %q, которого в паке нет",
						tid, tierID, i, ph.Attack)
				}
			}
		}
	}
}

// TestBiomeCoverage — в каждом биоме водится хотя бы три типа врагов, и все
// упомянутые биомы существуют.
//
// Три — это минимум, ниже которого биом перестаёт читаться как место: игрок
// встречает там одно и то же и решает, что мир пустой. Проверка идёт по
// каталогу assets/biomes, а не по списку в коде, поэтому новый биом обязан
// сразу обзавестись населением — иначе тест упадёт.
//
// Боссы не в счёт: их ставят под арену, а не в общую популяцию.
func TestBiomeCoverage(t *testing.T) {
	const minTypes = 3
	l, c := enemies(t, "mobs/enemies/enemies.json")

	entries, err := fs.ReadDir(l.FS(), "biomes")
	if err != nil {
		t.Fatal(err)
	}
	live := map[string]int{}
	for _, e := range entries {
		if e.IsDir() {
			live[e.Name()] = 0
		}
	}

	for _, tid := range c.TypeIDs() {
		for b, w := range c.Types[tid].Habitat.Biomes {
			if _, ok := live[b]; !ok {
				t.Errorf("%s: биом %q не существует в assets/biomes", tid, b)
				continue
			}
			if w <= 0 {
				t.Errorf("%s: вес биома %q равен %d", tid, b, w)
			}
			live[b]++
		}
	}

	for _, b := range slices.Sorted(maps.Keys(live)) {
		if live[b] < minTypes {
			t.Errorf("биом %s: типов врагов %d, нужно хотя бы %d", b, live[b], minTypes)
		}
	}
}

// TestElementFallback — у слизня стихия своя (цвет), у остальных берётся из
// силы типа. Без этого лавовый слизень был бы просто «рубящим».
func TestElementFallback(t *testing.T) {
	_, c := enemies(t, "mobs/enemies/enemies.json")
	if got := c.Get("slime_lava").Elem(); got != "fire" {
		t.Errorf("lava slime: стихия %q вместо fire", got)
	}
	if got := c.Get("demon_t2").Elem(); got != "fire" {
		t.Errorf("demon t2: стихия %q вместо fire (из power типа)", got)
	}
	if got := c.Get("skeleton_t1").Elem(); got != "slash" {
		t.Errorf("skeleton t1: стихия %q вместо slash", got)
	}
}
