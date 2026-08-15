package save

// Мир персонажа на диске.
//
// Мир сохраняется целиком, а не выводится из сида. Соблазн держать в файле одно
// число велик — генератор детерминирован, и тот же сид сегодня даёт ту же
// карту. Но только сегодня: первая же правка worldgen (а он правится постоянно)
// сделает из того же числа другую карту, и сохранённый герой очнётся в чужом
// мире — с сундуком посреди озера и точкой выхода в скале. Сид остаётся в
// записи как память о происхождении мира, а не как способ его вернуть.
//
// Хранится мир в двух местах, и это не небрежность, а разница в природе данных:
//
//	карта      — тяжёлая (сотни килобайт) и неизменная: пишется один раз, при
//	             создании персонажа, отдельным сжатым файлом worlds/<слот>.map.json.gz
//	население  — лёгкое и меняется каждый тик: живёт в самой записи персонажа
//	             (Beasts/Foes/Ground) и переписывается при каждом сохранении
//
// Иначе каждое автосохранение пережимало бы мегабайт неизменных тайлов посреди
// боя.

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/vladislav/game/internal/worldgen"
)

// Beast — зверь на карте в момент выхода.
type Beast struct {
	Species string     `json:"species"`
	Pos     [2]float64 `json:"pos"`
	Floor   uint8      `json:"floor"`
	HP      int        `json:"hp"`
}

// Foe — враг на карте в момент выхода. Тир и усиление сохраняются: от них
// зависят и числа особи, и её имя над полоской здоровья.
type Foe struct {
	Type  string     `json:"type"`
	Tier  string     `json:"tier"`
	Pos   [2]float64 `json:"pos"`
	Floor uint8      `json:"floor"`
	HP    int        `json:"hp"`
	Elite bool       `json:"elite,omitempty"`
}

// Drop — вещь, лежащая на земле и не подобранная.
type Drop struct {
	ID    string     `json:"id"`
	N     int        `json:"n"`
	Pos   [2]float64 `json:"pos"`
	Floor uint8      `json:"floor"`
	// Skill — чья сила заперта в камне, если это камень умения. Сила не
	// сохраняется числами: они выводятся из таблицы врагов, а от неё зависит
	// баланс — сохранённые числа разошлись бы с ней при первой же правке.
	Skill *Skill `json:"skill,omitempty"`
}

// Skill — украденная сила: в ячейке героя или в камне на земле. Хранятся имена
// того, с кого она снята, и остаток зарядов; всё остальное восстанавливается из
// enemies.json.
type Skill struct {
	Type string `json:"type"`
	Tier string `json:"tier"`
	Left int    `json:"left"`
}

// mapPath — файл карты слота. Карты лежат рядом с книгой, в своём каталоге:
// их правят не руками, и мешаться с чтением chars.json им незачем.
func (s *Store) mapPath(slot int) string {
	return filepath.Join(filepath.Dir(s.path), "worlds", fmt.Sprintf("map-%d.json.gz", slot+1))
}

// MapPath — где лежит карта слота (для сообщений и тестов).
func (s *Store) MapPath(slot int) string { return s.mapPath(slot) }

// LoadMap читает карту слота. ok == false — карты нет или она испорчена; тогда
// мир собирают заново из сида (хуже, чем вернуть свой, но лучше, чем ничего).
func (s *Store) LoadMap(slot int) (*worldgen.MapV1, bool) {
	if slot < 0 || slot >= Slots {
		return nil, false
	}
	f, err := os.Open(s.mapPath(slot))
	if err != nil {
		return nil, false
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, false
	}
	defer func() { _ = zr.Close() }()

	var mv worldgen.MapV1
	if json.NewDecoder(zr).Decode(&mv) != nil {
		return nil, false
	}
	if mv.Width <= 0 || mv.Height <= 0 || mv.TileSize <= 0 {
		return nil, false // недописанный файл: пустую карту играть нельзя
	}
	return &mv, true
}

// SaveMap пишет карту слота — один раз, при создании персонажа. Запись
// атомарная, как и у книги: недописанная карта не должна пережить вылет.
func (s *Store) SaveMap(slot int, mv *worldgen.MapV1) error {
	if slot < 0 || slot >= Slots {
		return fmt.Errorf("save: слот %d вне книги", slot)
	}
	if mv == nil {
		return fmt.Errorf("save: пустая карта")
	}
	p := s.mapPath(slot)
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("save: каталог %q: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".map-*.json.gz")
	if err != nil {
		return fmt.Errorf("save: временный файл в %q: %w", dir, err)
	}
	name := tmp.Name()
	fail := func(err error) error {
		tmp.Close()
		_ = os.Remove(name)
		return err
	}
	zw := gzip.NewWriter(tmp)
	if err := json.NewEncoder(zw).Encode(mv); err != nil {
		return fail(fmt.Errorf("save: запись карты %q: %w", name, err))
	}
	if err := zw.Close(); err != nil {
		return fail(fmt.Errorf("save: сжатие карты %q: %w", name, err))
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("save: закрытие %q: %w", name, err)
	}
	if err := os.Rename(name, p); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("save: переименование в %q: %w", p, err)
	}
	return nil
}

// DeleteMap стирает карту слота: удалённый персонаж уносит свой мир с собой.
func (s *Store) DeleteMap(slot int) {
	if slot < 0 || slot >= Slots {
		return
	}
	_ = os.Remove(s.mapPath(slot))
}
