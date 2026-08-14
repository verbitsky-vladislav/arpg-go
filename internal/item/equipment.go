package item

// Надетое: что герой носит в каждом гнезде.
//
// Гнёзда не перечислены списком: гнездо — это то, что написано у вещи в поле
// slot, а какие из них показывать, решает разметка окна (panels.json). Так
// новое гнездо появляется правкой данных, а не кода.

import "github.com/vladislav/game/internal/combat"

// Equipment — надетые вещи по гнёздам.
type Equipment struct {
	cat  *Catalog
	worn map[string]Slot
}

// NewEquipment создаёт пустое снаряжение.
func NewEquipment(cat *Catalog) *Equipment {
	return &Equipment{cat: cat, worn: map[string]Slot{}}
}

// At — что надето в гнезде.
func (e *Equipment) At(slot string) Slot {
	if e == nil {
		return Slot{}
	}
	return e.worn[slot]
}

// Wear надевает вещь и возвращает то, что было в гнезде (пустая ячейка, если
// гнездо пустовало). ok == false — вещь не носится вовсе.
//
// Надевается ровно одна штука: стопка из двух клинков (её не бывает, но данные
// могут соврать) не должна исчезать целиком — остаток вернётся вызывающему.
func (e *Equipment) Wear(s Slot) (prev Slot, rest Slot, ok bool) {
	if e == nil || s.Empty() {
		return Slot{}, s, false
	}
	it, found := e.cat.Get(s.ID)
	if !found || !it.Wearable() {
		return Slot{}, s, false
	}
	prev = e.worn[it.Slot]
	e.worn[it.Slot] = Slot{ID: s.ID, N: 1}
	if s.N > 1 {
		rest = Slot{ID: s.ID, N: s.N - 1}
	}
	return prev, rest, true
}

// TakeOff снимает вещь из гнезда.
func (e *Equipment) TakeOff(slot string) Slot {
	if e == nil {
		return Slot{}
	}
	s := e.worn[slot]
	delete(e.worn, slot)
	return s
}

// Worn — всё надетое: гнездо → вещь. Копия, а не сама карта: снаряжение
// меняется только через Wear/TakeOff, и правка со стороны его бы рассинхронила
// с лоадаутом персонажа. Нужен тем, кто обходит надетое целиком, — сохранению.
func (e *Equipment) Worn() map[string]Slot {
	if e == nil {
		return nil
	}
	out := make(map[string]Slot, len(e.worn))
	for slot, s := range e.worn {
		if !s.Empty() {
			out[slot] = s
		}
	}
	return out
}

// Loadout — каким оружием герой сейчас машет ("" — ничем). Смотрит только в
// гнездо weapon: остальное снаряжение анимации не меняет.
func (e *Equipment) Loadout() string {
	if e == nil {
		return ""
	}
	s := e.worn[SlotWeapon]
	if s.Empty() {
		return ""
	}
	if it, ok := e.cat.Get(s.ID); ok {
		return it.Loadout
	}
	return ""
}

// Weapon — боевые свойства того, что в гнезде оружия (nil — гнездо пусто или
// вещь в нём боевых свойств не несёт). Ими герой и бьёт: урон, скорость, форма
// удара и состояния приходят от вещи, а не от лоадаута.
func (e *Equipment) Weapon() *combat.Weapon {
	if e == nil {
		return nil
	}
	s := e.worn[SlotWeapon]
	if s.Empty() {
		return nil
	}
	return e.cat.Weapon(s.ID)
}

// SlotWeapon — гнездо оружия. Оно единственное, о котором знает код: от него
// зависит, чем герой бьёт.
const SlotWeapon = "weapon"
