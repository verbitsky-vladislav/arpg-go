package item_test

import (
	"os"
	"sort"
	"testing"

	"github.com/vladislav/game/internal/assets"
	"github.com/vladislav/game/internal/item"
	"github.com/vladislav/game/internal/mob"
)

const (
	assetsRoot = "../../assets"
	catalogAt  = "items/items.json"
)

func load(t *testing.T) (*assets.Loader, *item.Catalog) {
	t.Helper()
	l := assets.NewLoader(os.DirFS(assetsRoot))
	c, err := item.Load(l.FS(), catalogAt)
	if err != nil {
		t.Fatalf("каталог не читается: %v", err)
	}
	return l, c
}

// dropIDs собирает всё, что таблицы мобов обещают ронять.
func dropIDs(t *testing.T, l *assets.Loader) map[string][]string {
	t.Helper()
	out := map[string][]string{} // drop id -> кто роняет
	add := func(id, who string) { out[id] = append(out[id], who) }

	sp, err := mob.LoadSpecies(l.FS(), "mobs/animals/species.json")
	if err != nil {
		t.Fatalf("species.json: %v", err)
	}
	for _, id := range sp.IDs() {
		for _, d := range sp.Get(id).Drops {
			add(d.ID, "зверь "+id)
		}
	}
	for _, tbl := range []string{"mobs/enemies/enemies.json", "mobs/bosses/bosses.json"} {
		cat, err := mob.LoadEnemies(l.FS(), tbl)
		if err != nil {
			t.Fatalf("%s: %v", tbl, err)
		}
		for _, id := range cat.TypeIDs() {
			for _, d := range cat.Types[id].Drops {
				add(d.ID, "моб "+id)
			}
		}
	}
	return out
}

// TestCatalogCoversDrops — главная проверка каталога: таблицы мобов раздают
// добычу идентификаторами, и каждый такой идентификатор обязан быть предметом.
// Без неё новая строка в drops молча превращается в невидимую добычу.
func TestCatalogCoversDrops(t *testing.T) {
	l, c := load(t)
	drops := dropIDs(t, l)
	if len(drops) == 0 {
		t.Fatal("в таблицах мобов не нашлось ни одной строки добычи — проверять нечего")
	}
	var missing []string
	for id := range drops {
		if _, ok := c.Get(id); !ok {
			missing = append(missing, id)
		}
	}
	sort.Strings(missing)
	for _, id := range missing {
		t.Errorf("добыча %q не описана в %s (роняют: %v)", id, catalogAt, drops[id])
	}
}

// TestIconsResolve — у каждого предмета иконка обязана находиться в своём паке.
// Ссылка на несуществующий спрайт — это дыра в интерфейсе, а не отсутствие
// картинки: рисовать будет нечего.
func TestIconsResolve(t *testing.T) {
	l, c := load(t)
	for _, id := range c.IDs() {
		img, err := c.Icon(l, id)
		if err != nil {
			t.Errorf("%s: %v", id, err)
			continue
		}
		if img.Bounds().Empty() {
			t.Errorf("%s: иконка пустого размера", id)
		}
	}
}

// TestCatalogSane — записи каталога заполнены: род известный, имя есть.
func TestCatalogSane(t *testing.T) {
	_, c := load(t)
	kinds := map[string]bool{
		item.KindMaterial: true, item.KindValuable: true,
		item.KindGear: true, item.KindSkill: true,
	}
	for _, id := range c.IDs() {
		it, _ := c.Get(id)
		if it.Name == "" {
			t.Errorf("%s: пустое имя", id)
		}
		if !kinds[it.Kind] {
			t.Errorf("%s: неизвестный род %q", id, it.Kind)
		}
	}
}

// TestUnusedItems — предмет, который никто не роняет, это не ошибка (впереди
// сундуки, крафт и магазин), но знать о нём полезно.
func TestUnusedItems(t *testing.T) {
	l, c := load(t)
	drops := dropIDs(t, l)
	for _, id := range c.IDs() {
		if _, ok := drops[id]; !ok {
			t.Logf("предмет %q пока никто не роняет", id)
		}
	}
}
