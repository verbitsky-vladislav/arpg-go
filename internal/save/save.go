// Package save — сохранение игрового прогресса на диск.
//
// Персонажей ровно три: игра — это не бесконечная песочница, а три отдельные
// судьбы, каждая со своим миром (сид карты), своей добычей и своим счётом
// убитых. Поэтому всё лежит в одном файле книгой из трёх слотов, а не россыпью
// по каталогу: слот пустой или занят, третьего состояния нет.
//
// Файл: <os.UserConfigDir>/pixel-arpg/chars.json (fallback — рядом с бинарником,
// как у settings). Чтение — best-effort: битый или чужой файл не валит игру, он
// просто читается как «сохранений нет». Запись — атомарная (временный файл +
// переименование): вылет посреди записи не должен превращать сохранение в мусор.
package save

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Slots — сколько персонажей помещается в книгу.
const Slots = 3

// version — версия формата. Файл другой версии читается как пустой: миграций
// пока нет, и молча подсунуть игроку полуразобранное сохранение хуже, чем
// начать заново.
const version = 1

// NameLimit — предел длины имени персонажа в символах.
const NameLimit = 12

// Slot — ячейка сумки: что лежит и сколько. Повторяет item.Slot нарочно: формат
// файла не должен ездить следом за внутренним устройством предметов.
type Slot struct {
	ID string `json:"id"`
	N  int    `json:"n"`
}

// Chest — сундук забега. Сам сундук (вид, место, содержимое) детерминированно
// восстанавливается из сида, поэтому сохраняется только то, что игрок в нём
// изменил: открыт ли он и что в нём осталось.
type Chest struct {
	Opened bool   `json:"opened"`
	Slots  []Slot `json:"slots"`
}

// Char — один персонаж целиком: кто он, в каком мире и что успел.
type Char struct {
	Name string `json:"name"`
	Body string `json:"body"` // тело из character.json (male/female)

	// Мир персонажа. Сид и биом — это и есть карта: по ним она собирается заново
	// со всеми деревьями, водой и точкой старта.
	Seed  int64  `json:"seed"`
	Biome string `json:"biome"`

	// Прокачка — та же тройка, что в progress.Character: уровень, опыт внутри
	// него и нерастраченные очки.
	Level  int `json:"level"`
	XP     int `json:"xp"`
	Points int `json:"points"`

	HP  int        `json:"hp"`
	Pos [2]float64 `json:"pos"` // где вышли из забега
	// Floor — этаж точки выхода (0 — низ, 1 — макушка плато). Без него
	// вернувшийся на плато герой оказался бы под обрывом: одна и та же точка
	// проходима для разных этажей по-разному.
	Floor uint8 `json:"floor"`

	// Kills — убитые по видам: ключи строит KillAnimal/KillEnemy.
	Kills map[string]int `json:"kills"`

	Bag   []Slot          `json:"bag"`
	Worn  map[string]Slot `json:"worn"` // гнездо → надетая вещь
	Chest *Chest          `json:"chest,omitempty"`

	Playtime int `json:"playtime"` // секунд в забеге
	Deaths   int `json:"deaths"`

	Created string `json:"created"` // RFC3339, проставляется один раз
	Updated string `json:"updated"`
}

// Book — файл сохранений: три слота, пустой слот — nil.
type Book struct {
	Version int          `json:"version"`
	Chars   [Slots]*Char `json:"chars"`
}

// NewBook — пустая книга.
func NewBook() *Book { return &Book{Version: version} }

// At — персонаж в слоте i (nil — слот пуст или индекс за границами).
func (b *Book) At(i int) *Char {
	if b == nil || i < 0 || i >= Slots {
		return nil
	}
	return b.Chars[i]
}

// Put кладёт персонажа в слот i.
func (b *Book) Put(i int, c *Char) {
	if b == nil || i < 0 || i >= Slots {
		return
	}
	b.Chars[i] = c
}

// Delete освобождает слот.
func (b *Book) Delete(i int) { b.Put(i, nil) }

// Free — первый свободный слот (-1, если все три заняты).
func (b *Book) Free() int {
	for i := range b.Chars {
		if b.Chars[i] == nil {
			return i
		}
	}
	return -1
}

// Store — файл сохранений на диске.
type Store struct{ path string }

// New — хранилище по явному пути (тесты, отладка, портативный запуск).
func New(path string) *Store { return &Store{path: path} }

// Default — хранилище в пользовательском каталоге ОС, рядом с настройками.
func Default() *Store {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		return New("chars.json") // fallback — рядом с бинарником
	}
	return New(filepath.Join(dir, "pixel-arpg", "chars.json"))
}

// Path — где лежит файл (для сообщений и тестов).
func (s *Store) Path() string { return s.path }

// Load читает книгу. Нет файла, битый JSON, чужая версия — пустая книга: играть
// это не мешает, а прежние сохранения не затираются до первой записи.
func (s *Store) Load() *Book {
	b, err := os.ReadFile(s.path)
	if err != nil {
		return NewBook()
	}
	var bk Book
	if json.Unmarshal(b, &bk) != nil || bk.Version != version {
		return NewBook()
	}
	bk.Version = version
	for i, c := range bk.Chars {
		if c == nil {
			continue
		}
		if !c.sane() {
			bk.Chars[i] = nil // полупустая запись — тот же мусор, что и битый файл
			continue
		}
		c.fix()
	}
	return &bk
}

