package settings

import (
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
)

// Action — игровое действие, которому назначена клавиша. Список закрытый:
// сцена спрашивает клавишу по действию и не знает, какая она сейчас.
type Action int

const (
	ActUp Action = iota
	ActDown
	ActLeft
	ActRight
	ActRun
	ActAttack
	ActMap
	ActUse
	ActBag
	ActCount
)

// Идентификаторы для файла настроек и подписи для экрана. Порядок — как у
// констант: это одна и та же таблица, разложенная по трём массивам.
var (
	actionID = [ActCount]string{"up", "down", "left", "right", "run", "attack", "map", "use", "bag"}

	actionTitle = [ActCount]string{"ВВЕРХ", "ВНИЗ", "ВЛЕВО", "ВПРАВО", "БЕГ", "УДАР", "КАРТА", "ОТКРЫТЬ", "СУМКА"}

	// Клавиши по умолчанию: ходьба на WASD, бег на пробеле (удобнее, чем
	// шифт, когда вторая рука на мыши), удар дублирует ЛКМ.
	actionKey = [ActCount]ebiten.Key{
		ebiten.KeyW, ebiten.KeyS, ebiten.KeyA, ebiten.KeyD,
		ebiten.KeySpace, ebiten.KeyF, ebiten.KeyTab, ebiten.KeyE, ebiten.KeyI,
	}
)

// Actions — все действия по порядку (для перебора в интерфейсе).
func Actions() []Action {
	out := make([]Action, 0, ActCount)
	for a := Action(0); a < ActCount; a++ {
		out = append(out, a)
	}
	return out
}

// Title — подпись действия.
func (a Action) Title() string {
	if a < 0 || a >= ActCount {
		return "?"
	}
	return actionTitle[a]
}

// Key — клавиша действия.
func Key(a Action) ebiten.Key {
	if a < 0 || a >= ActCount {
		return ebiten.KeyMax
	}
	if k, ok := current.Keys[actionID[a]]; ok {
		return k
	}
	return actionKey[a]
}

// Owner — какое действие уже занимает клавишу k.
func Owner(k ebiten.Key) (Action, bool) {
	for a := Action(0); a < ActCount; a++ {
		if Key(a) == k {
			return a, true
		}
	}
	return 0, false
}

// Bind назначает действию клавишу. Если она занята другим действием, действия
// меняются клавишами: так привязка никогда не оставляет действие без клавиши и
// не создаёт молчаливых дублей.
func Bind(a Action, k ebiten.Key) {
	if a < 0 || a >= ActCount {
		return
	}
	if current.Keys == nil {
		current.Keys = map[string]ebiten.Key{}
	}
	if other, ok := Owner(k); ok && other != a {
		current.Keys[actionID[other]] = Key(a)
	}
	current.Keys[actionID[a]] = k
	save()
}

// ResetKeys возвращает все привязки к умолчанию.
func ResetKeys() {
	current.Keys = map[string]ebiten.Key{}
	save()
}

// KeyLabel — как показать клавишу игроку. Ebiten отдаёт латинские имена
// ("Space", "ArrowUp"); привычные подписываем по-русски, остальное — как есть
// заглавными.
func KeyLabel(k ebiten.Key) string {
	switch k {
	case ebiten.KeySpace:
		return "ПРОБЕЛ"
	case ebiten.KeyTab:
		return "ТАБ"
	case ebiten.KeyEnter, ebiten.KeyNumpadEnter:
		return "ВВОД"
	case ebiten.KeyBackspace:
		return "СТИРАНИЕ"
	case ebiten.KeyArrowUp:
		return "↑"
	case ebiten.KeyArrowDown:
		return "↓"
	case ebiten.KeyArrowLeft:
		return "←"
	case ebiten.KeyArrowRight:
		return "→"
	case ebiten.KeyShiftLeft, ebiten.KeyShift:
		return "SHIFT"
	case ebiten.KeyShiftRight:
		return "SHIFT ПР"
	case ebiten.KeyControlLeft, ebiten.KeyControl:
		return "CTRL"
	case ebiten.KeyControlRight:
		return "CTRL ПР"
	case ebiten.KeyAltLeft, ebiten.KeyAlt:
		return "ALT"
	case ebiten.KeyAltRight:
		return "ALT ПР"
	}
	return strings.ToUpper(k.String())
}

// knownAction — есть ли такое действие в коде (файл настроек мог остаться от
// версии, где действия были другие).
func knownAction(id string) bool {
	for _, a := range actionID {
		if a == id {
			return true
		}
	}
	return false
}
