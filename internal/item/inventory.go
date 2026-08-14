package item

// Сумка: ячейки с предметами. Одна и та же штука служит и сумкой героя, и
// содержимым сундука — разница только в размере и в том, кто её показывает.

// Slot — ячейка: что лежит и сколько. Пустая ячейка — та, у которой N == 0.
type Slot struct {
	ID string
	N  int
}

// Empty — пуста ли ячейка.
func (s Slot) Empty() bool { return s.N <= 0 || s.ID == "" }

// Inventory — набор ячеек фиксированного размера.
//
// Размер стопки берётся из каталога, поэтому монеты ложатся по 999 в ячейку, а
// оружие по одному. Незнакомый предмет (его нет в каталоге) не теряется: он
// кладётся стопкой по одному — лучше показать игроку непонятную вещь, чем
// молча съесть добычу.
type Inventory struct {
	cat   *Catalog
	slots []Slot
}

// NewInventory создаёт сумку на size ячеек.
func NewInventory(cat *Catalog, size int) *Inventory {
	if size < 0 {
		size = 0
	}
	return &Inventory{cat: cat, slots: make([]Slot, size)}
}

// Size — сколько всего ячеек.
func (v *Inventory) Size() int { return len(v.slots) }

// At — содержимое ячейки i (пустая, если индекс за границами).
func (v *Inventory) At(i int) Slot {
	if i < 0 || i >= len(v.slots) {
		return Slot{}
	}
	return v.slots[i]
}

// Slots — все ячейки по порядку.
func (v *Inventory) Slots() []Slot { return v.slots }

// Empty — нет ли в сумке вообще ничего.
func (v *Inventory) Empty() bool {
	for _, s := range v.slots {
		if !s.Empty() {
			return false
		}
	}
	return true
}

// stackOf — предельный размер стопки предмета.
func (v *Inventory) stackOf(id string) int {
	if v.cat != nil {
		if it, ok := v.cat.Get(id); ok && it.Stack > 0 {
			return it.Stack
		}
	}
	return 1
}

// Count — сколько всего предметов id лежит в сумке.
func (v *Inventory) Count(id string) int {
	n := 0
	for _, s := range v.slots {
		if s.ID == id {
			n += s.N
		}
	}
	return n
}

// Add кладёт n штук предмета id и возвращает остаток, который НЕ поместился.
// Сначала добиваются начатые стопки, потом занимаются пустые ячейки: иначе
// сумка забивается огрызками по одной монете.
func (v *Inventory) Add(id string, n int) int {
	if id == "" || n <= 0 {
		return 0
	}
	stack := v.stackOf(id)
	for i := range v.slots {
		if n == 0 {
			break
		}
		s := &v.slots[i]
		if s.ID != id || s.N <= 0 || s.N >= stack {
			continue
		}
		take := min(stack-s.N, n)
		s.N += take
		n -= take
	}
	for i := range v.slots {
		if n == 0 {
			break
		}
		s := &v.slots[i]
		if !s.Empty() {
			continue
		}
		take := min(stack, n)
		*s = Slot{ID: id, N: take}
		n -= take
	}
	return n
}

// Take забирает всё содержимое ячейки i.
func (v *Inventory) Take(i int) Slot {
	if i < 0 || i >= len(v.slots) {
		return Slot{}
	}
	s := v.slots[i]
	v.slots[i] = Slot{}
	return s
}

// Put кладёт ячейку обратно (например, когда перенос не удался целиком).
func (v *Inventory) Put(i int, s Slot) {
	if i < 0 || i >= len(v.slots) || s.Empty() {
		return
	}
	v.slots[i] = s
}

// MoveTo переносит содержимое ячейки i в другую сумку. Возвращает, сколько
// штук переехало: если у приёмника не хватило места, остаток остаётся на
// месте, а не пропадает.
func (v *Inventory) MoveTo(dst *Inventory, i int) int {
	s := v.At(i)
	if s.Empty() || dst == nil {
		return 0
	}
	left := dst.Add(s.ID, s.N)
	moved := s.N - left
	if left > 0 {
		v.Put(i, Slot{ID: s.ID, N: left})
	} else {
		v.Take(i)
	}
	return moved
}

// MoveAllTo переносит всё, что влезет, и возвращает число перенесённых штук.
func (v *Inventory) MoveAllTo(dst *Inventory) int {
	moved := 0
	for i := range v.slots {
		moved += v.MoveTo(dst, i)
	}
	return moved
}