// Save пишет книгу целиком, атомарно: сначала временный файл рядом, потом
// переименование поверх. Так прерванная запись не съедает то, что уже было.
func (s *Store) Save(b *Book) error {
	if b == nil {
		return fmt.Errorf("save: пустая книга")
	}
	b.Version = version
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return fmt.Errorf("save: сборка %q: %w", s.path, err)
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("save: каталог %q: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".chars-*.json")
	if err != nil {
		return fmt.Errorf("save: временный файл в %q: %w", dir, err)
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return fmt.Errorf("save: запись %q: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return fmt.Errorf("save: закрытие %q: %w", name, err)
	}
	if err := os.Rename(name, s.path); err != nil {
		os.Remove(name)
		return fmt.Errorf("save: переименование в %q: %w", s.path, err)
	}
	return nil
}

// sane — похожа ли запись на живого персонажа. Без имени и тела восстанавливать
// нечего: такой слот честнее показать пустым.
func (c *Char) sane() bool { return CleanName(c.Name) != "" && c.Body != "" }

// fix приводит запись в рабочий вид: карты не nil, числа в разумных границах.
// Правится молча — файл мог остаться от прошлой версии игры, и падать из-за
// нуля в уровне незачем.
func (c *Char) fix() {
	c.Name = CleanName(c.Name)
	if c.Level < 1 {
		c.Level = 1
	}
	if c.XP < 0 {
		c.XP = 0
	}
	if c.Points < 0 {
		c.Points = 0
	}
	if c.HP < 0 {
		c.HP = 0
	}
	if c.Playtime < 0 {
		c.Playtime = 0
	}
	if c.Deaths < 0 {
		c.Deaths = 0
	}
	if c.Biome == "" {
		c.Biome = "forest"
	}
	if c.Kills == nil {
		c.Kills = map[string]int{}
	}
	for k, n := range c.Kills {
		if n <= 0 || k == "" {
			delete(c.Kills, k)
		}
	}
	if c.Worn == nil {
		c.Worn = map[string]Slot{}
	}
	for slot, s := range c.Worn {
		if s.Empty() {
			delete(c.Worn, slot)
		}
	}
	for i, s := range c.Bag {
		if s.Empty() {
			c.Bag[i] = Slot{}
		}
	}
}

// Empty — пуста ли ячейка.
func (s Slot) Empty() bool { return s.N <= 0 || s.ID == "" }

// Touch отмечает время записи (и время создания, если его ещё не было).
func (c *Char) Touch(now time.Time) {
	stamp := now.UTC().Format(time.RFC3339)
	if c.Created == "" {
		c.Created = stamp
	}
	c.Updated = stamp
}

// Kill засчитывает убитого: key строят KillAnimal/KillEnemy.
func (c *Char) Kill(key string) {
	if key == "" {
		return
	}
	if c.Kills == nil {
		c.Kills = map[string]int{}
	}
	c.Kills[key]++
}

// KillTotal — сколько всего существ убито.
func (c *Char) KillTotal() int {
	n := 0
	for _, k := range c.Kills {
		n += k
	}
	return n
}

// Ключи счётчика убийств. Вид записан вместе с родом («зверь» или «враг»),
// потому что имена у них независимые: rat есть и там и там.
const (
	KindAnimal = "animal"
	KindEnemy  = "enemy"
)

// KillAnimal — ключ счётчика для зверя (species.json).
func KillAnimal(species string) string { return KindAnimal + ":" + species }

// KillEnemy — ключ счётчика для врага: тип и тир (enemies.json). Тир входит в
// ключ, потому что это разные существа с разными именами: гоблин и вожак
// гоблинов считаются порознь.
func KillEnemy(typeID, tier string) string { return KindEnemy + ":" + typeID + "/" + tier }

// SplitKill разбирает ключ обратно: род ("animal"/"enemy"), вид и тир (у зверя
// пустой). ok == false — ключ не наш (файл из будущей версии).
func SplitKill(key string) (kind, id, tier string, ok bool) {
	k, rest, found := strings.Cut(key, ":")
	if !found || rest == "" {
		return "", "", "", false
	}
	switch k {
	case KindAnimal:
		return k, rest, "", true
	case KindEnemy:
		id, tier, found = strings.Cut(rest, "/")
		if !found || id == "" || tier == "" {
			return "", "", "", false
		}
		return k, id, tier, true
	}
	return "", "", "", false
}

// CleanName приводит имя к тому, что игра умеет показать: обрезает пробелы по
// краям, схлопывает внутренние и режет по длине. Пустой результат — имя не
// годится (одни пробелы).
func CleanName(s string) string {
	name := strings.Join(strings.Fields(s), " ")
	if rs := []rune(name); len(rs) > NameLimit {
		name = strings.TrimSpace(string(rs[:NameLimit]))
	}
	return name
}
