// Package mob — животные и прочие обитатели мира: описательные данные о видах
// и (дальше) общий автомат поведения.
//
// Данные вида лежат в assets/mobs/animals/species.json и описывают смысл —
// дикое/домашнее, нападает ли, где водится, во что вырастает. Графика в них не
// упоминается: кадры и анимации грузит sprite.Pack по manifest.json. Формат
// таблицы: docs/mobs/species.md.
package mob

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
)

// Species — описание одного вида. Набор полей одинаков у всех видов; пустые
// значения задаются явно (null, [], 0), умолчаний загрузчик не подставляет.
type Species struct {
	Art   string `json:"art"` // каталог со спрайтами (== ключ записи)
	Title struct {
		RU string `json:"ru"`
		EN string `json:"en"`
	} `json:"title"`
	Family    string `json:"family"`
	SizeClass string `json:"size_class"`

	Domestication string `json:"domestication"` // wild | domestic
	Stage         string `json:"stage"`         // adult | young
	GrowsInto     string `json:"grows_into"`    // "" — не вырастает
	YoungForm     string `json:"young_form"`    // "" — детёныша нет

	Temper     string `json:"temper"`   // passive|skittish|territorial|predator|tame
	Activity   string `json:"activity"` // day|night|any
	Locomotion struct {
		Ground bool `json:"ground"`
		Water  bool `json:"water"`
		Air    bool `json:"air"`
	} `json:"locomotion"`
	Threat struct {
		Attacks    bool     `json:"attacks"`
		Unprovoked bool     `json:"unprovoked"`
		Damage     int      `json:"damage"`
		Sight      float64  `json:"sight"`
		FleeHP     float64  `json:"flee_hp"`
		Prey       []string `json:"prey"`
	} `json:"threat"`
	Stats struct {
		HP    int `json:"hp"`
		Speed struct {
			Walk float64 `json:"walk"`
			Run  float64 `json:"run"`
			Swim float64 `json:"swim"`
		} `json:"speed"`
	} `json:"stats"`
	Habitat struct {
		Biomes          map[string]int `json:"biomes"` // биом → вес спавна
		Zones           []string       `json:"zones"`
		Avoid           []string       `json:"avoid"`
		NeedsSettlement bool           `json:"needs_settlement"`
	} `json:"habitat"`
	Spawn struct {
		Group struct {
			Min int `json:"min"`
			Max int `json:"max"`
		} `json:"group"`
		With string `json:"with"` // спавнится рядом с этим видом
	} `json:"spawn"`
	Use struct {
		Tameable bool     `json:"tameable"`
		Rideable bool     `json:"rideable"`
		Products []string `json:"products"`
	} `json:"use"`
	Drops []Drop `json:"drops"`
	Voice string `json:"voice"`
	Notes string `json:"notes"`

	ID string `json:"-"` // ключ записи, проставляется загрузчиком
}

// Drop — строка таблицы выпадения.
type Drop struct {
	ID     string  `json:"id"`
	Min    int     `json:"min"`
	Max    int     `json:"max"`
	Chance float64 `json:"chance"`
}

// Wild — дикий ли вид (в противовес домашнему).
func (s *Species) Wild() bool { return s.Domestication == "wild" }

// Young — детёныш ли это (у детёныша обычно есть GrowsInto).
func (s *Species) Young() bool { return s.Stage == "young" }

// Catalog — таблица видов целиком.
type Catalog struct {
	Version int                 `json:"version"`
	Vocab   map[string]any      `json:"vocab"`
	Species map[string]*Species `json:"species"`
}

// LoadSpecies читает таблицу видов из fsys по пути p
// (обычно "mobs/animals/species.json").
func LoadSpecies(fsys fs.FS, p string) (*Catalog, error) {
	b, err := fs.ReadFile(fsys, p)
	if err != nil {
		return nil, fmt.Errorf("mob: чтение %q: %w", p, err)
	}
	var c Catalog
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("mob: разбор %q: %w", p, err)
	}
	for id, s := range c.Species {
		s.ID = id
	}
	return &c, nil
}

// IDs — идентификаторы видов в стабильном (алфавитном) порядке.
func (c *Catalog) IDs() []string {
	ids := make([]string, 0, len(c.Species))
	for id := range c.Species {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Get — вид по идентификатору (nil, если нет).
func (c *Catalog) Get(id string) *Species { return c.Species[id] }

// Validate проверяет внутреннюю связность таблицы и возвращает список проблем
// (пустой — всё сошлось). Графику не трогает: соответствие анимаций намерениям
// вида проверяет отдельная сверка с sprite.Pack.
func (c *Catalog) Validate() []string {
	var probs []string
	ref := func(who, field, id string) {
		if id != "" && c.Species[id] == nil {
			probs = append(probs, fmt.Sprintf("%s.%s ссылается на неизвестный вид %q", who, field, id))
		}
	}
	for _, id := range c.IDs() {
		s := c.Species[id]
		if s.Art != id {
			probs = append(probs, fmt.Sprintf("%s: art=%q не совпадает с ключом", id, s.Art))
		}
		ref(id, "grows_into", s.GrowsInto)
		ref(id, "young_form", s.YoungForm)
		ref(id, "spawn.with", s.Spawn.With)
		for _, p := range s.Threat.Prey {
			ref(id, "threat.prey", p)
		}
		if s.Young() && s.GrowsInto == "" && s.Notes == "" {
			probs = append(probs, fmt.Sprintf("%s: детёныш без grows_into и без пояснения в notes", id))
		}
		if s.Spawn.Group.Min > s.Spawn.Group.Max {
			probs = append(probs, fmt.Sprintf("%s: spawn.group min > max", id))
		}
		if s.Stats.HP <= 0 {
			probs = append(probs, fmt.Sprintf("%s: hp <= 0", id))
		}
		if s.Stats.Speed.Walk <= 0 {
			probs = append(probs, fmt.Sprintf("%s: нулевая скорость шага", id))
		}
		// Урон и признак «нападает» — одно и то же утверждение, записанное
		// дважды; разошлись — значит правка задела только половину.
		if s.Threat.Attacks != (s.Threat.Damage > 0) {
			probs = append(probs, fmt.Sprintf("%s: attacks=%v при damage=%d",
				id, s.Threat.Attacks, s.Threat.Damage))
		}
		if len(s.Habitat.Biomes) == 0 {
			probs = append(probs, fmt.Sprintf("%s: не задан ни один биом", id))
		}
		for _, p := range dropProblems(s.Drops) {
			probs = append(probs, fmt.Sprintf("%s: %s", id, p))
		}
	}
	return probs
}
