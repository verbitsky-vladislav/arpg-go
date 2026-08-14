// Package audio — звуковой слой игры: банк эффектов и правила их
// проигрывания.
//
// Эффекты синтезированы (см. tools/sfxgen) и лежат в assets/audio/sfx рядом с
// манифестом sfx.json: id → файлы вариантов, минимальный интервал и потолок
// голосов. Пакет знает про манифест, дистанцию и громкость, но ничего не знает
// про бой — сцена сама говорит, что и где случилось.
//
// Главная задача здесь не «проиграть файл», а не превратить драку в кашу:
// пять мобов, ударивших в один кадр, без ограничений дают щелчок и перегруз,
// а не «мощно».
package audio

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"math"
	"path"

	eaudio "github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/audio/wav"

	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/engine"
)

// SampleRate — частота аудиоконтекста. Совпадает с частотой файлов sfxgen,
// поэтому ресемплинга при проигрывании не происходит.
const SampleRate = 44100

// Идентификаторы звуков. Константы, а не строки по месту: опечатка в id
// не роняет игру (звука просто не будет), и найти её потом тяжело.
const (
	SwingLight = "swing_light"
	SwingHeavy = "swing_heavy"
	HitFlesh   = "hit_flesh"
	HitMetal   = "hit_metal"
	HitBlunt   = "hit_blunt"
	EnemyDeath = "enemy_death"

	Hurt    = "hurt"
	LevelUp = "level_up"
	Pickup  = "pickup"

	StepGrass = "step_grass"
	StepStone = "step_stone"
	StepWood  = "step_wood"
	StepWater = "step_water"

	UIMove    = "ui_move"
	UIConfirm = "ui_confirm"
	UICancel  = "ui_cancel"
	UIDenied  = "ui_denied"
	UIEquip   = "ui_equip"
	ChestOpen = "chest_open"
)

// Дистанции слышимости в пикселях мира. Ближе near — полная громкость, дальше
// far — тишина. Отсчёт от ширины экрана: то, что видно, должно быть слышно.
const (
	nearDist = config.ScreenW / 4
	farDist  = config.ScreenW * 6 / 5
)

// Sink — то, что умеет звучать. Интерфейс нужен не ради абстракции, а ради
// тестов сцен: аудиоустройства в них нет, и любая сцена принимает Nop.
type Sink interface {
	// Play — звук вне мира: интерфейс, события забега.
	Play(id string)
	// PlayAt — звук из точки мира: громкость и панорама считаются от слушателя.
	PlayAt(id string, pos engine.Vec2)
	// SetListener — куда «поставлены уши»; обычно позиция игрока.
	SetListener(pos engine.Vec2)
	// Music переводит фон на трек id (обычно это биом); "" — уход в тишину.
	// Повторный вызов с тем же id ничего не делает.
	Music(id string)
	// Update — раз в кадр: двигает счётчик тиков и убирает отзвучавшие голоса.
	Update()
}

// Nop — глухой приёмник для тестов и для запуска без звукового устройства.
type Nop struct{}

func (Nop) Play(string)                {}
func (Nop) PlayAt(string, engine.Vec2) {}
func (Nop) SetListener(engine.Vec2)    {}
func (Nop) Music(string)               {}
func (Nop) Update()                    {}

// voice — один играющий экземпляр звука. За интерфейсом прячется ebiten,
// благодаря чему логика интервалов и голосов проверяется без устройства.
type voice interface {
	Play()
	IsPlaying() bool
	Close() error
}

// voiceFunc создаёт голос из PCM с готовой громкостью каналов.
type voiceFunc func(pcm []byte, left, right float64) voice

type sound struct {
	pcm       [][]byte // варианты, декодированные в стерео 16 бит
	cooldown  int      // минимальный интервал, тики
	maxVoices int
	jitter    float64

	last   int // тик последнего запуска
	prev   int // номер прошлого варианта: подряд один и тот же не берём
	active []voice
}

// Bank — банк звуков и всё состояние проигрывания.
type Bank struct {
	sounds   map[string]*sound
	mus      *music
	listener engine.Vec2
	tick     int
	master   float64
	sfx      float64
	musicVol float64
	rng      uint64
	mk       voiceFunc
}

type manifest struct {
	Sounds map[string]struct {
		Files    []string `json:"files"`
		Cooldown int      `json:"cooldown_ms"`
		Voices   int      `json:"voices"`
		Jitter   float64  `json:"jitter"`
	} `json:"sounds"`
}

