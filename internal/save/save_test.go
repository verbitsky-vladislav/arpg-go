package save

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newChar — минимально живой персонаж для проверок.
func newChar(name string) *Char {
	c := &Char{Name: name, Body: "male", Seed: 4242, Biome: "forest", Level: 3, XP: 40}
	c.Touch(time.Now())
	return c
}

// TestRoundTrip — записанное читается обратно тем же. Проверка формата целиком:
// всё, что игрок нажил, должно пережить запись на диск.
func TestRoundTrip(t *testing.T) {
	st := New(filepath.Join(t.TempDir(), "chars.json"))

	c := newChar("ГЕРОЙ")
	c.Points = 2
	c.HP = 37
	c.Pos = [2]float64{123.5, 456.25}
	c.Playtime, c.Deaths = 754, 2
	c.Kill(KillAnimal("black_grouse"))
	c.Kill(KillAnimal("black_grouse"))
	c.Kill(KillEnemy("orc", "t2"))
	c.Bag = []Slot{{ID: "coin", N: 42}, {}, {ID: "raw_meat", N: 3}}
	c.Worn = map[string]Slot{"weapon": {ID: "sword_iron", N: 1}}
	c.Chest = &Chest{Opened: true, Slots: []Slot{{ID: "coin", N: 7}}}

	book := NewBook()
	book.Put(1, c)
	if err := st.Save(book); err != nil {
		t.Fatal(err)
	}

	got := st.Load().At(1)
	if got == nil {
		t.Fatal("персонаж не прочитался")
	}
	if got.Name != "ГЕРОЙ" || got.Body != "male" || got.Seed != 4242 || got.Biome != "forest" {
		t.Errorf("паспорт разошёлся: %+v", got)
	}
	if got.Level != 3 || got.XP != 40 || got.Points != 2 || got.HP != 37 {
		t.Errorf("прокачка разошлась: level=%d xp=%d points=%d hp=%d", got.Level, got.XP, got.Points, got.HP)
	}
	if got.Pos != c.Pos {
		t.Errorf("точка выхода %v, ожидалась %v", got.Pos, c.Pos)
	}
	if got.Playtime != 754 || got.Deaths != 2 {
		t.Errorf("время %d, смертей %d", got.Playtime, got.Deaths)
	}
	if n := got.Kills[KillAnimal("black_grouse")]; n != 2 {
		t.Errorf("тетеревов %d, ожидалось 2", n)
	}
	if n := got.Kills[KillEnemy("orc", "t2")]; n != 1 {
		t.Errorf("орков %d, ожидался 1", n)
	}
	if got.KillTotal() != 3 {
		t.Errorf("всего убито %d, ожидалось 3", got.KillTotal())
	}
	if len(got.Bag) != 3 || got.Bag[0] != (Slot{ID: "coin", N: 42}) || got.Bag[2].N != 3 {
		t.Errorf("сумка разошлась: %+v", got.Bag)
	}
	if got.Worn["weapon"].ID != "sword_iron" {
		t.Errorf("надетое разошлось: %+v", got.Worn)
	}
	if got.Chest == nil || !got.Chest.Opened || len(got.Chest.Slots) != 1 {
		t.Errorf("сундук разошёлся: %+v", got.Chest)
	}
	if got.Created == "" || got.Updated == "" {
		t.Error("время создания и правки не записалось")
	}
}

// TestSlotsIndependent — слоты не путаются между собой, а удаление одного не
// трогает соседей: у каждого персонажа своя жизнь.
func TestSlotsIndependent(t *testing.T) {
	st := New(filepath.Join(t.TempDir(), "chars.json"))
	book := NewBook()
	for i := range Slots {
		c := newChar(string(rune('А' + i)))
		c.Seed = int64(100 + i)
		book.Put(i, c)
	}
	if free := book.Free(); free != -1 {
		t.Errorf("книга полна, а свободным считается слот %d", free)
	}
	if err := st.Save(book); err != nil {
		t.Fatal(err)
	}

	book = st.Load()
	book.Delete(1)
	if err := st.Save(book); err != nil {
		t.Fatal(err)
	}

	book = st.Load()
	if book.At(1) != nil {
		t.Error("удалённый слот вернулся")
	}
	if book.Free() != 1 {
		t.Errorf("свободным считается слот %d, ожидался 1", book.Free())
	}
	if book.At(0) == nil || book.At(0).Seed != 100 || book.At(2) == nil || book.At(2).Seed != 102 {
		t.Error("удаление задело соседние слоты")
	}
	if book.At(Slots) != nil {
		t.Error("за границами книги нашёлся персонаж")
	}
}

// TestBadFileIsEmpty — битый, чужой и отсутствующий файл читаются одинаково: как
// «сохранений нет». Игра из-за них не падает и не показывает полутрупов.
func TestBadFileIsEmpty(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]string{
		"мусор":            "не json вовсе",
		"чужая версия":     `{"version":999,"chars":[null,null,null]}`,
		"запись без имени": `{"version":1,"chars":[{"body":"male","level":4},null,null]}`,
		"запись без тела":  `{"version":1,"chars":[{"name":"ГЕРОЙ"},null,null]}`,
	}
	for name, body := range cases {
		p := filepath.Join(dir, name+".json")
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if c := New(p).Load().At(0); c != nil {
			t.Errorf("%s: прочитался персонаж %+v", name, c)
		}
	}
	if c := New(filepath.Join(dir, "нет-такого.json")).Load().At(0); c != nil {
		t.Errorf("из пустоты прочитался персонаж %+v", c)
	}
}

