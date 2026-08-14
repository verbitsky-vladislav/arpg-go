package scene

// Снаряжение героя: надеть вещь из сумки, снять обратно и переключить на неё
// боевой лоадаут.
//
// Оружие тут — не число урона, а лоадаут персонажа: у каждого свой набор
// анимаций и свой удар (см. assets/character/character.json). Поэтому «надел
// клинок» означает «переключил героя на лоадаут sword», и безоружный герой,
// у которого damage=0, наконец начинает бить.

import (
	"fmt"

	"github.com/vladislav/game/internal/character"
)

// equipFromBag надевает вещь из ячейки сумки. Возвращает подпись о том, что
// вышло: экран показывает её игроку.
func (g *Game) equipFromBag(i int) string {
	s := g.bag.At(i)
	if s.Empty() {
		return ""
	}
	it, ok := g.items.Get(s.ID)
	if !ok || !it.Wearable() {
		return "ЭТО НЕ НАДЕТЬ"
	}
	prev, rest, ok := g.eq.Wear(s)
	if !ok {
		return "ЭТО НЕ НАДЕТЬ"
	}
	// Ячейка освобождается целиком, а потом в неё возвращается всё, что не
	// ушло на героя: остаток стопки и снятая вещь.
	g.bag.Take(i)
	if !rest.Empty() {
		g.bag.Put(i, rest)
	}
	if !prev.Empty() && g.bag.Add(prev.ID, prev.N) > 0 {
		// Места под снятую вещь не нашлось — надевать нельзя, иначе она
		// пропадёт. Откатываемся целиком.
		g.eq.Wear(prev)
		g.bag.Take(i)
		g.bag.Put(i, s)
		return "СУМКА ПОЛНА"
	}
	if err := g.applyLoadout(); err != nil {
		return "НЕТ АНИМАЦИЙ"
	}
	return "НАДЕТО: " + it.Name
}

// unequip снимает вещь из гнезда в сумку.
func (g *Game) unequip(slot string) string {
	worn := g.eq.At(slot)
	if worn.Empty() {
		return ""
	}
	if left := g.bag.Add(worn.ID, worn.N); left > 0 {
		return "СУМКА ПОЛНА"
	}
	g.eq.TakeOff(slot)
	if err := g.applyLoadout(); err != nil {
		return "НЕТ АНИМАЦИЙ"
	}
	return "СНЯТО: " + g.items.Name(worn.ID)
}

// applyLoadout приводит героя в соответствие с надетым оружием. Без оружия
// герой возвращается к безоружному лоадауту — тому, с которым он начинал.
func (g *Game) applyLoadout() error {
	id := g.eq.Loadout()
	if id == "" {
		id = gameLoadout
	}
	lo := g.chars.Loadout(id)
	if lo == nil {
		return fmt.Errorf("scene: нет лоадаута %q", id)
	}
	if lo == g.pl.Loadout {
		return nil
	}
	pack, err := character.LoadPack(g.l, "character", g.body, lo)
	if err != nil {
		return fmt.Errorf("scene: пак лоадаута %q: %w", id, err)
	}
	g.pl.Equip(lo, pack)
	return nil
}

// wornName — имя надетого в гнезде (пусто, если гнездо пустое).
func (g *Game) wornName(slot string) string {
	if s := g.eq.At(slot); !s.Empty() {
		return g.items.Name(s.ID)
	}
	return ""
}

// bagWearable — можно ли надеть то, что лежит в ячейке сумки.
func (g *Game) bagWearable(i int) bool {
	s := g.bag.At(i)
	if s.Empty() {
		return false
	}
	it, ok := g.items.Get(s.ID)
	return ok && it.Wearable()
}
