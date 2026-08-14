package mob_test

import (
	"math/rand/v2"
	"testing"

	"github.com/vladislav/game/internal/mob"
)

func rng() *rand.Rand { return rand.New(rand.NewPCG(1, 0x9E3779B97F4A7C15)) }

// total — сколько всего штук в добыче.
func total(loot []mob.Loot) int {
	n := 0
	for _, l := range loot {
		n += l.N
	}
	return n
}

// count — сколько штук предмета id в добыче.
func count(loot []mob.Loot, id string) int {
	for _, l := range loot {
		if l.ID == id {
			return l.N
		}
	}
	return 0
}

// TestRollDropsSureOnce — обязательная строка выпадает всегда и ровно один раз,
// сколько бы ни было бросков. Иначе тир раздувал бы стопку мяса вместо того,
// чтобы давать шанс на редкость.
func TestRollDropsSureOnce(t *testing.T) {
	table := []mob.Drop{{ID: "meat", Min: 1, Max: 1, Chance: 1}}
	r := rng()
	for _, rolls := range []int{1, 3, 10} {
		for range 200 {
			if got := count(mob.RollDrops(table, rolls, 0, r), "meat"); got != 1 {
				t.Fatalf("бросков %d: выпало %d мяса, ждали ровно 1", rolls, got)
			}
		}
	}
}

// TestRollDropsRareGrowsWithRolls — редкое зависит от числа бросков и от
// бонуса: в этом и состоит разница между тиром и тиром, между рядовым и элитой.
func TestRollDropsRareGrowsWithRolls(t *testing.T) {
	// Обязательная строка в таблице обязана быть: без неё сработала бы страховка
	// «пусто не бывает» и редкость выпадала бы всегда — мерить было бы нечего.
	table := []mob.Drop{
		{ID: "meat", Min: 1, Max: 1, Chance: 1},
		{ID: "horn", Min: 1, Max: 1, Chance: 0.2},
	}
	rate := func(rolls int, bonus float64) float64 {
		r, hits := rng(), 0
		const tries = 3000
		for range tries {
			if count(mob.RollDrops(table, rolls, bonus, r), "horn") > 0 {
				hits++
			}
		}
		return float64(hits) / tries
	}
	one, three, elite := rate(1, 0), rate(3, 0), rate(1, 0.2)
	if !(one < three) {
		t.Errorf("три броска дают %.2f, один — %.2f: тир ничего не меняет", three, one)
	}
	if !(one < elite) {
		t.Errorf("элита даёт %.2f, рядовой — %.2f: бонус ничего не меняет", elite, one)
	}
	if one < 0.15 || one > 0.25 {
		t.Errorf("шанс 0.2 выпадает с частотой %.2f", one)
	}
}

// TestRollDropsNeverEmpty — с трупа не уходят с пустыми руками, даже если в
// таблице одно редкое и оно не выпало. Гарантия держится кодом: строку с
// chance 1.0 в данных легко забыть, а моб без добычи выглядит как поломка.
func TestRollDropsNeverEmpty(t *testing.T) {
	table := []mob.Drop{{ID: "gem", Min: 1, Max: 3, Chance: 0.01}}
	r := rng()
	for range 500 {
		if loot := mob.RollDrops(table, 1, 0, r); total(loot) < 1 {
			t.Fatal("таблица разыгралась в пустоту")
		}
	}
	if loot := mob.RollDrops(nil, 1, 0, r); loot != nil {
		t.Errorf("пустая таблица родила добычу: %v", loot)
	}
}

// TestGameLootIsSmallAndSure — главное про баланс, проверенное на настоящих
// таблицах: с каждого моба игры что-то падает всегда, но падает мало. Числа
// проверяются вместе, потому что порознь каждое легко удовлетворить — щедрой
// таблицей или пустой.
func TestGameLootIsSmallAndSure(t *testing.T) {
	const (
		tries      = 300
		maxStacks  = 2.5 // разных вещей за убийство в среднем
		maxUnits   = 8.0 // штук за убийство в среднем
		monsterMin = 1   // без добычи не уходят никогда
	)
	check := func(t *testing.T, who string, drops []mob.Drop, rolls int) {
		t.Helper()
		r := rng()
		stacks, units := 0, 0
		for range tries {
			loot := mob.RollDrops(drops, rolls, 0, r)
			if total(loot) < monsterMin {
				t.Fatalf("%s: убит и ничего не отдал", who)
			}
			stacks += len(loot)
			units += total(loot)
		}
		if s := float64(stacks) / tries; s > maxStacks {
			t.Errorf("%s: %.2f вещи за убийство — добычи слишком много", who, s)
		}
		if u := float64(units) / tries; u > maxUnits {
			t.Errorf("%s: %.2f штук за убийство — добычи слишком много", who, u)
		}
	}

	_, animals := catalog(t)
	for _, id := range animals.IDs() {
		check(t, id, animals.Get(id).Drops, 1)
	}
	for _, tab := range tables {
		_, c := enemies(t, tab.file)
		for _, tid := range c.TypeIDs() {
			ty := c.Types[tid]
			for _, tier := range ty.TierIDs() {
				check(t, tid+"_"+tier, ty.Drops, ty.Tiers[tier].DropRolls)
			}
		}
	}
}