// TestLoadFixes — запись из прошлой версии игры чинится, а не отвергается:
// нулевой уровень, отрицательное время и пустые ячейки не должны стоить игроку
// персонажа.
func TestLoadFixes(t *testing.T) {
	p := filepath.Join(t.TempDir(), "chars.json")
	raw := `{"version":1,"chars":[{"name":"  ГЕРОЙ  ","body":"male","level":0,
		"xp":-5,"points":-1,"playtime":-3,"kills":{"animal:hare":0,"":4},
		"worn":{"weapon":{"id":"","n":0}}},null,null]}`
	if err := os.WriteFile(p, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	c := New(p).Load().At(0)
	if c == nil {
		t.Fatal("чинимая запись отвергнута")
	}
	if c.Name != "ГЕРОЙ" {
		t.Errorf("имя %q, ожидалось %q", c.Name, "ГЕРОЙ")
	}
	if c.Level != 1 || c.XP != 0 || c.Points != 0 || c.Playtime != 0 {
		t.Errorf("числа не поправлены: %+v", c)
	}
	if c.Biome == "" {
		t.Error("биом остался пустым — карту будет не из чего собрать")
	}
	if len(c.Kills) != 0 {
		t.Errorf("пустые счётчики остались: %v", c.Kills)
	}
	if len(c.Worn) != 0 {
		t.Errorf("пустое гнездо осталось: %v", c.Worn)
	}
}

// TestSaveOverwritesAtomically — вторая запись заменяет первую целиком и не
// оставляет рядом временных файлов.
func TestSaveOverwritesAtomically(t *testing.T) {
	dir := t.TempDir()
	st := New(filepath.Join(dir, "chars.json"))

	book := NewBook()
	book.Put(0, newChar("ПЕРВЫЙ"))
	if err := st.Save(book); err != nil {
		t.Fatal(err)
	}
	book.Put(0, newChar("ВТОРОЙ"))
	if err := st.Save(book); err != nil {
		t.Fatal(err)
	}

	if got := st.Load().At(0); got == nil || got.Name != "ВТОРОЙ" {
		t.Errorf("после перезаписи в слоте %+v", got)
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 1 {
		t.Errorf("рядом с сохранением остались файлы: %v", ents)
	}

	// Файл должен оставаться читаемым JSON: его правят руками при отладке.
	b, err := os.ReadFile(st.Path())
	if err != nil {
		t.Fatal(err)
	}
	var probe map[string]any
	if err := json.Unmarshal(b, &probe); err != nil {
		t.Errorf("сохранение перестало быть json: %v", err)
	}
}

// TestKillKeys — ключи счётчика разбираются обратно, а зверь и враг с одним
// именем не сливаются в один счёт (rat есть и в species.json, и в enemies.json).
func TestKillKeys(t *testing.T) {
	if KillAnimal("rat") == KillEnemy("rat", "t1") {
		t.Fatal("зверь и враг с одним именем дают один ключ")
	}
	kind, id, tier, ok := SplitKill(KillAnimal("black_grouse"))
	if !ok || kind != KindAnimal || id != "black_grouse" || tier != "" {
		t.Errorf("зверь разобрался как %q/%q/%q (ok=%v)", kind, id, tier, ok)
	}
	kind, id, tier, ok = SplitKill(KillEnemy("orc", "t3"))
	if !ok || kind != KindEnemy || id != "orc" || tier != "t3" {
		t.Errorf("враг разобрался как %q/%q/%q (ok=%v)", kind, id, tier, ok)
	}
	for _, bad := range []string{"", "orc", "boss:slime", "enemy:", "enemy:orc", "выдумка:orc/t1"} {
		if _, _, _, ok := SplitKill(bad); ok {
			t.Errorf("ключ %q принят за свой", bad)
		}
	}
}

// TestCleanName — имя приводится к тому, что игра покажет: без краевых
// пробелов, без двойных внутри и не длиннее предела.
func TestCleanName(t *testing.T) {
	cases := map[string]string{
		"  ГЕРОЙ  ":   "ГЕРОЙ",
		"ЗЛОЙ   ВОЛК": "ЗЛОЙ ВОЛК",
		"   ":         "",
		"ОЧЕНЬДЛИННОЕИМЯГЕРОЯ": "ОЧЕНЬДЛИННОЕ",
	}
	for in, want := range cases {
		if got := CleanName(in); got != want {
			t.Errorf("CleanName(%q) = %q, ожидалось %q", in, got, want)
		}
	}
	if n := len([]rune(CleanName("АБВГДЕЁЖЗИЙКЛМНОП"))); n > NameLimit {
		t.Errorf("имя длиной %d знаков, предел %d", n, NameLimit)
	}
}
