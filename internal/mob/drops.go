package mob

// Розыгрыш таблицы выпадения. Правило одно на всех — зверей, врагов и боссов,
// потому что таблица у них одного формата (см. Drop).

import (
	"fmt"
	"math/rand/v2"
)

// Loot — сколько чего выпало с одного трупа. Строки с одинаковым id уже
// сложены: игроку падает «угли ×3», а не три раза по одному.
type Loot struct {
	ID string
	N  int
}

// RollDrops катает таблицу drops.
//
// Строка с chance >= 1 — обязательная добыча: выпадает всегда и ровно один раз,
// сколько бы ни было бросков. Тир не должен раздувать стопку мяса: смысл
// гарантии — «с пустыми руками от трупа не уходят», а не «добычи много».
//
// Остальные строки редкие: каждая бросается rolls раз, и тир меняет не размер
// стопки, а вероятность увидеть редкость. bonus прибавляется к их шансу —
// столько добавляет усиленная особь.
//
// Если не выпало вообще ничего, отдаётся первая строка таблицы в минимальном
// количестве. Гарантия держится кодом, а не только данными: строку с chance = 1
// в таблице легко забыть, а моб без добычи выглядит как поломка игры.
func RollDrops(drops []Drop, rolls int, bonus float64, rng *rand.Rand) []Loot {
	if len(drops) == 0 {
		return nil
	}
	if rolls < 1 {
		rolls = 1
	}
	var out []Loot
	add := func(id string, n int) {
		if n <= 0 {
			return
		}
		for i := range out {
			if out[i].ID == id {
				out[i].N += n
				return
			}
		}
		out = append(out, Loot{ID: id, N: n})
	}
	for _, d := range drops {
		if d.ID == "" {
			continue
		}
		if d.Chance >= 1 {
			add(d.ID, rollAmount(d, rng))
			continue
		}
		for range rolls {
			if rng.Float64() < d.Chance+bonus {
				add(d.ID, rollAmount(d, rng))
			}
		}
	}
	if len(out) == 0 {
		add(drops[0].ID, max(drops[0].Min, 1))
	}
	return out
}

// dropProblems — проверки таблицы выпадения, одинаковые у зверей и врагов:
// строки осмысленны и обязательная среди них есть. Пустой список — всё сошлось.
//
// Отсутствие гарантированной строки — именно ошибка данных, а не мелочь: без
// неё игрок узнаёт о выпадении по частоте, а не по факту, и убийство перестаёт
// что-либо давать. Код такую таблицу переживёт (см. RollDrops), но чинить её
// надо в данных.
func dropProblems(drops []Drop) []string {
	var probs []string
	if len(drops) == 0 {
		return []string{"пустая таблица добычи"}
	}
	sure := false
	for _, d := range drops {
		if d.ID == "" || d.Min < 1 || d.Min > d.Max || d.Chance <= 0 || d.Chance > 1 {
			probs = append(probs, fmt.Sprintf("кривая строка дропа %q (%d..%d, шанс %.2f)",
				d.ID, d.Min, d.Max, d.Chance))
		}
		if d.Chance >= 1 {
			sure = true
		}
	}
	if !sure {
		probs = append(probs, "нет обязательной строки добычи (chance 1.0)")
	}
	return probs
}

// rollAmount — сколько штук выпало по строке.
func rollAmount(d Drop, rng *rand.Rand) int {
	lo, hi := max(d.Min, 1), d.Max
	if hi <= lo {
		return lo
	}
	return lo + rng.IntN(hi-lo+1)
}
