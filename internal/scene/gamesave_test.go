package scene

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vladislav/game/internal/assets"
	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/engine"
	"github.com/vladislav/game/internal/item"
	"github.com/vladislav/game/internal/save"
)

// weaponID — первый носимый в гнездо оружия предмет каталога. Тест не должен
// знать названий вещей: они меняются в данных, а проверяется здесь другое.
func weaponID(t *testing.T, cat *item.Catalog) string {
	t.Helper()
	for _, id := range cat.IDs() {
		if it, ok := cat.Get(id); ok && it.Slot == item.SlotWeapon && it.Loadout != "" {
			return id
		}
	}
	t.Fatal("в каталоге нет ни одного оружия")
	return ""
}

// TestRunSurvivesSave — забег переживает выход и возвращение: тот же мир, тот
// же уровень, та же добыча, тот же счёт убитых и та же точка на карте.
//
// Проверка сквозная нарочно: по отдельности снимок и сборка мира уже проверены
// (internal/save, TestGameRunsWithEnemies), а сломаться скорее всего может стык
// — то, что снимок снимают не с того, накладывают не туда или теряют по дороге.
func TestRunSurvivesSave(t *testing.T) {
	l := assets.NewLoader(os.DirFS("../../assets"))
	st := save.New(filepath.Join(t.TempDir(), "chars.json"))

	g, err := NewSavedGame(l, nil, st, 0, NewChar("ГЕРОЙ", "male"))
	if err != nil {
		t.Fatal(err)
	}

	// Нажитое: уровень, добыча в сумке, оружие в руках, счёт убитых, раны.
	g.prog.Add(250)
	if g.prog.Level < 2 {
		t.Fatalf("опыт не поднял уровень: %+v", g.prog)
	}
	sword := weaponID(t, g.items)
	g.bag.Add("coin", 17)
	g.bag.Add(sword, 1)
	if note := g.equipFromBag(bagSlotOf(t, g, sword)); note == "" {
		t.Fatal("оружие не надевается")
	}
	g.count(save.KillAnimal("black_grouse"))
	g.count(save.KillAnimal("black_grouse"))
	g.count(save.KillEnemy("orc", "t1"))
	g.pl.HP = max(g.pl.MaxHP-7, 1)

	// Уход подальше от старта: точка выхода обязана вернуться своя, а не
	// точка появления.
	spawn := g.m.Spawn()
	g.pl.Place(g.m.Field(), engine.Vec2{X: spawn.X + 96, Y: spawn.Y})
	left := g.pl.Pos
	if engine.Dist(left, spawn) < 1 {
		t.Fatal("герой не сдвинулся со старта — проверять нечего")
	}

	// Полминуты игры и выход в меню (он и сохраняет).
	for range config.TPS {
		g.tickSave()
	}
	g.toMenu()

	c := st.Load().At(0)
	if c == nil {
		t.Fatal("забег не сохранился")
	}
	if c.Name != "ГЕРОЙ" || c.Body != "male" {
		t.Errorf("сохранён не тот персонаж: %+v", c)
	}
	if c.Playtime != 1 {
		t.Errorf("время игры %d с, ожидалась 1", c.Playtime)
	}

	g2, err := NewSavedGame(l, nil, st, 0, c)
	if err != nil {
		t.Fatal(err)
	}
	if g2.m.Seed() != g.m.Seed() {
		t.Errorf("мир пересобрался другим: сид %d вместо %d", g2.m.Seed(), g.m.Seed())
	}
	if g2.prog != g.prog {
		t.Errorf("прокачка вернулась как %+v, ожидалась %+v", g2.prog, g.prog)
	}
	if n := g2.bag.Count("coin"); n != 17 {
		t.Errorf("в сумке %d монет, ожидалось 17", n)
	}
	if worn := g2.eq.At(item.SlotWeapon); worn.ID != sword {
		t.Errorf("в гнезде оружия %q, ожидалось %q", worn.ID, sword)
	}
	if g2.pl.Loadout.ID != g.pl.Loadout.ID {
		t.Errorf("герой дерётся лоадаутом %q вместо %q", g2.pl.Loadout.ID, g.pl.Loadout.ID)
	}
	if g2.pl.HP != g.pl.HP {
		t.Errorf("здоровье %d, ожидалось %d", g2.pl.HP, g.pl.HP)
	}
	if d := engine.Dist(g2.pl.Pos, left); d > 1 {
		t.Errorf("герой вернулся в %v вместо %v (разница %.1f px)", g2.pl.Pos, left, d)
	}
	if n := g2.char.Kills[save.KillAnimal("black_grouse")]; n != 2 {
		t.Errorf("тетеревов %d, ожидалось 2", n)
	}
	if n := g2.char.Kills[save.KillEnemy("orc", "t1")]; n != 1 {
		t.Errorf("орков %d, ожидался 1", n)
	}

	// Счёт продолжается, а не начинается заново: следующий забег добавляет к
	// прежнему.
	g2.count(save.KillAnimal("black_grouse"))
	g2.persist()
	if n := st.Load().At(0).Kills[save.KillAnimal("black_grouse")]; n != 3 {
		t.Errorf("после второго забега тетеревов %d, ожидалось 3", n)
	}
}

