package scene

import (
	"math"

	"github.com/vladislav/game/internal/audio"
	"github.com/vladislav/game/internal/character"
	"github.com/vladislav/game/internal/engine"
)

// Звук забега: банк эффектов плюс то, что должна решать именно сцена —
// когда шаг, чем ударили и по чему попали.
//
// Сам internal/audio про бой не знает ничего, и это намеренно: там дистанция,
// панорама и потолок голосов, здесь — смысл событий.

// Звук интерфейса берётся прямо из общего банка, а не через сцену: меню,
// настройки и бестиарий существуют без забега, героя и мира, и заводить им
// собственный звуковой объект незачем.

// uiMove — перебор пунктов. Звучит только на реальной смене выбора: мышь
// водит по пунктам непрерывно, и щелчок на каждый кадр наведения превратил бы
// меню в трещотку.
func uiMove(prev, now int) {
	if prev != now {
		audio.Shared().Play(audio.UIMove)
	}
}

// uiConfirm — выбор принят: вход в пункт, переключение настройки.
func uiConfirm() { audio.Shared().Play(audio.UIConfirm) }

// uiCancel — выход назад по ESC.
func uiCancel() { audio.Shared().Play(audio.UICancel) }

// stride — сколько пикселей герой проходит между шагами. Шаги считаются по
// пройденному пути, а не по кадрам анимации: клипы ходьбы у лоадаутов разной
// длины, а бег от шага отличается ровно тем, что путь набегает быстрее — то
// есть скорость шагов подстраивается сама.
const stride = 26

// gameSound — звуковая часть забега. Указатель может быть nil (звукового
// устройства нет, тесты сцены), поэтому все методы терпят nil-получателя:
// молчащая игра лучше упавшей.
type gameSound struct {
	audio.Sink

	prev   engine.Vec2 // где герой стоял в прошлом кадре
	walked float64     // путь, набежавший с прошлого шага
	inited bool
}

// newGameSound берёт общий банк процесса (его собирает точка входа). Свой банк
// на забег означал бы вторую копию всех сэмплов в памяти и вторую очередь
// голосов — при том, что устройство одно.
func newGameSound() *gameSound {
	return &gameSound{Sink: audio.Shared()}
}

func (s *gameSound) play(id string) {
	if s != nil {
		s.Sink.Play(id)
	}
}

func (s *gameSound) at(id string, pos engine.Vec2) {
	if s != nil {
		s.Sink.PlayAt(id, pos)
	}
}

// tick вызывается раз в кадр: переносит точку прослушивания за героем и
// отсчитывает шаги. Время самого банка двигает корень движка (audio.Tick) —
// голоса надо освобождать и тогда, когда забега на экране нет.
func (s *gameSound) tick(g *Game) {
	if s == nil {
		return
	}
	s.SetListener(g.pl.Pos)
	// Музыка задаётся каждый кадр, а не один раз при старте: повторный вызов
	// с тем же треком ничего не делает, зато возврат из меню, сумки или
	// сундука сам включает биом обратно, и хранить это состояние в сцене не
	// нужно.
	s.Music(g.m.Biome())
	s.steps(g)
}

// steps отсчитывает шаги по пройденному пути.
func (s *gameSound) steps(g *Game) {
	pos := g.pl.Pos
	if !s.inited {
		s.prev, s.inited = pos, true
		return
	}
	d := pos.Sub(s.prev)
	s.prev = pos

	if st := g.pl.State(); st != character.Walk && st != character.Run {
		// Остановился — следующий шаг звучит сразу, а не через остаток пути:
		// иначе первый шаг после паузы проглатывается.
		s.walked = stride
		return
	}
	s.walked += math.Hypot(d.X, d.Y)
	if s.walked < stride {
		return
	}
	s.walked = 0
	// Шаг звучит из-под ног, а не из центра героя: слушатель стоит там же,
	// так что это ровно центр панорамы и полная громкость.
	s.at(stepSound(g), pos)
}

// stepSound выбирает звук по тому, на чём герой стоит.
func stepSound(g *Game) string {
	switch g.m.Zone(g.pl.Pos) {
	case "water", "shore":
		return audio.StepWater
	case "plateau", "trail":
		// Плато — камень, тропа — утоптанная земля: обе поверхности твёрдые и
		// звучат суше травы. Отдельный звук для тропы завели бы, если бы она
		// была не куском луга, а местом, где игрок проводит время.
		return audio.StepStone
	default:
		return audio.StepGrass
	}
}

// swingSound — звук замаха по тому, чем герой машет. Площадное оружие
// тяжелее: у него шире сектор и медленнее темп, значит и воздух оно рвёт
// иначе.
func (g *Game) swingSound() string {
	if w := g.pl.Power(); w.Area() {
		return audio.SwingHeavy
	}
	return audio.SwingLight
}

// hitSound — звук попадания. Зависит не от цели, а от того, чем бьют: брони,
// по которой можно было бы различать цели, в данных нет, а вот разница между
// кулаком, дубиной и клинком слышна сразу.
func (g *Game) hitSound() string {
	w := g.pl.Power()
	switch {
	case !g.pl.Armed():
		return audio.HitFlesh // безоружный бьёт телом
	case w.Area():
		return audio.HitBlunt
	default:
		return audio.HitMetal
	}
}
