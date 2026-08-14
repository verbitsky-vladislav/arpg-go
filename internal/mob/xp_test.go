package mob_test

import (
	"os"
	"testing"

	"github.com/vladislav/game/internal/assets"
	"github.com/vladislav/game/internal/mob"
	"github.com/vladislav/game/internal/progress"
)

// bandSlack — во сколько раз опыт в таблице разрешено уводить от опорной полосы
// своего уровня. Отклонение бывает осмысленным (золотой слизень — находка, босс
// — событие забега), но втрое в скупую сторону или вчетверо в щедрую — это уже
// не замысел, а забытый ноль.
const bandSlack = 4.0

// TestXPBandsSane — опыт в таблицах сходится с уровнем существа.
func TestXPBandsSane(t *testing.T) {
	for _, tab := range tables {
		t.Run(tab.name, func(t *testing.T) {
			_, c := enemies(t, tab.file)
			for _, id := range c.IDs() {
				tier := c.Get(id)
				checkBand(t, id, tier.Level(), tier.XP)
			}
		})
	}
	t.Run("животные", func(t *testing.T) {
		l := assets.NewLoader(os.DirFS(assetsRoot))
		cat, err := mob.LoadSpecies(l.FS(), "mobs/animals/species.json")
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range cat.IDs() {
			s := cat.Get(id)
			checkBand(t, id, s.Level(), s.XP)
		}
	})
}

func checkBand(t *testing.T, id string, lvl int, xp mob.XPRange) {
	t.Helper()
	if xp.Min <= 0 || xp.Max < xp.Min {
		t.Errorf("%s: полоса опыта %d..%d", id, xp.Min, xp.Max)
		return
	}
	if lvl < 1 || lvl > progress.MaxLevel {
		t.Errorf("%s: уровень %d вне 1..%d", id, lvl, progress.MaxLevel)
	}
	off := progress.BandOff(lvl, int(xp.Mid()))
	if off > bandSlack || off < 1/bandSlack {
		lo, hi := progress.Band(lvl)
		t.Errorf("%s (ур %d): опыт %d..%d — ×%.1f от полосы уровня %d..%d",
			id, lvl, xp.Min, xp.Max, off, lo, hi)
	}
}

// TestXPPickInBand — выбранный опыт всегда внутри полосы и не одинаков у всех:
// иначе разброс «от и до» был бы записан впустую.
func TestXPPickInBand(t *testing.T) {
	r := mob.XPRange{Min: 16, Max: 25}
	seen := map[int]bool{}
	for i := range 200 {
		got := r.Pick(uint64(i) * 0x9E3779B97F4A7C15)
		if got < r.Min || got > r.Max {
			t.Fatalf("выбор %d вне полосы %d..%d", got, r.Min, r.Max)
		}
		seen[got] = true
	}
	if len(seen) < 5 {
		t.Errorf("на 200 затравок всего %d разных значений — разброса нет", len(seen))
	}
	// Вырожденная полоса не должна ни падать, ни выдумывать.
	if got := (mob.XPRange{Min: 7, Max: 7}).Pick(1); got != 7 {
		t.Errorf("полоса 7..7 дала %d", got)
	}
}

// TestForestCeiling — лес первая локация, и выкачаться в нём до потолка нельзя.
// Потолок задан не числом в коде, а самым сильным лесным мобом: выше его уровня
// плюс OutlevelGap опыта в лесу взять не с кого.
func TestForestCeiling(t *testing.T) {
	_, c := enemies(t, "mobs/enemies/enemies.json")
	top := 0
	for _, id := range c.TypeIDs() {
		ty := c.Types[id]
		if ty.Habitat.Biomes["forest"] == 0 {
			continue
		}
		for _, tid := range ty.TierIDs() {
			top = max(top, ty.Tiers[tid].Level())
		}
	}
	if top == 0 {
		t.Fatal("в лесу не водится ни одного врага")
	}
	cap := top + progress.OutlevelGap
	if cap >= progress.MaxLevel {
		t.Errorf("лес прокачивает до %d уровня при потолке %d — первая локация не должна", cap, progress.MaxLevel)
	}
	t.Logf("лес: сильнейший враг %d уровня, выше %d в нём не вырасти", top, cap)
}
