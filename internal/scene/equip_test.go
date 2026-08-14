package scene

import (
	"math/rand/v2"
	"os"
	"testing"

	"github.com/vladislav/game/internal/assets"
	"github.com/vladislav/game/internal/item"
	"github.com/vladislav/game/internal/ui"
)

// TestChestAlwaysHasSword — первый клинок обязан находиться в сундуке при любом
// броске. Он не украшение: без оружия герой не бьёт вообще, и забег, в котором
// клинок не выпал, играть нечем.
func TestChestAlwaysHasSword(t *testing.T) {
	l := assets.NewLoader(os.DirFS(testAssets))
	cat, err := item.Load(l.FS(), itemsFile)
	if err != nil {
		t.Fatalf("каталог предметов: %v", err)
	}
	cc, err := loadChests(l.FS(), chestsFile)
	if err != nil {
		t.Fatalf("таблица сундуков: %v", err)
	}
	kind := cc.Kinds[0]
	for i := range 60 {
		c := &chest{kind: kind, inv: item.NewInventory(cat, chestSlots)}
		c.fill(rand.New(rand.NewPCG(uint64(i), 0x9E3779B9)))
		if n := c.inv.Count("rusty_blade"); n < 1 {
			t.Fatalf("бросок %d: в сундуке нет клинка", i)
		}
	}
}

// TestEquipSwordArmsHero — главное правило боя: без оружия герой не атакует, с
// надетым клинком атакует, снял — снова не может. Проверяется через Armed(),
// потому что именно на нём стоит запрет удара.
func TestEquipSwordArmsHero(t *testing.T) {
	l := assets.NewLoader(os.DirFS(testAssets))
	g, err := NewGame(l, nil, "male")
	if err != nil {
		t.Fatalf("забег не собрался: %v", err)
	}
	if g.pl.Armed() {
		t.Fatal("герой вооружён с самого начала — драться должно быть нечем")
	}

	g.bag.Add("rusty_blade", 1)
	if note := g.equipFromBag(0); note == "СУМКА ПОЛНА" || note == "ЭТО НЕ НАДЕТЬ" || note == "НЕТ АНИМАЦИЙ" {
		t.Fatalf("клинок не надевается: %s", note)
	}
	if !g.pl.Armed() {
		t.Error("клинок надет, а бить всё ещё нечем")
	}
	if got := g.pl.Loadout.Art; got != "sword" {
		t.Errorf("лоадаут героя %q, ожидался sword", got)
	}
	if s := g.eq.At(item.SlotWeapon); s.ID != "rusty_blade" {
		t.Errorf("в гнезде оружия %+v", s)
	}
	if !g.bag.At(0).Empty() {
		t.Error("надетая вещь осталась ещё и в сумке")
	}

	if note := g.unequip(item.SlotWeapon); note == "СУМКА ПОЛНА" {
		t.Fatal("снять оружие некуда, хотя сумка почти пуста")
	}
	if g.pl.Armed() {
		t.Error("оружие снято, а герой всё ещё может бить")
	}
	if g.bag.Count("rusty_blade") != 1 {
		t.Error("снятый клинок не вернулся в сумку")
	}
}

// TestEquipRejectsPlainItem — материал не надевается: у него нет гнезда.
func TestEquipRejectsPlainItem(t *testing.T) {
	l := assets.NewLoader(os.DirFS(testAssets))
	g, err := NewGame(l, nil, "female")
	if err != nil {
		t.Fatalf("забег не собрался: %v", err)
	}
	g.bag.Add("bone", 3)
	if note := g.equipFromBag(0); note != "ЭТО НЕ НАДЕТЬ" {
		t.Errorf("кость надели, ответ: %q", note)
	}
	if g.bag.Count("bone") != 3 {
		t.Error("кости пропали из сумки при попытке надеть")
	}
	if g.pl.Armed() {
		t.Error("кость вооружила героя")
	}
}

// TestEquipSlotsHaveArt — каждое гнездо снаряжения обязано быть на арте окна:
// невидимое гнездо означает предмет, который некуда положить.
func TestEquipSlotsHaveArt(t *testing.T) {
	l := assets.NewLoader(os.DirFS(testAssets))
	cat, err := item.Load(l.FS(), itemsFile)
	if err != nil {
		t.Fatalf("каталог предметов: %v", err)
	}
	if err := ui.InitPanels(l, "ui/rpg_basic"); err != nil {
		t.Fatalf("окна интерфейса: %v", err)
	}
	p := ui.Window("equipment")
	if p == nil {
		t.Fatal("нет окна equipment")
	}
	slots := map[string]bool{}
	for _, name := range p.EquipSlots() {
		slots[name] = true
	}
	for _, id := range cat.IDs() {
		it, _ := cat.Get(id)
		if it.Wearable() && !slots[it.Slot] {
			t.Errorf("предмет %q надевается в гнездо %q, которого нет в окне", id, it.Slot)
		}
	}
}
