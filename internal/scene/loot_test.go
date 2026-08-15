package scene

import (
	"math/rand/v2"
	"os"
	"testing"

	"github.com/vladislav/game/internal/assets"
	"github.com/vladislav/game/internal/engine"
	"github.com/vladislav/game/internal/item"
	"github.com/vladislav/game/internal/mob"
	"github.com/vladislav/game/internal/physics"
	"github.com/vladislav/game/internal/world"
)

// TestMobLootIsReal — таблицы мобов раздают добычу идентификаторами, и каждый
// обязан быть в каталоге предметов. Опечатка в species.json иначе превращается
// в вещь без имени и без картинки, а заметить это можно только по земле.
//
// Проверка живёт в сцене, а не в mob: только здесь встречаются обе стороны —
// таблицы мобов и каталог предметов.
func TestMobLootIsReal(t *testing.T) {
	l := assets.NewLoader(os.DirFS(testAssets))
	cat, err := item.Load(l.FS(), itemsFile)
	if err != nil {
		t.Fatalf("каталог предметов: %v", err)
	}
	seen := func(who, id string) {
		if _, ok := cat.Get(id); !ok {
			t.Errorf("%s: добыча %q не описана в %s", who, id, itemsFile)
		}
	}

	sp, err := mob.LoadSpecies(l.FS(), animalsRoot+"/species.json")
	if err != nil {
		t.Fatalf("таблица видов: %v", err)
	}
	for _, id := range sp.IDs() {
		for _, d := range sp.Get(id).Drops {
			seen(id, d.ID)
		}
	}
	for _, f := range []string{enemiesRoot + "/enemies.json", bossesRoot + "/bosses.json"} {
		c, err := mob.LoadEnemies(l.FS(), f)
		if err != nil {
			t.Fatalf("таблица %s: %v", f, err)
		}
		for _, id := range c.TypeIDs() {
			for _, d := range c.Types[id].Drops {
				seen(id, d.ID)
			}
		}
	}
}

// TestLootRulesLoad — правила добычи читаются и осмысленны. Числа проверяет сам
// загрузчик; тест сторожит то, что забег без них не собрать.
func TestLootRulesLoad(t *testing.T) {
	l := assets.NewLoader(os.DirFS(testAssets))
	r, err := loadLoot(l.FS(), lootFile)
	if err != nil {
		t.Fatalf("правила добычи: %v", err)
	}
	// Подобрать вещь нужно успевать на ходу: за время, пока герой пересекает
	// зону притяжения, притяжение обязано его догнать.
	if r.Pickup.Speed < 100 {
		t.Errorf("притяжение %0.f px/с — вещь не догонит идущего героя", r.Pickup.Speed)
	}
	if ticks(r.Pickup.DelayMS) < ticks(r.Ground.HopMS) {
		t.Errorf("подбор разрешён через %d тиков, а прыжок длится %d — вещь всосётся в полёте",
			ticks(r.Pickup.DelayMS), ticks(r.Ground.HopMS))
	}
}

// TestLootSpotOnGround — вещь ложится туда, куда за ней можно прийти: на
// проходимую клетку своего этажа. Без этого добыча с летуна над водой была бы
// видна и недостижима.
func TestLootSpotOnGround(t *testing.T) {
	l := assets.NewLoader(os.DirFS(testAssets))
	m, err := world.Generate(l, GameBiome, 11, 96)
	if err != nil {
		t.Fatalf("карта: %v", err)
	}
	f, from := m.Field(), m.Spawn()
	body := physics.Body{Radius: 3, Floor: physics.FloorLow}
	r := rand.New(rand.NewPCG(3, 0x5DEECE66D))
	bad := 0
	for range 300 {
		p := lootSpot(m, from, physics.FloorLow, 12, r)
		if !f.Fits(p, body) {
			bad++
		}
		if engine.Dist(p, from) > 60 {
			t.Fatalf("вещь улетела на %.0f px от трупа", engine.Dist(p, from))
		}
	}
	// Ноль допусков: точка старта стоит на открытой земле, отступать в сторону
	// там всегда есть куда.
	if bad > 0 {
		t.Errorf("%d из 300 мест непроходимы", bad)
	}
}

// TestKillDropsAndPickup — сквозная петля, ради которой всё и делалось: моб
// убит, добыча лежит на земле, герой подошёл — она в сумке. Ломается это
// по-разному (не выпало, легло не туда, не подобралось), а выглядит одинаково:
// с мобов ничего не падает.
func TestKillDropsAndPickup(t *testing.T) {
	l := assets.NewLoader(os.DirFS(testAssets))
	g, err := NewGame(l, nil, "male")
	if err != nil {
		t.Fatalf("забег не собрался: %v", err)
	}
	drops := []mob.Drop{{ID: "raw_meat", Min: 1, Max: 2, Chance: 1}}
	g.dropLoot(g.pl.Pos, g.pl.Floor(), drops, 1, false)
	if len(g.drops) == 0 {
		t.Fatal("с убитого ничего не выпало")
	}
	want := g.drops[0].n
	if !g.bag.Empty() {
		t.Fatal("сумка не пуста в начале забега")
	}

	// Герой стоит на месте: вещь обязана дойти до него сама.
	for range 5 * 60 {
		g.updateLoot()
		if len(g.drops) == 0 {
			break
		}
	}
	if len(g.drops) != 0 {
		t.Errorf("вещь так и осталась лежать в %.0f px от героя",
			engine.Dist(g.drops[0].pos, g.pl.Pos))
	}
	if got := g.bag.Count("raw_meat"); got != want {
		t.Errorf("в сумке %d мяса, выпало %d", got, want)
	}
}

// TestFullBagKeepsLoot — сумка полна: добыча остаётся лежать, а не пропадает.
// Молча съесть выпавшее — худшее, что может сделать подбор.
func TestFullBagKeepsLoot(t *testing.T) {
	l := assets.NewLoader(os.DirFS(testAssets))
	g, err := NewGame(l, nil, "male")
	if err != nil {
		t.Fatalf("забег не собрался: %v", err)
	}
	// Забиваем сумку разными вещами: одинаковые сложились бы в стопки и место
	// осталось бы.
	ids := g.items.IDs()
	for i := range g.bag.Size() {
		g.bag.Add(ids[i%len(ids)], 1)
	}
	g.dropLoot(g.pl.Pos, g.pl.Floor(), []mob.Drop{{ID: "slime_crown", Min: 1, Max: 1, Chance: 1}}, 1, false)
	if len(g.drops) == 0 {
		t.Fatal("с убитого ничего не выпало")
	}
	for range 5 * 60 {
		g.updateLoot()
	}
	if len(g.drops) == 0 {
		t.Fatal("вещь исчезла, хотя класть её было некуда")
	}
	if g.bag.Count("slime_crown") != 0 {
		t.Error("вещь попала в полную сумку")
	}
}
