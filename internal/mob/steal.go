package mob

// Кража силы: что достаётся герою с убитого врага и на сколько ударов.
//
// Сила описана у типа врага (enemies.json, поле power) — это его собственная
// атака: размах, угол, урон и слой-эффект, которым она нарисована. Украденная,
// она становится альтернативным ударом героя, поэтому здесь только числа: чем
// её рисовать и как класть в ячейку, решает игровой слой.
//
// Заряды не лежат в данных. Сила снимается с конкретной особи, а особи одного
// типа разной крепости: у старшего тира и у усиленного (★) урон выше, значит и
// украденная сила сильнее. Сильную дают подержать меньше — правило одно на все
// умения и живёт в PowerCharges.

import (
	"math"
	"math/rand/v2"

	"github.com/vladislav/game/internal/combat"
)

// Сколько ударов даёт один камень.
//
// ChargeBudget — «запас урона» камня: заряды получаются его делением на силу
// умения, поэтому слабая сила держится дольше сильной. Потолок общий на все
// умения: даже крысиный замах не должен превращаться в бесконечное оружие.
const (
	ChargeBudget = 96
	MaxCharges   = 8
	MinCharges   = 1
)

// StealShare — какая доля чужого урона достаётся герою. Сила в чужих руках
// всегда страшнее: герой не умеет ею пользоваться так, как тот, у кого её
// отняли, и бьёт остатком поверх собственного оружия.
const StealShare = 0.4

// Stolen — сила, снятая с убитого врага: чем бить, чем рисовать и сколько раз.
type Stolen struct {
	Type string // id типа врага — по нему сила восстанавливается из сохранения
	Tier string
	// Title — чья это сила («СИЛА ГОБЛИНА»): показывается в ячейке умений.
	Title   string
	Element string // стихия исходной атаки (fire, slash, magic, ...)
	Layer   string // слой-эффект в паке врага
	Tint    string // цвет следа для рубящих сил ("" — цвет говорит сам эффект)

	// Attack — удар силы уже в руках героя: геометрия чужая, урон — тоже, но
	// герою из него достаётся только StealShare (см. Rolls).
	Attack PowerAttack
	// Charges — сколько ударов осталось. Камень с нулём зарядов исчезает.
	Charges int
}

// Empty — пустая ли ячейка/камень.
func (s *Stolen) Empty() bool { return s == nil || s.Charges <= 0 }

// Rolls — что сила добавляет к удару героя: доля чужого урона в стихии этой
// силы. Диапазона нет: чужая сила бьёт ровно так, как умела, разброс приносит
// оружие в руке.
func (s *Stolen) Rolls() combat.Rolls {
	if s == nil {
		return combat.Rolls{}
	}
	d := int(math.Round(float64(s.Attack.Damage) * StealShare))
	if d < 1 {
		d = 1
	}
	r := combat.Roll{Min: d, Max: d}
	switch DamageKind(s.Element) {
	case combat.Fire:
		return combat.Rolls{Fire: r}
	case combat.Cold:
		return combat.Rolls{Cold: r}
	case combat.Lightning:
		return combat.Rolls{Lightning: r}
	}
	return combat.Rolls{Physical: r}
}

// DamageKind — каким видом урона бьёт стихия врага. Стихии мобов описывают
// картинку (огонь, дым, призрачная, клинок), а не физику, поэтому в виды урона
// они переводятся здесь, а не в данных: пока в игре есть только огонь и
// физика, всё остальное — физический удар.
func DamageKind(element string) string {
	if element == "fire" {
		return combat.Fire
	}
	return combat.Physical
}

// PowerCharges — сколько ударов даёт сила с уроном dmg. Слабую держат дольше,
// сильную — считанные разы; потолок общий (MaxCharges).
//
// Правило нарочно одно на все умения и завязано на урон, а не на тип врага:
// когда у типа появится вторая сила, сильная из них сама получит меньше
// зарядов, без отдельной строки в таблице.
func PowerCharges(dmg int) int {
	if dmg <= 0 {
		return 0
	}
	return min(max(ChargeBudget/dmg, MinCharges), MaxCharges)
}

// Steal разыгрывает кражу силы с убитого врага: nil — не выпало (или у типа
// красть нечего). Урон силы берётся не из таблицы типа, а с поправкой на саму
// особь: старший тир и усиленный бьют сильнее, а значит и сила их крепче.
func Steal(e *Enemy, rng *rand.Rand) *Stolen {
	if e == nil || e.Type == nil || e.Type.Power == nil {
		return nil
	}
	p := e.Type.Power
	r := rand.Float64()
	if rng != nil {
		r = rng.Float64()
	}
	if r >= p.StealChance {
		return nil
	}
	return stolen(e.Type, e.Tier, e.Damage)
}

// stolen собирает силу типа под крепость особи с уроном damage.
func stolen(ty *EnemyType, tr *Tier, damage int) *Stolen {
	p := ty.Power
	a := p.Attack
	// Масштаб — во сколько раз эта особь бьёт больнее самой слабой в типе.
	// Считается по урону особи, поэтому в него уже входит и тир, и усиление.
	if base := ty.BaseDamage(); base > 0 && damage > base {
		a.Damage = int(math.Round(float64(a.Damage) * float64(damage) / float64(base)))
	}
	s := &Stolen{
		Type: ty.ID, Element: p.Element, Layer: p.Layer, Tint: p.Tint,
		Attack: a, Charges: PowerCharges(a.Damage),
		Title: "СИЛА: " + ty.Title.RU,
	}
	if tr != nil {
		s.Tier = tr.ID
	}
	return s
}

// StolenDamage — урон силы, снятой с обычной (не усиленной) особи этого тира.
// Нужен бестиарию: в карточке тира и сила должна быть его, а не общая на тип.
func (t *Tier) StolenDamage() int {
	if t == nil || t.Type == nil || t.Type.Power == nil {
		return 0
	}
	return stolen(t.Type, t, t.Damage).Attack.Damage
}

// Power — сила типа typeID тира tierID, какой она достаётся с обычной особи.
// Нужна возвращению из сохранения: там известны только имена, а числа обязаны
// сойтись с теми, что были в забеге.
func (s *EnemySpawner) Power(typeID, tierID string) *Stolen {
	ty := s.cat.Types[typeID]
	if ty == nil || ty.Power == nil {
		return nil
	}
	tr := ty.Tiers[tierID]
	if tr == nil {
		return nil
	}
	return stolen(ty, tr, tr.Damage)
}

// BaseDamage — урон самого слабого тира типа (0, если тиров нет). Это точка
// отсчёта крепости: во сколько раз особь бьёт больнее него, во столько же раз
// крепче снятая с неё сила.
func (t *EnemyType) BaseDamage() int {
	base := 0
	for _, tr := range t.Tiers {
		if tr.Damage > 0 && (base == 0 || tr.Damage < base) {
			base = tr.Damage
		}
	}
	return base
}
