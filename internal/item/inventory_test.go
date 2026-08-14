package item_test

import (
	"testing"

	"github.com/vladislav/game/internal/item"
)

// bag — сумка на реальном каталоге: размеры стопок берутся оттуда, и тест
// заодно ловит их изменение.
func bag(t *testing.T, n int) (*item.Inventory, *item.Catalog) {
	t.Helper()
	_, c := load(t)
	return item.NewInventory(c, n), c
}

func stackOf(t *testing.T, c *item.Catalog, id string) int {
	t.Helper()
	it, ok := c.Get(id)
	if !ok {
		t.Fatalf("нет предмета %q", id)
	}
	return it.Stack
}

// TestAddFillsStartedStacks — добивать начатую стопку надо раньше, чем занимать
// пустую ячейку: иначе двадцать подобранных костей займут двадцать ячеек.
func TestAddFillsStartedStacks(t *testing.T) {
	v, c := bag(t, 4)
	max := stackOf(t, c, "bone")
	if max < 3 {
		t.Skipf("стопка костей всего %d — проверять нечего", max)
	}
	v.Add("bone", 1)
	v.Add("bone", 2)
	if got := v.At(0); got.N != 3 {
		t.Errorf("в первой ячейке %d костей, ожидалось 3", got.N)
	}
	if !v.At(1).Empty() {
		t.Errorf("занята вторая ячейка: %+v", v.At(1))
	}
}

// TestAddOverflowsToNextSlot — стопка не резиновая: лишнее уходит в следующую
// ячейку, а не наращивает первую сверх предела.
func TestAddOverflowsToNextSlot(t *testing.T) {
	v, c := bag(t, 4)
	max := stackOf(t, c, "bone")
	if left := v.Add("bone", max+2); left != 0 {
		t.Fatalf("не влезло %d, хотя место было", left)
	}
	if got := v.At(0).N; got != max {
		t.Errorf("в первой ячейке %d, ожидался предел стопки %d", got, max)
	}
	if got := v.At(1).N; got != 2 {
		t.Errorf("во второй ячейке %d, ожидалось 2", got)
	}
	if n := v.Count("bone"); n != max+2 {
		t.Errorf("всего костей %d, положили %d", n, max+2)
	}
}

// TestAddReturnsLeftover — сумка обязана сказать, сколько не влезло. Молчаливое
// проглатывание остатка — это потерянная добыча.
func TestAddReturnsLeftover(t *testing.T) {
	v, c := bag(t, 1)
	max := stackOf(t, c, "bone")
	if left := v.Add("bone", max+5); left != 5 {
		t.Errorf("остаток %d, ожидалось 5", left)
	}
	if n := v.Count("bone"); n != max {
		t.Errorf("в сумке %d, ожидался полный стек %d", n, max)
	}
}

// TestUnknownItemStacksByOne — предмета нет в каталоге, но добыча не должна
// исчезать: он кладётся по одному в ячейку.
func TestUnknownItemStacksByOne(t *testing.T) {
	v, _ := bag(t, 3)
	if left := v.Add("такого-нет", 2); left != 0 {
		t.Fatalf("не влезло %d", left)
	}
	if v.At(0).N != 1 || v.At(1).N != 1 {
		t.Errorf("разложилось не по одному: %+v %+v", v.At(0), v.At(1))
	}
}

// TestMoveToKeepsRemainder — если у приёмника не хватило места, остаток обязан
// остаться в источнике. Это то место, где добыча теряется тише всего.
func TestMoveToKeepsRemainder(t *testing.T) {
	_, c := load(t)
	src := item.NewInventory(c, 2)
	dst := item.NewInventory(c, 1)
	max := stackOf(t, c, "bone")
	src.Add("bone", max)
	dst.Add("bone", max-1) // в приёмнике место ровно на одну кость

	moved := src.MoveTo(dst, 0)
	if moved != 1 {
		t.Errorf("переехало %d, ожидалась 1 кость", moved)
	}
	if got := src.At(0).N; got != max-1 {
		t.Errorf("в источнике осталось %d, ожидалось %d", got, max-1)
	}
	if got := dst.Count("bone"); got != max {
		t.Errorf("в приёмнике %d, ожидался полный стек %d", got, max)
	}
}

// TestMoveAllToEmptiesWhatFits — «забрать всё» переносит помещающееся и не
// теряет то, что не влезло.
func TestMoveAllToEmptiesWhatFits(t *testing.T) {
	_, c := load(t)
	src := item.NewInventory(c, 3)
	dst := item.NewInventory(c, 1)
	src.Add("bone", 2)
	src.Add("coin", 5)
	src.Add("hardwood", 3)

	before := src.Count("bone") + src.Count("coin") + src.Count("hardwood")
	moved := src.MoveAllTo(dst)
	after := src.Count("bone") + src.Count("coin") + src.Count("hardwood")
	if moved <= 0 {
		t.Fatal("не переехало ничего, хотя ячейка у приёмника была")
	}
	if before-after != moved {
		t.Errorf("из источника ушло %d, а приёмник принял %d", before-after, moved)
	}
	if dst.Empty() {
		t.Error("приёмник пуст после переноса")
	}
}

// TestTakeEmptiesSlot — забрали ячейку, значит она пуста.
func TestTakeEmptiesSlot(t *testing.T) {
	v, _ := bag(t, 2)
	v.Add("coin", 7)
	got := v.Take(0)
	if got.ID != "coin" || got.N != 7 {
		t.Errorf("забрали %+v, ожидали 7 монет", got)
	}
	if !v.At(0).Empty() || !v.Empty() {
		t.Error("ячейка осталась занятой")
	}
}