// shared — банк процесса. Глобальный по той же причине, по какой глобален
// аудиоконтекст ebiten: звуковое устройство одно, а звук интерфейса нужен
// сценам, у которых нет ни мира, ни героя (меню, настройки, бестиарий).
// До Init — тишина, поэтому забыть инициализацию не страшно.
var shared Sink = Nop{}

// Init собирает общий банк. Ошибка возвращается для журнала, но игра обязана
// идти и молча: отсутствие звуковой карты — не повод не запускаться.
func Init(fsys fs.FS, dir string) error {
	b, err := New(fsys, dir)
	if err != nil {
		return err
	}
	shared = b
	return nil
}

// Shared — общий банк. Никогда не nil.
func Shared() Sink { return shared }

// Tick двигает время банка и убирает отзвучавшие голоса. Вызывается ровно из
// одного места — корня движка (internal/core), раз в кадр и независимо от
// сцены: иначе в меню, где никто не тикает, потолок голосов упёрся бы навсегда
// и звук пропал бы после первых же нажатий.
func Tick() { shared.Update() }

// SetVolume задаёт громкость общего банка в процентах (0..100).
func SetVolume(master, sfx, music int) {
	if b, ok := shared.(*Bank); ok {
		b.SetVolumes(master, sfx, music)
	}
}

// New читает манифест и декодирует все эффекты в память.
//
// Всё загружается сразу и целиком: набор занимает около мегабайта, а ленивая
// загрузка означала бы подгрузку файла в момент первого удара — то есть фриз
// ровно тогда, когда игра должна быть отзывчивой.
func New(fsys fs.FS, dir string) (*Bank, error) {
	bank, err := load(fsys, dir)
	if err != nil {
		return nil, err
	}
	// Контекст создаётся один на процесс; повторный вызов NewContext — паника.
	// Отделён от загрузки намеренно: банк собирается и проверяется без
	// звукового устройства, которого в тестах нет.
	if eaudio.CurrentContext() == nil {
		eaudio.NewContext(SampleRate)
	}
	return bank, nil
}

// load собирает банк из каталога ресурсов root: эффекты в root/sfx, музыка в
// root/music. Отделён от New тем, что не трогает аудиоустройство, — так весь
// разбор манифестов проверяется тестами.
func load(fsys fs.FS, root string) (*Bank, error) {
	dir := path.Join(root, "sfx")
	b, err := fs.ReadFile(fsys, path.Join(dir, "sfx.json"))
	if err != nil {
		return nil, fmt.Errorf("манифест звуков: %w", err)
	}
	var m manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("манифест звуков: %w", err)
	}

	bank := &Bank{
		sounds:   make(map[string]*sound, len(m.Sounds)),
		master:   0.7,
		sfx:      1,
		musicVol: 0.6,
		rng:      0x2545f4914f6cdd1d,
		mk:       ebitenVoice,
	}
	if bank.mus, err = loadMusic(fsys, path.Join(root, "music")); err != nil {
		return nil, err
	}
	for id, e := range m.Sounds {
		s := &sound{
			cooldown:  e.Cooldown * config.TPS / 1000,
			maxVoices: e.Voices,
			jitter:    e.Jitter,
			prev:      -1,
		}
		for _, f := range e.Files {
			pcm, err := decode(fsys, path.Join(dir, f))
			if err != nil {
				return nil, err
			}
			s.pcm = append(s.pcm, pcm)
		}
		if len(s.pcm) == 0 {
			continue // звук без файлов — не повод не запускаться
		}
		bank.sounds[id] = s
	}
	return bank, nil
}

// decode читает WAV в стерео 16 бит. Файлы моно (так их пишет sfxgen) ebiten
// разворачивает в два канала сам — панораму мы после этого считаем поверх.
func decode(fsys fs.FS, name string) ([]byte, error) {
	raw, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, fmt.Errorf("звук %s: %w", name, err)
	}
	st, err := wav.DecodeWithoutResampling(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("звук %s: %w", name, err)
	}
	pcm, err := io.ReadAll(st)
	if err != nil {
		return nil, fmt.Errorf("звук %s: %w", name, err)
	}
	return pcm, nil
}

// SetVolumes задаёт громкость в процентах (0..100), как её хранят настройки.
func (b *Bank) SetVolumes(master, sfx, music int) {
	b.master = clamp01(float64(master) / 100)
	b.sfx = clamp01(float64(sfx) / 100)
	b.musicVol = clamp01(float64(music) / 100)
}

