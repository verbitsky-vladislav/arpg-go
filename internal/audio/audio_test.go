package audio

import (
	"math"
	"os"
	"testing"
	"testing/fstest"

	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/engine"
)

// Тесты проверяют то, ради чего пакет и существует: чтобы бой не превращался
// в кашу. Само звучание проверяется ушами, а вот интервалы, потолок голосов и
// затухание по дистанции — обычная логика, и ошибаться в ней нельзя.

// fakeVoice — голос без устройства: помнит, с какой громкостью и каким
// вариантом его запустили.
type fakeVoice struct {
	left, right float64
	variant     int
	playing     bool
	closed      bool
}

func (v *fakeVoice) Play()           { v.playing = true }
func (v *fakeVoice) IsPlaying() bool { return v.playing }
func (v *fakeVoice) Close() error    { v.closed = true; return nil }

// testBank собирает банк из одного звука с заданными правилами, минуя диск.
// Вариант помечен первым байтом PCM — по нему тест и узнаёт, что выбрано.
func testBank(t *testing.T, variants, cooldownMS, voices int) (*Bank, *[]*fakeVoice) {
	t.Helper()
	pcm := make([][]byte, variants)
	for i := range pcm {
		pcm[i] = make([]byte, 64)
		pcm[i][0] = byte(i)
	}
	var made []*fakeVoice
	b := &Bank{
		sounds: map[string]*sound{
			"test": {
				pcm:       pcm,
				cooldown:  cooldownMS * config.TPS / 1000,
				maxVoices: voices,
				prev:      -1,
			},
		},
		master: 1, sfx: 1, rng: 12345,
	}
	b.mk = func(pcm []byte, l, r float64) voice {
		v := &fakeVoice{left: l, right: r, variant: int(pcm[0])}
		made = append(made, v)
		return v
	}
	return b, &made
}

func TestCooldownSwallowsRepeats(t *testing.T) {
	// Пять мобов, ударивших в один кадр, должны дать один удар, а не пять.
	b, made := testBank(t, 1, 50, 0) // 50 мс = 3 тика
	b.tick = 100
	for i := 0; i < 5; i++ {
		b.Play("test")
	}
	if len(*made) != 1 {
		t.Fatalf("в один тик запущено %d голосов, ожидался 1", len(*made))
	}
	b.tick += 2 // интервал ещё не вышел
	b.Play("test")
	if len(*made) != 1 {
		t.Fatalf("звук прошёл раньше интервала: голосов %d", len(*made))
	}
	b.tick += 2
	b.Play("test")
	if len(*made) != 2 {
		t.Fatalf("после интервала звук не прошёл: голосов %d", len(*made))
	}
}

func TestVoiceCapHolds(t *testing.T) {
	b, made := testBank(t, 1, 0, 2)
	for i := 0; i < 6; i++ {
		b.tick = i * 10
		b.Play("test")
	}
	if len(*made) != 2 {
		t.Fatalf("потолок в 2 голоса пробит: запущено %d", len(*made))
	}
	// Отзвучавший голос освобождает место и закрывается.
	(*made)[0].playing = false
	b.Update()
	if !(*made)[0].closed {
		t.Error("отзвучавший голос не закрыт — утечка плеера")
	}
	b.Play("test")
	if len(*made) != 3 {
		t.Fatalf("освободившийся голос не переиспользован: запущено %d", len(*made))
	}
}

func TestVariantsNeverRepeatBackToBack(t *testing.T) {
	// Именно повтор подряд слышится как заедание, а не малое число вариантов.
	b, made := testBank(t, 3, 0, 0)
	for i := 0; i < 40; i++ {
		b.tick = i * 10
		b.Play("test")
	}
	if len(*made) != 40 {
		t.Fatalf("запущено %d голосов из 40", len(*made))
	}
	seen := map[int]int{}
	for i, v := range *made {
		seen[v.variant]++
		if i > 0 && v.variant == (*made)[i-1].variant {
			t.Fatalf("вариант %d повторился подряд на шаге %d", v.variant, i)
		}
	}
	// И при этом перебираются все: застрять на двух из трёх тоже плохо.
	if len(seen) != 3 {
		t.Errorf("использовано вариантов: %d из 3 (%v)", len(seen), seen)
	}
}

func TestUnknownIDIsSilentNotFatal(t *testing.T) {
	// Опечатка в id не должна ронять игру: звука просто нет.
	b, made := testBank(t, 1, 0, 0)
	b.Play("нет такого")
	b.PlayAt("нет такого", engine.Vec2{})
	if len(*made) != 0 {
		t.Fatalf("несуществующий звук что-то запустил: %d", len(*made))
	}
}

