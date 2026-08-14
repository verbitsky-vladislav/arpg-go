package scene

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/vladislav/game/internal/assets"
	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/save"
)

// TestSlotCardsFitScreen — три карточки помещаются в экран и не налезают друг
// на друга. Проверка геометрии, а не рисования: сдвинуть одну константу и
// увести третий слот за край экрана — самая дешёвая ошибка в этом файле.
func TestSlotCardsFitScreen(t *testing.T) {
	var prevRight float32
	for i := range save.Slots {
		x, y, w, h := slotRect(i)
		if x < 0 || x+w > config.ScreenW {
			t.Errorf("слот %d уехал за край: x=%.0f..%.0f при ширине экрана %d", i, x, x+w, config.ScreenW)
		}
		if y < 0 || y+h > config.ScreenH-30 {
			t.Errorf("слот %d налез на подсказку: y=%.0f..%.0f", i, y, y+h)
		}
		if i > 0 && x < prevRight {
			t.Errorf("слот %d налез на предыдущий: %.0f < %.0f", i, x, prevRight)
		}
		prevRight = x + w
	}
}

// TestScreensDraw — экраны сохранения рисуются: и пустой слот, и занятый, и
// журнал персонажа со счётом. Ввод здесь не подделать (его держит Ebiten),
// поэтому проверяется то, что проверить можно: сборка и полный проход отрисовки
// не падают ни на пустых данных, ни на заполненных.
func TestScreensDraw(t *testing.T) {
	l := assets.NewLoader(os.DirFS("../../assets"))
	st := save.New(filepath.Join(t.TempDir(), "chars.json"))

	c := NewChar("ГЕРОЙ", "male")
	c.Level, c.XP, c.Points = 4, 120, 3
	c.Playtime, c.Deaths = 3725, 1
	c.Kill(save.KillAnimal("black_grouse"))
	c.Kill(save.KillEnemy("orc", "t1"))
	book := save.NewBook()
	book.Put(1, c) // первый слот пуст нарочно: рисуются обе половины экрана
	if err := st.Save(book); err != nil {
		t.Fatal(err)
	}

	dst := ebiten.NewImage(config.ScreenW, config.ScreenH)
	p := newProfiles(l, nil, st)
	if p.sel != 1 {
		t.Errorf("курсор встал на слот %d, а живой персонаж во втором", p.sel)
	}
	p.Draw(dst)
	p.confirm = true // подтверждение удаления рисуется поверх карточки
	p.Draw(dst)

	newName(p, "male", "Мужчина", func(string) (Scene, error) { return p, nil }).Draw(dst)
	newJournal(l, c, p).Draw(dst)
	newJournal(l, NewChar("ПУСТОЙ", "female"), p).Draw(dst) // журнал без единого убитого
}

// TestJournalNamesKills — счётчики превращаются в человеческие названия, а
// список идёт от частых к редким. Это единственное место, где игрок видит, кого
// именно он извёл, и «animal:black_grouse» там быть не должно.
func TestJournalNamesKills(t *testing.T) {
	l := assets.NewLoader(os.DirFS("../../assets"))
	rows := killRows(l, map[string]int{
		save.KillAnimal("black_grouse"): 5,
		save.KillEnemy("orc", "t1"):     1,
		save.KillEnemy("goblin", "t1"):  9,
		"выдумка:кто-то":                2, // ключ из будущей версии
	})
	if len(rows) != 4 {
		t.Fatalf("строк %d, ожидалось 4: %+v", len(rows), rows)
	}
	for i := 1; i < len(rows); i++ {
		if rows[i-1].n < rows[i].n {
			t.Errorf("список не по убыванию: %+v", rows)
		}
	}
	byTitle := map[string]int{}
	for _, r := range rows {
		byTitle[r.title] = r.n
	}
	if byTitle["Тетерев"] != 5 {
		t.Errorf("тетерев не назвался по-русски: %+v", byTitle)
	}
	if byTitle["Орк"] != 1 && byTitle["Орк-рубака"] != 1 {
		t.Errorf("орк не назвался по-русски: %+v", byTitle)
	}
	if byTitle["выдумка:кто-то"] != 2 {
		t.Errorf("незнакомый ключ потерялся вместо того, чтобы показаться как есть: %+v", byTitle)
	}
}

// TestPlaytime — время в игре читается человеком.
func TestPlaytime(t *testing.T) {
	cases := map[int]string{0: "0С", 45: "45С", 60: "1М 00С", 3599: "59М 59С", 3600: "1Ч 00М", 7325: "2Ч 02М"}
	for sec, want := range cases {
		if got := playtime(sec); got != want {
			t.Errorf("playtime(%d) = %q, ожидалось %q", sec, got, want)
		}
	}
}
