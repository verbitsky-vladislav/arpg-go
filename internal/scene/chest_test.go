package scene

import (
	"math/rand/v2"
	"os"
	"testing"

	"github.com/vladislav/game/internal/assets"
	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/engine"
	"github.com/vladislav/game/internal/item"
	"github.com/vladislav/game/internal/physics"
	"github.com/vladislav/game/internal/ui"
	"github.com/vladislav/game/internal/world"
)

const testAssets = "../../assets"

// TestChestLootIsReal — таблица сундука раздаёт предметы идентификаторами, и
// каждый обязан быть в каталоге. Опечатка в chests.json иначе превращается в
// пустую ячейку с непонятной подписью.
func TestChestLootIsReal(t *testing.T) {
	l := assets.NewLoader(os.DirFS(testAssets))
	cat, err := item.Load(l.FS(), itemsFile)
	if err != nil {
		t.Fatalf("каталог предметов: %v", err)
	}
	cc, err := loadChests(l.FS(), chestsFile)
	if err != nil {
		t.Fatalf("таблица сундуков: %v", err)
	}
	for _, k := range cc.Kinds {
		if k.Title == "" {
			t.Errorf("сундук %q: нет названия для окна", k.ID)
		}
		if len(k.Loot.Entries) == 0 {
			t.Errorf("сундук %q: пустая таблица добычи", k.ID)
		}
		for _, e := range k.Loot.Entries {
			if _, ok := cat.Get(e.ID); !ok {
				t.Errorf("сундук %q: добыча %q не описана в %s", k.ID, e.ID, itemsFile)
			}
			if e.Min < 1 || e.Max < e.Min || e.Weight <= 0 {
				t.Errorf("сундук %q: строка %q задана как min=%d max=%d weight=%d",
					k.ID, e.ID, e.Min, e.Max, e.Weight)
			}
		}
	}
}

// TestChestSpotRules — главное про место сундука: он рядом с героем, но за
// краем экрана, стоит на твёрдой земле нижнего этажа и никогда не на плато.
// Плато отрезано намеренно: туда ведёт единственная лестница, и «рядом» через
// неё превращается в поход через полкарты.
func TestChestSpotRules(t *testing.T) {
	l := assets.NewLoader(os.DirFS(testAssets))
	m, err := world.Generate(l, gameBiome, 7, 96)
	if err != nil {
		t.Fatalf("карта: %v", err)
	}
	from := m.Spawn()
	f := m.Field()
	w, h := m.Size()

	found := 0
	for i := range 120 {
		p, ok := chestSpot(m, from, rand.New(rand.NewPCG(uint64(i), 0x5DEECE66D)))
		if !ok {
			continue
		}
		found++
		d := engine.Dist(p, from)
		if d < chestNear || d > chestFar {
			t.Errorf("сид %d: сундук в %.0f px от героя, ждали %d..%d", i, d, chestNear, chestFar)
		}
		if p.X < 0 || p.Y < 0 || p.X >= w || p.Y >= h {
			t.Errorf("сид %d: сундук за границей мира: %.0f,%.0f", i, p.X, p.Y)
		}
		if c := f.CellAt(p); c != physics.Ground {
			t.Errorf("сид %d: сундук не на земле (клетка %v)", i, c)
		}
		if z := m.Zone(p); z == "plateau" {
			t.Errorf("сид %d: сундук на плато", i)
		}
		if !f.Fits(p, physics.Body{Radius: 10, Floor: physics.FloorLow}) {
			t.Errorf("сид %d: сундук не помещается целиком — к нему не подойти", i)
		}
	}
	if found == 0 {
		t.Fatal("ни одного места под сундук на карте — правила размещения слишком строгие")
	}
	// Сундук должен находиться почти всегда: иначе забеги молча пойдут без него.
	if found < 100 {
		t.Errorf("место нашлось лишь в %d случаях из 120", found)
	}
}

// TestChestInView — «в поле зрения» проверяется не на глаз. Дальняя граница
// обязана быть меньше половины высоты экрана: по вертикали кадр у́же, чем по
// горизонтали, и сундук, поставленный строго над героем, иначе окажется за
// краем — а он должен быть виден с точки старта в любую сторону.
//
// Раньше правило было ровно обратным (сундук прятали за экран, чтобы к нему
// шли), и на игре это читалось как «сундука нет».
func TestChestInView(t *testing.T) {
	halfH := float64(config.ScreenH) / 2
	if chestFar >= halfH {
		t.Errorf("сундук ставится до %d px, а по вертикали видно только %.0f px", chestFar, halfH)
	}
	if chestNear <= chestReach {
		t.Errorf("ближняя граница %d px не дальше вытянутой руки (%d px) — сундук встанет под ногами",
			chestNear, chestReach)
	}
	if chestNear >= chestFar {
		t.Errorf("кольцо постановки пустое: %d..%d", chestNear, chestFar)
	}
}

