// Package item — каталог предметов: что такое drop.id из таблиц мобов.
//
// Таблицы зверей и врагов давно раздают добычу идентификаторами (raw_meat,
// slime_gel, coin), но чем этот идентификатор является, нигде не было записано.
// Каталог и есть недостающая половина: имя, род, размер стопки и иконка.
//
// Картинки каталог не грузит сам — он лишь ссылается в иконочные паки
// (internal/atlas). Загрузка отложена до первого показа: в бою иконки не нужны,
// а сумку и бестиарий открывают не каждый забег.
package item

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/vladislav/game/internal/assets"
	"github.com/vladislav/game/internal/atlas"
)

// Kind — род предмета.
const (
	KindMaterial = "material"
	KindValuable = "valuable"
	KindGear     = "gear"
)

// Item — запись каталога.
type Item struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Kind  string `json:"kind"`
	Stack int    `json:"stack"`
	Pack  string `json:"pack"` // каталог иконочного пака внутри assets
	Icon  string `json:"icon"` // id спрайта в манифесте этого пака

	// Slot — в какое гнездо надевается ("" — вещь не носится).
	Slot string `json:"slot,omitempty"`
	// Loadout — какой лоадаут персонажа включает надетая вещь. Пустой лоадаут у
	// вещи с гнездом означает «надеть можно, но драться этим ещё нечем»:
	// анимаций для неё в паке персонажа нет.
	Loadout string `json:"loadout,omitempty"`
}

// Wearable — можно ли вещь надеть.
func (i *Item) Wearable() bool { return i != nil && i.Slot != "" }

// Catalog — все предметы игры.
type Catalog struct {
	Version int               `json:"version"`
	About   string            `json:"about"`
	Kinds   map[string]string `json:"kinds"`
	Items   []*Item           `json:"items"`

	byID  map[string]*Item
	packs map[string]*atlas.Pack // ленивый кэш паков
}

// Load читает каталог из fsys.
func Load(fsys fs.FS, p string) (*Catalog, error) {
	b, err := fs.ReadFile(fsys, p)
	if err != nil {
		return nil, fmt.Errorf("item: чтение %q: %w", p, err)
	}
	var c Catalog
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("item: разбор %q: %w", p, err)
	}
	c.byID = make(map[string]*Item, len(c.Items))
	c.packs = map[string]*atlas.Pack{}
	for _, it := range c.Items {
		if it.ID == "" {
			return nil, fmt.Errorf("item: %q: запись без id", p)
		}
		if _, dup := c.byID[it.ID]; dup {
			return nil, fmt.Errorf("item: %q: предмет %q объявлен дважды", p, it.ID)
		}
		if it.Stack < 1 {
			return nil, fmt.Errorf("item: %q: у %q стопка %d, должна быть хотя бы 1", p, it.ID, it.Stack)
		}
		c.byID[it.ID] = it
	}
	return &c, nil
}

// Get — предмет по id.
func (c *Catalog) Get(id string) (*Item, bool) {
	it, ok := c.byID[id]
	return it, ok
}

// Name — имя предмета для показа. Незнакомый id не прячется: лучше увидеть в
// интерфейсе сырой идентификатор, чем пустое место. По той же причине метод
// терпит nil-каталог: интерфейс, которому каталог не дали, должен показать
// добычу хоть как-то, а не падать.
func (c *Catalog) Name(id string) string {
	if c == nil {
		return id
	}
	if it, ok := c.byID[id]; ok {
		return it.Name
	}
	return id
}

// Names — имена нескольких предметов через запятую, в порядке ids.
func (c *Catalog) Names(ids []string) string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, c.Name(id))
	}
	return strings.Join(out, ", ")
}

// IDs — идентификаторы всех предметов, по алфавиту.
func (c *Catalog) IDs() []string {
	out := make([]string, 0, len(c.byID))
	for id := range c.byID {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// Icon отдаёт иконку предмета, подгружая нужный пак при первом обращении.
func (c *Catalog) Icon(l *assets.Loader, id string) (*ebiten.Image, error) {
	it, ok := c.byID[id]
	if !ok {
		return nil, fmt.Errorf("item: нет предмета %q", id)
	}
	p, ok := c.packs[it.Pack]
	if !ok {
		var err error
		if p, err = atlas.Load(l, it.Pack); err != nil {
			return nil, fmt.Errorf("item: %q: %w", id, err)
		}
		c.packs[it.Pack] = p
	}
	img, ok := p.Sprite(it.Icon)
	if !ok {
		return nil, fmt.Errorf("item: %q: в паке %q нет спрайта %q", id, it.Pack, it.Icon)
	}
	return img, nil
}