func TestDistantSoundIsDropped(t *testing.T) {
	b, made := testBank(t, 1, 0, 0)
	b.SetListener(engine.Vec2{X: 0, Y: 0})
	b.PlayAt("test", engine.Vec2{X: farDist + 10})
	if len(*made) != 0 {
		t.Fatalf("звук за пределом слышимости проигран: %d", len(*made))
	}
	// И не должен был потратить интервал звука, слышимого рядом.
	b.PlayAt("test", engine.Vec2{X: 10})
	if len(*made) != 1 {
		t.Fatalf("ближний звук не прошёл: %d", len(*made))
	}
}

func TestPanFollowsSource(t *testing.T) {
	b, made := testBank(t, 1, 0, 0)
	b.SetListener(engine.Vec2{})
	b.PlayAt("test", engine.Vec2{X: -config.ScreenW / 2}) // источник слева
	if len(*made) != 1 {
		t.Fatal("звук не проигран")
	}
	v := (*made)[0]
	if v.left <= v.right {
		t.Errorf("источник слева, а каналы %.3f/%.3f", v.left, v.right)
	}
	// Второй канал не должен обнуляться: в наушниках это слышно как дырка.
	if v.right < 0.2 {
		t.Errorf("правый канал почти пуст: %.3f", v.right)
	}
}

func TestAttenuationFalls(t *testing.T) {
	if got := attenuation(0); got != 1 {
		t.Errorf("в упор громкость %.3f, ожидалась 1", got)
	}
	if got := attenuation(nearDist); got != 1 {
		t.Errorf("на границе ближней зоны громкость %.3f, ожидалась 1", got)
	}
	if got := attenuation(farDist); got != 0 {
		t.Errorf("за пределом слышимости громкость %.3f, ожидался 0", got)
	}
	mid := attenuation((nearDist + farDist) / 2)
	if mid <= 0 || mid >= 1 {
		t.Errorf("на середине громкость %.3f — вне (0, 1)", mid)
	}
	// Монотонность: любой шаг дальше не может стать громче.
	prev := 1.0
	for d := 0.0; d < farDist+50; d += 17 {
		if v := attenuation(d); v > prev+1e-9 {
			t.Fatalf("громкость выросла с расстоянием на %.0f px: %.3f → %.3f", d, prev, v)
		} else {
			prev = v
		}
	}
}

func TestVolumeSettingsScalePlayback(t *testing.T) {
	b, made := testBank(t, 1, 0, 0)
	b.SetVolumes(50, 50, 100)
	b.Play("test")
	v := (*made)[0]
	// Play — звук вне мира (интерфейс, события забега): идёт в оба канала
	// целиком, панорама к нему не применяется. Значит остаётся произведение
	// общей громкости на громкость эффектов: 0.5 × 0.5.
	if want := 0.25; math.Abs(v.left-want) > 1e-9 || math.Abs(v.right-want) > 1e-9 {
		t.Errorf("каналы %.3f/%.3f, ожидалось %.3f", v.left, v.right, want)
	}
	b.SetVolumes(0, 100, 100)
	b.Play("test")
	if got := (*made)[1].left; got != 0 {
		t.Errorf("при нулевой общей громкости канал %.3f", got)
	}
}

func TestLoadReadsManifestAndFiles(t *testing.T) {
	// Проверяем связку с настоящими ассетами: манифест sfxgen должен читаться
	// этим пакетом без переходников.
	fsys := os.DirFS("../../assets")
	b, err := load(fsys, "audio")
	if err != nil {
		t.Fatalf("банк не собрался: %v", err)
	}
	for _, id := range []string{SwingLight, HitFlesh, Hurt, StepGrass, UIMove, LevelUp} {
		s, ok := b.sounds[id]
		if !ok {
			t.Errorf("в банке нет звука %s", id)
			continue
		}
		if len(s.pcm) < 2 {
			t.Errorf("%s: вариантов %d, ожидалось хотя бы 2", id, len(s.pcm))
		}
		for i, pcm := range s.pcm {
			if len(pcm) == 0 {
				t.Errorf("%s вариант %d: пустой PCM", id, i)
			}
			if len(pcm)%4 != 0 {
				t.Errorf("%s вариант %d: %d байт — не целое число стерео-кадров",
					id, i, len(pcm))
			}
		}
	}
}

func TestMissingManifestIsAnError(t *testing.T) {
	if _, err := load(fstest.MapFS{}, "audio"); err == nil {
		t.Fatal("отсутствие манифеста должно быть ошибкой загрузки")
	}
}
