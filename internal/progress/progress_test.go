package progress_test

import (
	"testing"

	"github.com/vladislav/game/internal/progress"
)

// TestNeedCurve — цена уровня растёт, но всё медленнее, и первый шаг ровно тот,
// который обещан балансом: 100, потом 120.
func TestNeedCurve(t *testing.T) {
	if got := progress.Need(1); got != 100 {
		t.Errorf("первый уровень стоит %d, а не 100", got)
	}
	if got := progress.Need(2); got != 120 {
		t.Errorf("второй уровень стоит %d, а не 120 (×1.2)", got)
	}
	if got := progress.Need(progress.MaxLevel); got != 0 {
		t.Errorf("с потолка уходить некуда, а Need вернул %d", got)
	}

	prev := 0.0
	for l := 1; l < progress.MaxLevel-1; l++ {
		a, b := progress.Need(l), progress.Need(l+1)
		if b <= a {
			t.Fatalf("уровень %d дешевле предыдущего: %d → %d", l+1, a, b)
		}
		g := float64(b) / float64(a)
		if g > progress.StartRatio+1e-9 || g < progress.TailRatio-0.01 {
			t.Fatalf("уровень %d: разгон ×%.4f вне %.2f..%.2f",
				l, g, progress.TailRatio, progress.StartRatio)
		}
		// Затухание: разгон не может вырасти обратно.
		if prev > 0 && g > prev+1e-9 {
			t.Fatalf("уровень %d: разгон вырос с ×%.4f до ×%.4f", l, prev, g)
		}
		prev = g
	}
}

// TestTotalReachable — вся дорога до потолка укладывается в числа, которые игра
// вообще способна выдать. Восемь миллиардов (постоянные ×1.2) — не укладывается.
func TestTotalReachable(t *testing.T) {
	total := progress.Total(progress.MaxLevel)
	if total <= 0 || total > 10_000_000 {
		t.Errorf("дорога до сотого уровня стоит %d опыта — это не баланс", total)
	}
	if progress.Total(1) != 0 {
		t.Error("до первого уровня опыт не нужен")
	}
	if progress.Total(3) != progress.Need(1)+progress.Need(2) {
		t.Error("Total не сходится с суммой Need")
	}
}

// TestLevelUp — опыт копится, уровни берутся, очки прокачки капают.
func TestLevelUp(t *testing.T) {
	c := progress.New()
	if c.Level != 1 || c.XP != 0 || c.Points != 0 {
		t.Fatalf("новый герой не с чистого листа: %+v", c)
	}

	if got := c.Add(progress.Need(1) - 1); got != 0 || c.Level != 1 {
		t.Errorf("уровень взят раньше времени: +%d, уровень %d", got, c.Level)
	}
	if got := c.Add(1); got != 1 || c.Level != 2 || c.XP != 0 {
		t.Errorf("уровень не взят ровно на пороге: +%d, %d ур, %d опыта", got, c.Level, c.XP)
	}
	if c.Points != progress.PointsPerLevel {
		t.Errorf("за уровень дали %d очков, а не %d", c.Points, progress.PointsPerLevel)
	}

	// Разом больше одного уровня: остаток переносится, очки капают за каждый.
	c = progress.New()
	if got := c.Add(progress.Need(1) + progress.Need(2) + 5); got != 2 {
		t.Errorf("за раз взято %d уровней вместо 2", got)
	}
	if c.Level != 3 || c.XP != 5 {
		t.Errorf("после двух уровней: %d ур, %d опыта (ждали 3 и 5)", c.Level, c.XP)
	}
	if c.Points != 2*progress.PointsPerLevel {
		t.Errorf("очков %d вместо %d", c.Points, 2*progress.PointsPerLevel)
	}
}

// TestCapStops — на потолке опыт больше не копится.
func TestCapStops(t *testing.T) {
	c := progress.Character{Level: progress.MaxLevel}
	if got := c.Add(1_000_000); got != 0 {
		t.Errorf("с потолка взято ещё %d уровней", got)
	}
	if c.Level != progress.MaxLevel || c.XP != 0 {
		t.Errorf("на потолке: %d ур, %d опыта", c.Level, c.XP)
	}
	if c.Frac() != 1 {
		t.Errorf("полоса на потолке заполнена на %.2f", c.Frac())
	}
}

// TestOutlevel — переросшему добычу опыта не дают. Разница ровно в два уровня
// уже отсекает: на этом стоит потолок локации.
func TestOutlevel(t *testing.T) {
	cases := []struct {
		hero, mob, want int
	}{
		{1, 1, 50}, // ровесник
		{3, 5, 50}, // добыча сильнее — всегда полный опыт
		{5, 4, 50}, // на уровень выше — ещё платят
		{6, 4, 0},  // на два выше — уборка, а не бой
		{20, 4, 0}, //
		{4, 4, 50}, //
	}
	for _, c := range cases {
		if got := progress.Gain(c.hero, c.mob, 50); got != c.want {
			t.Errorf("герой %d, моб %d: дали %d, ждали %d", c.hero, c.mob, got, c.want)
		}
	}
	if progress.Gain(1, 1, 0) != 0 {
		t.Error("моб без опыта что-то дал")
	}
}

// TestMobLevel — уровень существа растёт от здоровья и урона, начинается с
// единицы и никогда не пробивает потолок.
func TestMobLevel(t *testing.T) {
	if l := progress.MobLevel(1, 0); l != 1 {
		t.Errorf("самая безобидная тварь — %d уровня", l)
	}
	if l := progress.MobLevel(1_000_000, 1_000_000); l != progress.MaxLevel {
		t.Errorf("сколь угодно сильная тварь пробила потолок: %d", l)
	}
	if a, b := progress.MobLevel(100, 5), progress.MobLevel(200, 5); b <= a {
		t.Errorf("вдвое живучее — тот же уровень: %d и %d", a, b)
	}
	if a, b := progress.MobLevel(100, 5), progress.MobLevel(100, 15); b <= a {
		t.Errorf("вдвое больнее — тот же уровень: %d и %d", a, b)
	}
	// Урон весит больше здоровья: очко урона дороже очка здоровья.
	if a, b := progress.MobLevel(40, 20), progress.MobLevel(60, 0); b >= a {
		t.Errorf("урон перестал весить больше здоровья: %d и %d", a, b)
	}
}

// TestBandFollowsCurve — опыт за ровесника растёт вместе с ценой уровня, и одно
// с другим сходится: уровень стоит примерно KillsMin..KillsMin+KillsSpan
// убийств. Без этого поздние уровни были бы недостижимы в принципе.
func TestBandFollowsCurve(t *testing.T) {
	for _, l := range []int{1, 2, 5, 10, 25, 50, 99} {
		lo, hi := progress.Band(l)
		if lo <= 0 || hi <= lo {
			t.Fatalf("уровень %d: полоса %d..%d", l, lo, hi)
		}
		mid := float64(lo+hi) / 2
		kills := float64(progress.Need(l)) / mid
		if kills < progress.KillsMin-0.5 || kills > progress.KillsMin+progress.KillsSpan+0.5 {
			t.Errorf("уровень %d: %.1f убийств ровесника на уровень — вне обещанного", l, kills)
		}
	}
	// Первые уровни берутся меньшим числом убийств, чем поздние.
	first := float64(progress.Need(1)) / midOf(1)
	last := float64(progress.Need(90)) / midOf(90)
	if first >= last {
		t.Errorf("ранние уровни качаются не быстрее: %.1f против %.1f убийств", first, last)
	}
}

func midOf(l int) float64 {
	lo, hi := progress.Band(l)
	return float64(lo+hi) / 2
}
