// Package combat — из чего состоит удар: урон по видам, его броски и
// состояния, которые он накладывает.
//
// Пакет один на всех, кто бьёт: и у оружия в сумке, и у героя, и (со временем)
// у мобов урон описывается одними и теми же полями. Ни про предметы, ни про
// персонажа, ни про сцену он не знает — иначе описание удара разъехалось бы по
// двум таблицам, у оружия своё, у героя своё.
//
// Урон делится на виды с самого начала, хотя вид пока один — физический.
// Сложить всё в одно число дешевле, чем потом разложить обратно: сопротивления,
// уязвимости и цвет всплывающего числа считаются по видам, и удар, пришедший
// одним итогом, для них уже безнадёжен.
package combat

import (
	"fmt"
	"math/rand/v2"
	"strings"
)

// Виды урона. Физический — то, чем бьют палкой и клинком; остальные три —
// стихии, их пока никто не наносит, но описать их нужно там же, где физический,
// иначе первое же огненное оружие потребует переделки формата.
const (
	Physical  = "physical"
	Fire      = "fire"
	Cold      = "cold"
	Lightning = "lightning"
)

// Kinds — виды урона в том порядке, в котором их показывают.
var Kinds = []string{Physical, Fire, Cold, Lightning}

// Roll — диапазон целого числа [Min, Max]. Урон оружия всегда диапазон, даже
// когда границы совпали: «2-4» — это свойство вещи, а не случайность броска.
type Roll struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// Empty — пустой ли диапазон (урона этого вида у оружия нет).
func (r Roll) Empty() bool { return r.Min <= 0 && r.Max <= 0 }

// Value — бросок в диапазоне. rng == nil берёт общий источник случайности:
// урон разыгрывается на каждом ударе, и тащить свой генератор ради этого не
// обязан никто, кроме тестов.
func (r Roll) Value(rng *rand.Rand) int {
	// Границы сначала поднимаются до нуля: отрицательный урон — это опечатка в
	// таблице, и лечить её отрицательным броском (то есть лечением цели) нельзя.
	lo, hi := max(r.Min, 0), max(r.Max, 0)
	if hi <= lo {
		return lo
	}
	if rng == nil {
		return lo + rand.IntN(hi-lo+1)
	}
	return lo + rng.IntN(hi-lo+1)
}

// String — «2-4» или «3», если границы совпали.
func (r Roll) String() string {
	if r.Max <= r.Min {
		return fmt.Sprint(max(r.Min, 0))
	}
	return fmt.Sprintf("%d-%d", max(r.Min, 0), r.Max)
}

// Problems — что в диапазоне не так (пусто — всё в порядке). who попадает в
// текст сообщения: это может быть и вещь, и базовый урон персонажа.
func (r Roll) Problems(who string) []string {
	var out []string
	if r.Min < 0 {
		out = append(out, fmt.Sprintf("%s: min=%d отрицателен", who, r.Min))
	}
	if r.Max < r.Min {
		out = append(out, fmt.Sprintf("%s: max=%d меньше min=%d", who, r.Max, r.Min))
	}
	return out
}

// Rolls — урон до броска: свой диапазон на каждый вид. Ровно это и лежит в
// таблицах — у оружия в items.json, у героя в character.json.
type Rolls struct {
	Physical  Roll `json:"physical,omitzero"`
	Fire      Roll `json:"fire,omitzero"`
	Cold      Roll `json:"cold,omitzero"`
	Lightning Roll `json:"lightning,omitzero"`
}

// Of — диапазон вида kind (пустой у незнакомого вида).
func (r Rolls) Of(kind string) Roll {
	switch kind {
	case Physical:
		return r.Physical
	case Fire:
		return r.Fire
	case Cold:
		return r.Cold
	case Lightning:
		return r.Lightning
	}
	return Roll{}
}

// Empty — не наносит ли урона вовсе.
func (r Rolls) Empty() bool {
	return r.Physical.Empty() && r.Fire.Empty() && r.Cold.Empty() && r.Lightning.Empty()
}

// Add складывает диапазоны повидно: так базовый урон героя соединяется с уроном
// оружия. Складываются именно границы, а не броски, — «1-2» плюс «2-4» даёт
// «3-6», один бросок на удар, а не два.
func (r Rolls) Add(o Rolls) Rolls {
	add := func(a, b Roll) Roll {
		if a.Empty() {
			return b
		}
		if b.Empty() {
			return a
		}
		return Roll{Min: a.Min + b.Min, Max: max(a.Max, a.Min) + max(b.Max, b.Min)}
	}
	return Rolls{
		Physical:  add(r.Physical, o.Physical),
		Fire:      add(r.Fire, o.Fire),
		Cold:      add(r.Cold, o.Cold),
		Lightning: add(r.Lightning, o.Lightning),
	}
}

// Value разыгрывает урон одного попадания: по броску на каждый вид.
func (r Rolls) Value(rng *rand.Rand) Damage {
	return Damage{
		Physical:  r.Physical.Value(rng),
		Fire:      r.Fire.Value(rng),
		Cold:      r.Cold.Value(rng),
		Lightning: r.Lightning.Value(rng),
	}
}

// String — «2-4 ФИЗ» через запятую по всем непустым видам; пусто, если урона
// нет. Ради интерфейса: показывать оружие построчно негде, строка одна.
func (r Rolls) String() string {
	var out []string
	for _, k := range Kinds {
		if v := r.Of(k); !v.Empty() {
			out = append(out, v.String()+" "+Short(k))
		}
	}
	return strings.Join(out, ", ")
}

// Problems — что не так с диапазонами (пусто — всё в порядке).
func (r Rolls) Problems(who string) []string {
	var out []string
	for _, k := range Kinds {
		out = append(out, r.Of(k).Problems(who+"."+k)...)
	}
	return out
}

// Damage — урон одного состоявшегося попадания, разложенный по видам.
type Damage struct {
	Physical  int
	Fire      int
	Cold      int
	Lightning int
}

// Of — урон вида kind.
func (d Damage) Of(kind string) int {
	switch kind {
	case Physical:
		return d.Physical
	case Fire:
		return d.Fire
	case Cold:
		return d.Cold
	case Lightning:
		return d.Lightning
	}
	return 0
}

// Total — сколько здоровья снимет попадание. Сопротивлений пока нет, поэтому
// итог — простая сумма; когда они появятся, вычитать их будет тот, по кому
// бьют, — до этой суммы, а не после.
func (d Damage) Total() int { return d.Physical + d.Fire + d.Cold + d.Lightning }

// Add складывает два попадания повидно.
func (d Damage) Add(o Damage) Damage {
	return Damage{
		Physical:  d.Physical + o.Physical,
		Fire:      d.Fire + o.Fire,
		Cold:      d.Cold + o.Cold,
		Lightning: d.Lightning + o.Lightning,
	}
}

// Short — короткое имя вида урона для интерфейса.
func Short(kind string) string {
	switch kind {
	case Physical:
		return "ФИЗ"
	case Fire:
		return "ОГОНЬ"
	case Cold:
		return "ХОЛОД"
	case Lightning:
		return "МОЛНИЯ"
	}
	return strings.ToUpper(kind)
}
