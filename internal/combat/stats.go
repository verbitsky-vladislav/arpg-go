package combat

// Stats — характеристики любого бойца (игрока или моба).
// Защита моделируется двумя механиками (задел под PoE-подобную систему):
//   - Armour снижает ФИЗИЧЕСКИЙ урон (тем сильнее, чем слабее удар).
//   - Resist[t] — доля снижения урона стихии t (0..0.9).
type Stats struct {
	MaxLife, Life float64
	MaxMana, Mana float64
	Armour        float64
	Resist        [NumDamageTypes]float64
	MoveSpeed     float64

	LifeRegen float64 // в секунду
	ManaRegen float64 // в секунду
}

func (s *Stats) Alive() bool { return s.Life > 0 }

// TakeDamage применяет снижение и вычитает урон из жизни. Возвращает,
// сколько урона реально прошло (пригодится для чисел урона на экране).
func (s *Stats) TakeDamage(d Damage) float64 {
	amt := d.Amount

	// Броня против физики: reduction = armour / (armour + 10*urgon).
	// Слабые удары режутся сильно, тяжёлые — почти проходят.
	if d.Type == Physical && s.Armour > 0 && amt > 0 {
		reduction := s.Armour / (s.Armour + 10*amt)
		amt *= 1 - reduction
	}

	// Сопротивление стихии.
	res := s.Resist[d.Type]
	if res > 0 {
		amt *= 1 - res
	}

	if amt < 0 {
		amt = 0
	}
	s.Life -= amt
	if s.Life < 0 {
		s.Life = 0
	}
	return amt
}

// RegenTick восстанавливает жизнь и ману за один тик (60 тиков = 1 сек).
func (s *Stats) RegenTick() {
	s.Life = min(s.MaxLife, s.Life+s.LifeRegen/60)
	s.Mana = min(s.MaxMana, s.Mana+s.ManaRegen/60)
}

// SpendMana пытается списать ману. false — если не хватает.
func (s *Stats) SpendMana(cost float64) bool {
	if s.Mana < cost {
		return false
	}
	s.Mana -= cost
	return true
}