// TestChestStaysLooted — разграбленный сундук остаётся разграбленным: иначе
// один и тот же сундук отдавал бы добычу каждый заход.
func TestChestStaysLooted(t *testing.T) {
	l := assets.NewLoader(os.DirFS("../../assets"))
	st := save.New(filepath.Join(t.TempDir(), "chars.json"))

	g, err := NewSavedGame(l, nil, st, 0, NewChar("ВОР", "female"))
	if err != nil {
		t.Fatal(err)
	}
	if g.chest == nil {
		t.Skip("на этой карте сундуку не нашлось места")
	}
	if g.chest.inv.Empty() {
		t.Fatal("сундук пуст ещё до грабежа")
	}
	g.chest.begin()
	g.chest.opened = true
	g.chest.inv.MoveAllTo(g.bag)
	g.persist()

	g2, err := NewSavedGame(l, nil, st, 0, st.Load().At(0))
	if err != nil {
		t.Fatal(err)
	}
	if g2.chest == nil {
		t.Fatal("сундук не восстановился, хотя место на карте то же")
	}
	if !g2.chest.opened {
		t.Error("сундук снова закрыт")
	}
	if !g2.chest.inv.Empty() {
		t.Errorf("в разграбленном сундуке снова добыча: %+v", g2.chest.inv.Slots())
	}
}

// TestDeathSavesAlive — забег, брошенный на экране смерти, возвращается живым и
// на старте. Смерть стоит пройденного пути, а не персонажа: сохранение, из
// которого поднимается труп, было бы концом игры без выхода.
func TestDeathSavesAlive(t *testing.T) {
	l := assets.NewLoader(os.DirFS("../../assets"))
	st := save.New(filepath.Join(t.TempDir(), "chars.json"))

	g, err := NewSavedGame(l, nil, st, 0, NewChar("ПАВШИЙ", "male"))
	if err != nil {
		t.Fatal(err)
	}
	spawn := g.m.Spawn()
	g.pl.Place(g.m.Field(), engine.Vec2{X: spawn.X + 96, Y: spawn.Y})
	g.pl.Kill()
	g.died()

	c := st.Load().At(0)
	if c == nil {
		t.Fatal("смерть не сохранилась")
	}
	if c.Deaths != 1 {
		t.Errorf("смертей %d, ожидалась 1", c.Deaths)
	}
	if c.HP <= 0 {
		t.Errorf("в сохранении здоровье %d — герой остался трупом", c.HP)
	}

	g2, err := NewSavedGame(l, nil, st, 0, c)
	if err != nil {
		t.Fatal(err)
	}
	if !g2.pl.Alive() {
		t.Error("из сохранения поднялся труп")
	}
	if d := engine.Dist(g2.pl.Pos, spawn); d > 1 {
		t.Errorf("погибший вернулся в %v, а старт карты в %v", g2.pl.Pos, spawn)
	}
}

// TestNewCharStartsClean — новый персонаж начинает с чистого листа: первый
// уровень, полное здоровье, точка старта и пустой счёт.
func TestNewCharStartsClean(t *testing.T) {
	l := assets.NewLoader(os.DirFS("../../assets"))
	c := NewChar("  НОВЫЙ  ", "male")
	if c.Name != "НОВЫЙ" {
		t.Errorf("имя %q не вычищено", c.Name)
	}
	if c.Seed == 0 {
		t.Error("у нового персонажа нет своего мира")
	}
	g, err := NewSavedGame(l, nil, nil, -1, c)
	if err != nil {
		t.Fatal(err)
	}
	if g.prog.Level != 1 || g.prog.XP != 0 {
		t.Errorf("новый герой начинает с %+v", g.prog)
	}
	if g.pl.HP != g.pl.MaxHP {
		t.Errorf("здоровье %d из %d", g.pl.HP, g.pl.MaxHP)
	}
	if d := engine.Dist(g.pl.Pos, g.m.Spawn()); d > 1 {
		t.Errorf("герой появился в %v, а старт карты в %v", g.pl.Pos, g.m.Spawn())
	}
	if !g.bag.Empty() || c.KillTotal() != 0 {
		t.Error("новый персонаж начинает с добычей или счётом")
	}
	// Забег без книги сохранений не должен ни падать, ни писать на диск.
	g.persist()
	g.tickSave()
}

// bagSlotOf — в какой ячейке сумки лежит предмет id.
func bagSlotOf(t *testing.T, g *Game, id string) int {
	t.Helper()
	for i, s := range g.bag.Slots() {
		if s.ID == id {
			return i
		}
	}
	t.Fatalf("предмета %q нет в сумке", id)
	return -1
}
