package mob_test

import (
	"math/rand/v2"
	"testing"

	"github.com/vladislav/game/internal/combat"
	"github.com/vladislav/game/internal/mob"
)

// foes — таблица врагов (у боссов силы пока нет, красть там нечего).
func foes(t *testing.T) *mob.EnemyCatalog {
	t.Helper()
	_, c := enemies(t, "mobs/enemies/enemies.json")
	return c
}

// weakestTier — тир с наименьшим уроном: с него сила снимается такой, как
// записана в таблице.
func weakestTier(ty *mob.EnemyType) *mob.Tier {
	var out *mob.Tier
	for _, tr := range ty.Tiers {
		if out == nil || tr.Damage < out.Damage {
			out = tr
		}
	}
	return out
}

// TestPowerChargesFallWithStrength — слабую силу дают подержать дольше сильной,
// и ни одна не переваливает за общий потолок.
func TestPowerChargesFallWithStrength(t *testing.T) {
	weak, mid, strong := mob.PowerCharges(9), mob.PowerCharges(16), mob.PowerCharges(40)
	if !(weak >= mid && mid > strong) {
		t.Errorf("заряды не падают с силой: 9→%d, 16→%d, 40→%d", weak, mid, strong)
	}
	for dmg := 1; dmg < 200; dmg++ {
		n := mob.PowerCharges(dmg)
		if n < mob.MinCharges || n > mob.MaxCharges {
			t.Fatalf("урон %d дал %d зарядов — вне %d..%d",
				dmg, n, mob.MinCharges, mob.MaxCharges)
		}
	}
	if n := mob.PowerCharges(0); n != 0 {
		t.Errorf("сила без урона дала %d зарядов", n)
	}
}

// TestStolenGrowsWithTier — сила, снятая со старшего тира, крепче, а зарядов у
// неё не больше: редкость моба и есть мера силы умения.
func TestStolenGrowsWithTier(t *testing.T) {
	for id, ty := range foes(t).Types {
		if ty.Power == nil || len(ty.Tiers) < 2 {
			continue
		}
		var weakest, strongest *mob.Tier
		for _, tr := range ty.Tiers {
			if weakest == nil || tr.Damage < weakest.Damage {
				weakest = tr
			}
			if strongest == nil || tr.Damage > strongest.Damage {
				strongest = tr
			}
		}
		lo, hi := weakest.StolenDamage(), strongest.StolenDamage()
		if hi <= lo {
			t.Errorf("%s: сила старшего тира %d не крепче младшего %d", id, hi, lo)
		}
		if mob.PowerCharges(hi) > mob.PowerCharges(lo) {
			t.Errorf("%s: у сильной версии зарядов больше (%d против %d)",
				id, mob.PowerCharges(hi), mob.PowerCharges(lo))
		}
	}
}

// TestStolenRollsShare — герою достаётся доля чужого урона, и она приходит
// стихией исходной силы: огненная сила бьёт огнём, рубящая — физически.
func TestStolenRollsShare(t *testing.T) {
	for id, ty := range foes(t).Types {
		if ty.Power == nil {
			continue
		}
		tr := weakestTier(ty)
		s := &mob.Stolen{Element: ty.Power.Element, Attack: ty.Power.Attack,
			Charges: mob.PowerCharges(ty.Power.Attack.Damage)}

		got := s.Rolls().Value(nil).Total()
		if got <= 0 || got >= s.Attack.Damage {
			t.Errorf("%s: украденный урон %d не является долей чужого %d",
				id, got, s.Attack.Damage)
		}
		kind := mob.DamageKind(ty.Power.Element)
		if s.Rolls().Of(kind).Empty() {
			t.Errorf("%s: сила стихии %q бьёт не своим видом урона (%s)",
				id, ty.Power.Element, kind)
		}
		if kind != combat.Fire && !s.Rolls().Fire.Empty() {
			t.Errorf("%s: нестихийная сила бьёт огнём", id)
		}
		if tr.StolenDamage() <= 0 {
			t.Errorf("%s: у тира %s нет украденного урона", id, tr.ID)
		}
	}
}

// TestStealRollsChance — кража разыгрывается шансом типа, а не выпадает всегда.
func TestStealRollsChance(t *testing.T) {
	cat := foes(t)
	var ty *mob.EnemyType
	for _, x := range cat.Types {
		if x.Power != nil && (ty == nil || x.ID < ty.ID) {
			ty = x
		}
	}
	if ty == nil {
		t.Skip("в таблице нет ни одной силы")
	}
	tr := weakestTier(ty)
	rng := rand.New(rand.NewPCG(7, 11))
	// Особь собирается полем, а не NewEnemy: краже нужны только тип, тир и урон,
	// а конструктор потребовал бы ещё пак со спрайтами.
	e := &mob.Enemy{Type: ty, Tier: tr, Damage: tr.Damage}

	got := 0
	const tries = 600
	for range tries {
		s := mob.Steal(e, rng)
		if s == nil {
			continue
		}
		got++
		if s.Charges < mob.MinCharges || s.Charges > mob.MaxCharges {
			t.Fatalf("%s: у снятой силы %d зарядов", ty.ID, s.Charges)
		}
		if s.Type != ty.ID || s.Tier != tr.ID {
			t.Fatalf("сила снята не с того: %s/%s вместо %s/%s", s.Type, s.Tier, ty.ID, tr.ID)
		}
	}
	rate, chance := float64(got)/tries, ty.Power.StealChance
	if rate < chance/2 || rate > chance*2 {
		t.Errorf("%s: сила выпала в %.0f%% случаев при шансе %.0f%%",
			ty.ID, rate*100, chance*100)
	}
}