// TestBagFitsPanel — ячеек в сумке ровно столько, сколько их в окне: лишние
// предметы иначе окажутся в невидимых ячейках, и добыча просто пропадёт.
func TestBagFitsPanel(t *testing.T) {
	l := assets.NewLoader(os.DirFS(testAssets))
	if err := ui.InitPanels(l, "ui/rpg_basic"); err != nil {
		t.Fatalf("окна интерфейса: %v", err)
	}
	for _, c := range []struct {
		window string
		slots  int
		what   string
	}{
		{"equipment", bagSlots, "сумке"},
		{"grid", chestSlots, "сундуке"},
	} {
		p := ui.Window(c.window)
		if p == nil {
			t.Errorf("нет окна %q", c.window)
			continue
		}
		if n := p.Slots(); n != c.slots {
			t.Errorf("в окне %q %d ячеек, а в %s %d", c.window, n, c.what, c.slots)
		}
	}
}

// TestRunHasChest — сквозная проверка: забег собирается и в нём стоит полный
// сундук. Ломается всё это по-разному (нет пака, не сошлась разметка, не нашлось
// места), а выглядит одинаково — сундука на карте просто нет.
func TestRunHasChest(t *testing.T) {
	l := assets.NewLoader(os.DirFS(testAssets))
	g, err := NewGame(l, nil, "male")
	if err != nil {
		t.Fatalf("забег не собрался: %v", err)
	}
	if g.chest == nil {
		t.Fatal("в забеге нет сундука")
	}
	if g.chest.inv.Empty() {
		t.Error("сундук пуст — таблица добычи не сработала")
	}
	if g.chest.frames < 2 {
		t.Errorf("у сундука %d кадр(ов) — анимации открывания нет", g.chest.frames)
	}
	if !g.bag.Empty() {
		t.Error("сумка героя не пуста в начале забега")
	}
	// Добыча из сундука должна помещаться в сумку целиком: иначе первое же
	// «забрать всё» упрётся в потолок на ровном месте.
	if left := g.chest.inv.MoveAllTo(g.bag); left == 0 {
		t.Error("из сундука в пустую сумку не переехало ничего")
	}
	if !g.chest.inv.Empty() {
		t.Error("после переноса в пустую сумку в сундуке что-то осталось")
	}
}

// TestChestGlow — подсветка говорит игроку три разные вещи и не гаснет там, где
// должна звать: закрытый сундук светится всегда, рядом с героем ярче, вскрытый
// едва тлеет.
//
// Меряется среднее по полному циклу мерцания: сравнивать мгновенные значения
// нельзя — они зависят от фазы.
func TestChestGlow(t *testing.T) {
	mean := func(c *chest) float64 {
		var sum float64
		const cycle = 240
		for i := range cycle {
			c.pulse = i
			sum += c.glow()
		}
		return sum / cycle
	}

	idle := mean(&chest{})
	near := mean(&chest{near: true})
	opened := mean(&chest{opened: true})

	if idle <= 0 {
		t.Error("закрытый сундук не подсвечен вовсе — его не найти в траве")
	}
	if near <= idle {
		t.Errorf("рядом с героем подсветка %.2f не ярче обычной %.2f", near, idle)
	}
	if opened >= idle {
		t.Errorf("вскрытый сундук светится как закрытый (%.2f против %.2f)", opened, idle)
	}
	if opened <= 0 {
		t.Error("вскрытый сундук погас совсем — на карте его будет не найти")
	}

	// Мерцание должно быть заметным, но не мельканием: размах в пределах
	// четверти от силы.
	c := &chest{}
	lo, hi := 1.0, 0.0
	for i := range 240 {
		c.pulse = i
		lo, hi = min(lo, c.glow()), max(hi, c.glow())
	}
	if hi-lo > idle*0.5 {
		t.Errorf("подсветка мигает слишком сильно: размах %.2f при силе %.2f", hi-lo, idle)
	}
}
