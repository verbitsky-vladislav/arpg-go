package main

import (
	"math"
	"testing"
)

// Звук нельзя проверить тестом на «нравится», но можно проверить на то, из-за
// чего он почти всегда оказывается плохим: тишина, перегруз, щелчок на краю и
// молчащий свип фильтра. Всё остальное решается ушами.

func TestRenderHitsTargetPeak(t *testing.T) {
	rng := newPRNG(1)
	buf := render(sounds["hit_flesh"].Layers, 0.8, rng)
	peak := 0.0
	for _, v := range buf {
		peak = math.Max(peak, math.Abs(v))
	}
	// Микрофейды срезают край, поэтому пик может оказаться чуть ниже цели, но
	// не выше — иначе при сложении с другими голосами пойдёт клип.
	if peak > 0.8 || peak < 0.7 {
		t.Fatalf("пик %.3f, ожидался около 0.8", peak)
	}
}

func TestRenderStartsAndEndsSilent(t *testing.T) {
	for id, s := range sounds {
		buf := render(s.Layers, s.Level, newPRNG(seedFor(id, 0)))
		if len(buf) < 64 {
			t.Fatalf("%s: буфер длиной %d — звук пустой", id, len(buf))
		}
		// Ненулевой отсчёт на границе файла — это щелчок при каждом
		// проигрывании, самый заметный дефект короткого сэмпла.
		if math.Abs(buf[0]) > 1e-6 {
			t.Errorf("%s: первый отсчёт %.6f, ожидалась тишина", id, buf[0])
		}
		if last := buf[len(buf)-1]; math.Abs(last) > 1e-6 {
			t.Errorf("%s: последний отсчёт %.6f, ожидалась тишина", id, last)
		}
	}
}

func TestEverySoundIsAudible(t *testing.T) {
	for id, s := range sounds {
		buf := render(s.Layers, s.Level, newPRNG(seedFor(id, 0)))
		sum := 0.0
		for _, v := range buf {
			sum += v * v
		}
		rms := math.Sqrt(sum / float64(len(buf)))
		// Порог низкий намеренно: шаги и клики меню действительно тихие,
		// ловим только полностью развалившиеся параметры.
		if rms < 0.01 {
			t.Errorf("%s: RMS %.4f — звук практически неслышен", id, rms)
		}
		if rms > 0.5 {
			t.Errorf("%s: RMS %.4f — звук задавит остальной набор", id, rms)
		}
	}
}

// zcr — доля смен знака: дешёвая мера «яркости». Точного спектра не даёт, но
// падение свипа фильтра ловит уверенно.
func zcr(buf []float64) float64 {
	if len(buf) < 2 {
		return 0
	}
	n := 0
	for i := 1; i < len(buf); i++ {
		if (buf[i-1] < 0) != (buf[i] < 0) {
			n++
		}
	}
	return float64(n) / float64(len(buf)-1)
}

func TestFilterSweepDarkensSound(t *testing.T) {
	// Слой со свипом вниз обязан глохнуть к концу: если LPEnd потеряется,
	// свип исчезнет молча, а замах превратится в ровный шум.
	//
	// Меряем слои поодиночке, а не готовый звук: в миксе слои гаснут в разное
	// время, и яркость суммы говорит о том, кто дожил до хвоста, а не о работе
	// фильтра. Только шум: у тонального слоя число смен знака задаёт основной
	// тон, и фильтр его почти не двигает.
	for id, s := range sounds {
		for i, l := range s.Layers {
			if l.Wave != WaveNoise || l.LP <= 0 || l.LPEnd <= 0 || l.LPEnd >= l.LP {
				continue
			}
			l.Delay = 0 // тишина в начале сместила бы половины
			buf := render([]Layer{l}, 0.8, newPRNG(seedFor(id, i)))
			half := len(buf) / 2
			first, second := zcr(buf[:half]), zcr(buf[half:])
			if second >= first {
				t.Errorf("%s слой %d: яркость не падает (%.4f → %.4f), свип фильтра не работает",
					id, i, first, second)
			}
		}
	}
}

func TestUpwardSweepBrightensSound(t *testing.T) {
	// Обратный случай — плеск воды светлеет к концу. Тональные слои тем же
	// способом не меряются (см. TestFilterSweepDarkensSound).
	for id, s := range sounds {
		for i, l := range s.Layers {
			if l.Wave != WaveNoise || l.LP <= 0 || l.LPEnd <= l.LP {
				continue
			}
			l.Delay = 0
			buf := render([]Layer{l}, 0.8, newPRNG(seedFor(id, i)))
			half := len(buf) / 2
			first, second := zcr(buf[:half]), zcr(buf[half:])
			if second <= first {
				t.Errorf("%s слой %d: яркость не растёт (%.4f → %.4f)", id, i, first, second)
			}
		}
	}
}

func TestRenderIsDeterministic(t *testing.T) {
	// Вариации обязаны воспроизводиться побайтово: иначе каждый прогон
	// sfxgen переписывает все ассеты и засоряет историю репозитория.
	a := render(vary(sounds["swing_light"].Layers, 2, newPRNG(seedFor("swing_light", 2))), 0.6,
		newPRNG(seedFor("swing_light", 2)))
	b := render(vary(sounds["swing_light"].Layers, 2, newPRNG(seedFor("swing_light", 2))), 0.6,
		newPRNG(seedFor("swing_light", 2)))
	if len(a) != len(b) {
		t.Fatalf("длины расходятся: %d и %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("отсчёт %d разошёлся: %.9f и %.9f", i, a[i], b[i])
		}
	}
}

func TestVariantsDifferButStayRecognizable(t *testing.T) {
	base := sounds["hit_flesh"]
	v0 := render(vary(base.Layers, 0, newPRNG(seedFor("hit_flesh", 0))), base.Level, newPRNG(1))
	v1 := render(vary(base.Layers, 1, newPRNG(seedFor("hit_flesh", 1))), base.Level, newPRNG(2))
	if len(v0) == len(v1) && zcr(v0) == zcr(v1) {
		t.Fatal("варианты неотличимы — разброс параметров не применился")
	}
	// Но и уезжать далеко нельзя: варианты одного удара, а не разные удары.
	if d := math.Abs(zcr(v0)-zcr(v1)) / zcr(v0); d > 0.5 {
		t.Errorf("варианты разошлись на %.0f%% по яркости — это уже разные звуки", d*100)
	}
}

func TestSoundTableIsSane(t *testing.T) {
	for id, s := range sounds {
		if s.Dir == "" {
			t.Errorf("%s: не задан каталог", id)
		}
		if s.Level <= 0 || s.Level > 1 {
			t.Errorf("%s: Level %.2f вне (0, 1]", id, s.Level)
		}
		if len(s.Layers) == 0 {
			t.Errorf("%s: нет слоёв", id)
		}
		for i, l := range s.Layers {
			if l.Gain <= 0 {
				t.Errorf("%s слой %d: Gain %.2f — слой не звучит", id, i, l.Gain)
			}
			if l.Freq <= 0 {
				t.Errorf("%s слой %d: Freq %.1f", id, i, l.Freq)
			}
			if l.dur() > 1.2 {
				t.Errorf("%s слой %d: длительность %.2f с — для эффекта это очень долго",
					id, i, l.dur())
			}
		}
	}
}
