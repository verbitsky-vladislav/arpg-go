package scene

import (
	"os"
	"strings"
	"testing"

	"github.com/vladislav/game/internal/assets"
)

// TestBestiarySections — каждый раздел книги собирается из данных: карточки
// есть, спрайты грузятся, характеристики непустые.
//
// Проверка нужна потому, что бестиарий устроен «мягко»: раздел без данных не
// роняет сцену, а показывает пустую страницу с пояснением. В игре это выглядит
// как «раздел ещё не сделали», и молча пережить потерю таблицы очень легко.
func TestBestiarySections(t *testing.T) {
	l := assets.NewLoader(os.DirFS("../../assets"))
	b := NewBestiary(l, nil)

	want := map[string]int{ // раздел → сколько карточек ждём минимум
		"ПЕРСОНАЖИ": 4,  // 2 тела × 2 лоадаута
		"ЖИВОТНЫЕ":  30, // виды из species.json
		"МОБЫ":      60, // 18 типов, у слизня 9 цветов вместо тиров
		"БОССЫ":     3,  // slime_boss t1..t3
	}

	for _, sec := range b.secs {
		sec.ensure()
		if sec.note != "" {
			t.Errorf("%s: раздел пуст (%s)", sec.title, sec.note)
			continue
		}
		if n := len(sec.entries); n < want[sec.title] {
			t.Errorf("%s: карточек %d, ожидалось хотя бы %d", sec.title, n, want[sec.title])
		}
		for _, e := range sec.entries {
			switch {
			case e.err != "":
				t.Errorf("%s / %s: %s", sec.title, e.title, e.err)
			case e.pack == nil || e.play == nil || e.play.Clip() == nil:
				t.Errorf("%s / %s: нечего показывать в рамке", sec.title, e.title)
			case e.title == "" || e.sub == "":
				t.Errorf("%s: карточка без подписи (%q / %q)", sec.title, e.title, e.sub)
			case len(e.facts) < 3:
				t.Errorf("%s / %s: характеристик всего %d", sec.title, e.title, len(e.facts))
			}
		}
	}
}

// latin — есть ли в слове латиница.
func latin(s string) bool {
	return strings.ContainsAny(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
}

// roman — римская цифра: "ПОДЗЕМЕЛЬЕ II" это перевод, а не забытый ключ.
func roman(s string) bool { return s != "" && strings.Trim(s, "IVXL") == "" }

// TestBestiaryTranslations — в статьях врагов не остаётся непереведённых
// ключей данных. Словарь молчалив по устройству (word отдаёт ключ как есть),
// поэтому новое значение в таблице иначе просто вылезет латиницей на странице.
func TestBestiaryTranslations(t *testing.T) {
	l := assets.NewLoader(os.DirFS("../../assets"))
	b := NewBestiary(l, nil)

	for _, sec := range b.secs {
		sec.ensure()
		for _, e := range sec.entries {
			for _, f := range e.facts {
				for _, tok := range strings.FieldsFunc(f, func(r rune) bool { return r == ' ' || r == ',' }) {
					if latin(tok) && !roman(tok) {
						t.Errorf("%s / %s: непереведённый ключ %q в строке %q",
							sec.title, e.title, tok, f)
					}
				}
			}
		}
	}
}
