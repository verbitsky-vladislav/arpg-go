package save

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vladislav/game/internal/worldgen"
)

// testMap — небольшая карта в выходном формате: проверяется хранение, а не
// генерация, поэтому клетки заполнены руками.
func testMap() *worldgen.MapV1 {
	mv := &worldgen.MapV1{
		Format: "map_format v1", Biome: "forest", Seed: 777,
		Width: 4, Height: 4, TileSize: 16,
		Sheets: []worldgen.SheetRef{{Name: "grass", File: "tiles/grass.png", Columns: 8, Count: 64, Firstgid: 1}},
	}
	mv.Layers.Ground = []uint16{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	mv.Layers.Plateau = make([]uint16, 16)
	mv.Props = []worldgen.PropInst{{ID: "tree"}}
	mv.Markers = []worldgen.Marker{{Kind: "spawn", X: 2, Y: 2}}
	mv.Nav.Scale = 4
	return mv
}

// TestMapRoundTrip — карта переживает запись и читается той же. Мир персонажа
// хранится целиком именно так: не сидом, а клетками.
func TestMapRoundTrip(t *testing.T) {
	st := New(filepath.Join(t.TempDir(), "chars.json"))
	mv := testMap()
	if err := st.SaveMap(0, mv); err != nil {
		t.Fatal(err)
	}

	got, ok := st.LoadMap(0)
	if !ok {
		t.Fatal("карта не прочиталась")
	}
	if got.Width != mv.Width || got.Height != mv.Height || got.TileSize != mv.TileSize || got.Seed != mv.Seed {
		t.Errorf("размеры разошлись: %+v", got)
	}
	for i, v := range mv.Layers.Ground {
		if got.Layers.Ground[i] != v {
			t.Fatalf("клетка %d: %d вместо %d", i, got.Layers.Ground[i], v)
		}
	}
	if len(got.Sheets) != 1 || got.Sheets[0].File != "tiles/grass.png" {
		t.Errorf("листы разошлись: %+v", got.Sheets)
	}
	if len(got.Props) != 1 || len(got.Markers) != 1 || got.Nav.Scale != 4 {
		t.Errorf("объекты, метки или сетка физики потерялись: %+v", got)
	}

	// Файл должен быть сжат: карта в разы больше всего остального сохранения.
	fi, err := os.Stat(st.MapPath(0))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() == 0 {
		t.Error("файл карты пуст")
	}
}

// TestMapSlotsAndDelete — карты слотов не путаются, а удаление персонажа уносит
// его мир (иначе новый жилец слота получил бы чужую карту).
func TestMapSlotsAndDelete(t *testing.T) {
	st := New(filepath.Join(t.TempDir(), "chars.json"))
	for i := range Slots {
		mv := testMap()
		mv.Seed = int64(100 + i)
		if err := st.SaveMap(i, mv); err != nil {
			t.Fatal(err)
		}
	}
	for i := range Slots {
		mv, ok := st.LoadMap(i)
		if !ok || mv.Seed != int64(100+i) {
			t.Errorf("в слоте %d чужая карта: %+v (ok=%v)", i, mv, ok)
		}
	}
	st.DeleteMap(1)
	if _, ok := st.LoadMap(1); ok {
		t.Error("удалённая карта вернулась")
	}
	if _, ok := st.LoadMap(0); !ok {
		t.Error("удаление задело соседний слот")
	}
	st.DeleteMap(1) // повторное удаление не должно быть ошибкой
}

// TestBadMapIsIgnored — испорченный, чужой и отсутствующий файл карты читаются
// как «карты нет»: мир соберётся заново из сида, а игра не упадёт.
func TestBadMapIsIgnored(t *testing.T) {
	dir := t.TempDir()
	st := New(filepath.Join(dir, "chars.json"))

	if _, ok := st.LoadMap(0); ok {
		t.Error("из пустоты прочиталась карта")
	}
	if err := os.MkdirAll(filepath.Dir(st.MapPath(0)), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"не gzip":      "просто текст",
		"gzip не json": "\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff мусор",
		"пустая карта": "",
	} {
		if err := os.WriteFile(st.MapPath(0), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, ok := st.LoadMap(0); ok {
			t.Errorf("%s: прочиталась как карта", name)
		}
	}
	if _, ok := st.LoadMap(Slots); ok {
		t.Error("за границами книги нашлась карта")
	}
	if err := st.SaveMap(Slots, testMap()); err == nil {
		t.Error("карта записалась в несуществующий слот")
	}
	if err := st.SaveMap(0, nil); err == nil {
		t.Error("пустая карта записалась")
	}
}

// TestWorldInBook — население мира едет в книге вместе с персонажем.
func TestWorldInBook(t *testing.T) {
	st := New(filepath.Join(t.TempDir(), "chars.json"))
	c := newChar("ХОЗЯИН")
	c.Beasts = []Beast{{Species: "hare", Pos: [2]float64{10, 20}, Floor: 1, HP: 3}}
	c.Foes = []Foe{{Type: "orc", Tier: "t2", Pos: [2]float64{30, 40}, HP: 17, Elite: true}}
	c.Ground = []Drop{{ID: "coin", N: 5, Pos: [2]float64{50, 60}, Floor: 1}}

	book := NewBook()
	book.Put(0, c)
	if err := st.Save(book); err != nil {
		t.Fatal(err)
	}

	got := st.Load().At(0)
	if len(got.Beasts) != 1 || got.Beasts[0] != c.Beasts[0] {
		t.Errorf("звери разошлись: %+v", got.Beasts)
	}
	if len(got.Foes) != 1 || got.Foes[0] != c.Foes[0] {
		t.Errorf("враги разошлись: %+v", got.Foes)
	}
	if len(got.Ground) != 1 || got.Ground[0] != c.Ground[0] {
		t.Errorf("добыча на земле разошлась: %+v", got.Ground)
	}
}