// Music переводит фон на трек id; "" или неизвестный id — уход в тишину.
func (b *Bank) Music(id string) { b.mus.play(id) }

// SetListener переносит точку прослушивания.
func (b *Bank) SetListener(pos engine.Vec2) { b.listener = pos }

// Update вызывается раз в кадр из сцены.
func (b *Bank) Update() {
	b.tick++
	b.mus.update(b.master * b.musicVol)
	for _, s := range b.sounds {
		live := s.active[:0]
		for _, v := range s.active {
			if v.IsPlaying() {
				live = append(live, v)
				continue
			}
			_ = v.Close()
		}
		s.active = live
	}
}

// Play проигрывает звук по центру и на полной громкости.
func (b *Bank) Play(id string) { b.play(id, 1, 1) }

// PlayAt проигрывает звук из точки мира: дальше — тише, сбоку — в свой канал.
func (b *Bank) PlayAt(id string, pos engine.Vec2) {
	d := pos.Sub(b.listener)
	gain := attenuation(math.Hypot(d.X, d.Y))
	if gain <= 0 {
		// Считать голоса и интервалы для неслышимого звука незачем: иначе
		// драка за экраном съедала бы лимиты у драки на экране.
		return
	}
	l, r := pan(d.X)
	b.play(id, gain*l, gain*r)
}

func (b *Bank) play(id string, left, right float64) {
	s, ok := b.sounds[id]
	if !ok {
		return
	}
	if s.cooldown > 0 && b.tick-s.last < s.cooldown && s.last != 0 {
		return
	}
	if s.maxVoices > 0 && len(s.active) >= s.maxVoices {
		return
	}
	s.last = b.tick

	i := b.pick(s)
	k := b.master * b.sfx
	if s.jitter > 0 {
		k *= 1 + (b.random()*2-1)*s.jitter
	}
	v := b.mk(s.pcm[i], clamp01(left*k), clamp01(right*k))
	v.Play()
	s.active = append(s.active, v)
}

// pick выбирает вариант, не повторяя предыдущий: именно повтор подряд, а не
// малое число вариантов, слышится как заедание.
func (b *Bank) pick(s *sound) int {
	if len(s.pcm) == 1 {
		return 0
	}
	i := int(b.random() * float64(len(s.pcm)))
	if i >= len(s.pcm) {
		i = len(s.pcm) - 1
	}
	if i == s.prev {
		i = (i + 1) % len(s.pcm)
	}
	s.prev = i
	return i
}

func (b *Bank) random() float64 {
	b.rng ^= b.rng >> 12
	b.rng ^= b.rng << 25
	b.rng ^= b.rng >> 27
	return float64((b.rng*2685821657736338717)>>11) / float64(1<<53)
}

// attenuation — громкость по расстоянию. Корень вместо линейного спада: на
// слух линейный обрыв читается как «звук выключили на полпути».
func attenuation(dist float64) float64 {
	switch {
	case dist <= nearDist:
		return 1
	case dist >= farDist:
		return 0
	default:
		return math.Sqrt(1 - (dist-nearDist)/(farDist-nearDist))
	}
}

// pan — равномощная панорама по горизонтальному смещению источника.
// Вертикаль не учитываем: сверху и снизу у стереопары одно и то же место.
func pan(dx float64) (left, right float64) {
	p := math.Max(-1, math.Min(1, dx/(config.ScreenW/2)))
	// Полное смещение в один канал оставляет второй пустым — на наушниках это
	// звучит как дырка, поэтому сужаем до 70% ширины.
	p *= 0.7
	a := (p + 1) * math.Pi / 4
	return math.Cos(a) * math.Sqrt2, math.Sin(a) * math.Sqrt2
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

// ebitenVoice — боевая реализация голоса: PCM оборачивается панорамирующим
// читателем, поэтому громкость каналов задаётся без копии буфера.
func ebitenVoice(pcm []byte, left, right float64) voice {
	ctx := eaudio.CurrentContext()
	if ctx == nil {
		return deadVoice{}
	}
	p, err := ctx.NewPlayer(newPanReader(pcm, left, right))
	if err != nil {
		return deadVoice{}
	}
	return p
}

// deadVoice — заглушка на случай, когда устройства нет: игра должна идти и
// без звука, а не падать.
type deadVoice struct{}

func (deadVoice) Play()           {}
func (deadVoice) IsPlaying() bool { return false }
func (deadVoice) Close() error    { return nil }
