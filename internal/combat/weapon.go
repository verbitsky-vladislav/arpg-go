package combat

import (
	"fmt"
	"math/rand/v2"
	"strings"
)

// Форма удара: достаётся одному или всем разом.
//
// Одиночное оружие бьёт ближайшую цель в секторе — палкой нельзя задеть троих
// одним взмахом. Площадное бьёт всё в круге радиуса Radius вокруг героя, и
// сектор для него не считается: молот, обрушенный оземь, не выбирает сторону.
const (
	ShapeSingle = "single"
	ShapeArea   = "area"
)

// Shapes — все известные формы удара.
var Shapes = map[string]bool{ShapeSingle: true, ShapeArea: true}

// Состояния, которые удар накладывает на цель. Название говорит, что именно
// накладывается, а Effect.Power — насколько сильно:
//
//	bleed   кровотечение: Power физического урона в секунду
//	poison  отравление:   то же, но ядом
//	burn    поджог:       Power урона огнём в секунду
//	chill   охлаждение:   Power — на сколько процентов цель медленнее
//	shock   шок:          Power — на сколько процентов цель уязвимее
//	stun    оглушение:    цель не действует; Power не нужен
const (
	Bleed  = "bleed"
	Poison = "poison"
	Burn   = "burn"
	Chill  = "chill"
	Shock  = "shock"
	Stun   = "stun"
)

// EffectKinds — все известные состояния.
var EffectKinds = map[string]bool{
	Bleed: true, Poison: true, Burn: true, Chill: true, Shock: true, Stun: true,
}

// Effect — состояние, которое оружие накладывает при попадании.
type Effect struct {
	Kind   string  `json:"kind"`
	Chance float64 `json:"chance"` // 0..1; 1 — накладывается всегда
	Ticks  int     `json:"ticks"`  // сколько держится (60 тиков = 1 с)
	Power  int     `json:"power"`  // сила: смысл зависит от вида, см. выше
}

// Problems — что не так с состоянием (пусто — всё в порядке).
func (e Effect) Problems(who string) []string {
	var out []string
	if !EffectKinds[e.Kind] {
		out = append(out, fmt.Sprintf("%s: неизвестное состояние %q", who, e.Kind))
	}
	if e.Chance <= 0 || e.Chance > 1 {
		out = append(out, fmt.Sprintf("%s: chance=%.2f вне (0..1]", who, e.Chance))
	}
	if e.Ticks <= 0 && e.Kind != "" {
		out = append(out, fmt.Sprintf("%s: ticks=%d — состояние нулевой длины", who, e.Ticks))
	}
	if e.Power < 0 {
		out = append(out, fmt.Sprintf("%s: power=%d отрицателен", who, e.Power))
	}
	return out
}

// Land — какие из состояний наложились этим ударом. Бросок один на состояние:
// оружие с кровотечением и оглушением может наложить оба разом, и это не
// ошибка, а причина такое оружие искать.
func Land(fx []Effect, rng *rand.Rand) []Effect {
	var out []Effect
	for _, e := range fx {
		if e.Chance <= 0 {
			continue
		}
		r := rand.Float64()
		if rng != nil {
			r = rng.Float64()
		}
		if r < e.Chance {
			out = append(out, e)
		}
	}
	return out
}

// Weapon — боевые свойства вещи: всё, чем удар одним оружием отличается от
// удара другим.
//
// Здесь нет ни размаха, ни угла сектора: их задаёт лоадаут персонажа, потому
// что это геометрия анимации — насколько широко герой машет руками. Оружие
// приносит числа: сколько снимает, как быстро и кого задевает.
type Weapon struct {
	// Speed — во сколько раз быстрее лоадаутного замах и перезарядка (1 —
	// как нарисовано, 2 — вдвое чаще, 0.5 — вдвое реже).
	Speed  float64 `json:"attack_speed"`
	Shape  string  `json:"shape"`
	Radius float64 `json:"radius,omitempty"` // радиус поражения для shape=area, px
	// Damage — урон по видам; складывается с базовым уроном персонажа.
	Damage  Rolls    `json:"damage"`
	Effects []Effect `json:"effects,omitempty"`
}

// Rate — множитель скорости удара (не заданный считается единицей).
func (w *Weapon) Rate() float64 {
	if w == nil || w.Speed <= 0 {
		return 1
	}
	return w.Speed
}

// Area — бьёт ли оружие по области.
func (w *Weapon) Area() bool { return w != nil && w.Shape == ShapeArea }

// Note — строка для интерфейса: «2-4 ФИЗ, ×1.2» (пусто, если показывать
// нечего). Скорость показывается только когда она не единичная: «×1.0» у
// каждой второй вещи — шум.
func (w *Weapon) Note() string {
	if w == nil {
		return ""
	}
	out := []string{}
	if s := w.Damage.String(); s != "" {
		out = append(out, s)
	}
	if w.Rate() != 1 {
		out = append(out, fmt.Sprintf("×%.2g", w.Rate()))
	}
	if w.Area() {
		out = append(out, fmt.Sprintf("ПО ОБЛАСТИ %.0f", w.Radius))
	}
	for _, e := range w.Effects {
		out = append(out, fmt.Sprintf("%s %d%%", strings.ToUpper(e.Kind), int(e.Chance*100)))
	}
	return strings.Join(out, ", ")
}

// Problems — что не так с описанием оружия (пусто — всё в порядке).
func (w *Weapon) Problems(who string) []string {
	if w == nil {
		return nil
	}
	out := w.Damage.Problems(who + ".damage")
	if w.Speed <= 0 {
		out = append(out, fmt.Sprintf("%s: attack_speed=%.2f — скорость должна быть больше нуля", who, w.Speed))
	}
	if !Shapes[w.Shape] {
		out = append(out, fmt.Sprintf("%s: неизвестная форма удара %q", who, w.Shape))
	}
	switch {
	case w.Area() && w.Radius <= 0:
		out = append(out, fmt.Sprintf("%s: shape=area без радиуса поражения", who))
	case !w.Area() && w.Radius != 0:
		out = append(out, fmt.Sprintf("%s: radius=%.0f при shape=%q — радиус есть только у области", who, w.Radius, w.Shape))
	}
	for i, e := range w.Effects {
		out = append(out, e.Problems(fmt.Sprintf("%s.effects[%d]", who, i))...)
	}
	return out
}
