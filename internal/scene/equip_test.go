package scene

import (
	"math/rand/v2"
	"os"
	"testing"

	"github.com/vladislav/game/internal/assets"
	"github.com/vladislav/game/internal/item"
	"github.com/vladislav/game/internal/ui"
)

// TestChestAlwaysHasWeapon — оружие обязано находиться в сундуке при любом
// броске. Оно не украшение: голыми руками герой снимает 1-2, и забег, в котором
// оружие не выпало, играть нечем. Ищется не конкретный предмет, а любая вещь с
// боевыми свойствами: чем именно игра начинается — дело таблицы.
func TestChestAlwaysHasWeapon(t *testing.T) {
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
		armed := false
		for s := range chestSlots {
			if id := c.inv.At(s).ID; id != "" && cat.Weapon(id) != nil {
				armed = true
				break
			}
		}
		if !armed {
			t.Fatalf("бросок %d: в сундуке нет ни одного оружия", i)
		}
	}
}

// TestEquipWeaponRaisesDamage — главное правило боя: надетое оружие бьёт своим
// уроном, снятое возвращает героя к базовому. Голые руки при этом остаются
// рабочими: бить герой умеет всегда, вопрос только в числах.
func TestEquipWeaponRaisesDamage(t *testing.T) {
	l := assets.NewLoader(os.DirFS(testAssets))
	g, err := NewGame(l, nil, "male")
	if err != nil {
		t.Fatalf("забег не собрался: %v", err)
	}
	base := g.pl.Power().Damage
	if base != g.chars.Base.Damage {
		t.Fatalf("герой начинает не с базового урона: %+v вместо %+v", base, g.chars.Base.Damage)
	}
	if !g.pl.Armed() {
		t.Error("герою нечем замахнуться даже голыми руками")
	}

	g.bag.Add("rusty_blade", 1)
	if note := g.equipFromBag(0); note == "СУМКА ПОЛНА" || note == "ЭТО НЕ НАДЕТЬ" || note == "НЕТ АНИМАЦИЙ" {
		t.Fatalf("клинок не надевается: %s", note)
	}
	blade := g.items.Weapon("rusty_blade")
	if blade == nil {
		t.Fatal("у клинка нет боевых свойств")
	}
	if got := g.pl.Power().Damage; got != blade.Damage {
		t.Errorf("с клинком урон %+v вместо его собственного %+v", got, blade.Damage)
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
	if got := g.pl.Power().Damage; got != base {
		t.Errorf("оружие снято, а урон остался %+v вместо базового %+v", got, base)
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
	base := g.pl.Power().Damage
	g.bag.Add("bone", 3)
	if note := g.equipFromBag(0); note != "ЭТО НЕ НАДЕТЬ" {
		t.Errorf("кость надели, ответ: %q", note)
	}
	if g.bag.Count("bone") != 3 {
		t.Error("кости пропали из сумки при попытке надеть")
	}
	if got := g.pl.Power().Damage; got != base {
		t.Errorf("кость вооружила героя: урон %+v вместо %+v", got, base)
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
