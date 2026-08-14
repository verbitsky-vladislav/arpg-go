package mob

// Возвращение населения из сохранения.
//
// Мир персонажа сохраняется целиком (см. docs/save.md), а значит и те, кто по
// нему ходил: волк с отгрызенным боком должен встретить вернувшегося игрока
// там же и таким же. Спавнеры умеют заселять карту сами, но заселяют они её
// случайно — здесь, наоборот, известно всё: вид, место, этаж и остаток
// здоровья.
//
// Возвращение проходит мимо всех правил заселения: ни зон обитания, ни бюджета
// опасности, ни лимитов. Эти правила решают, кому появиться на пустой карте, а
// тут никто не появляется — тут возвращают тех, кто уже был. Учёт (счётчики по
// видам, занятая опасность) при этом ведётся честно, иначе спавнер посчитает
// карту пустой и досыплет сверху.

import (
	"github.com/vladislav/game/internal/engine"
)

// RestoreAnimal возвращает на карту зверя вида species. hp <= 0 — здоровье вида
// целиком. false — вид или его пак пропали из данных: такого зверя не вернуть.
func (s *Spawner) RestoreAnimal(species string, pos engine.Vec2, floor uint8, hp int) bool {
	sp := s.cat.Get(species)
	if sp == nil {
		return false
	}
	pack, err := s.packs(sp.Art)
	if err != nil {
		s.noteErr(sp.ID, err)
		return false
	}
	a := NewAnimal(sp, pack, pos, s.rng)
	a.floor = floor
	if hp > 0 {
		a.HP = min(hp, sp.Stats.HP)
	}
	s.animals = append(s.animals, a)
	s.counts[sp.ID]++
	return true
}

// RestoreEnemy возвращает на карту врага типа typeID тира tier.
//
// Усиление применяется до здоровья: MakeElite домножает и предел, и текущее
// значение, и сохранённый остаток иначе оказался бы умножен второй раз.
func (s *EnemySpawner) RestoreEnemy(typeID, tier string, pos engine.Vec2, floor uint8, hp int, elite bool) bool {
	ty := s.cat.Types[typeID]
	if ty == nil {
		return false
	}
	tr := ty.Tiers[tier]
	if tr == nil {
		return false
	}
	pack, err := s.packs(ty.Art + "/" + tr.ID)
	if err != nil {
		s.noteErr(ty.ID, err)
		return false
	}
	e := NewEnemy(tr, pack, s.bhv.For(ty.Temper, ty.Family), pos, s.rng)
	e.floor = floor
	if elite {
		e.MakeElite(s.cfg.Elite.HPScale, s.cfg.Elite.DamageScale, s.cfg.Elite.XPScale)
	}
	if hp > 0 {
		e.HP = min(hp, e.MaxHP)
	}
	s.squad.Add(e)
	s.counts[ty.ID]++
	s.danger += e.Danger()
	return true
}
