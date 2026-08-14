package main

import (
	"math"
	"testing"
)

// Проверяется то, из-за чего луп биома обычно негоден: щелчок на стыке,
// мёртвый хвост, перегруз и схлопнувшееся в моно стерео. «Красиво» тесты не
// ловят — это уши.

func rms(buf []float64) float64 {
	sum := 0.0
	for _, v := range buf {
		sum += v * v
	}
	return math.Sqrt(sum / float64(len(buf)))
}

func peakOf(s stereo) float64 {
	p := 0.0
	for i := range s.l {
		p = math.Max(p, math.Max(math.Abs(s.l[i]), math.Abs(s.r[i])))
	}
	return p
}

func TestLoopHasNoGapAtTheSeam(t *testing.T) {
	// Классический провал лупа: хвост доигрывает за границей файла, никто его
	// не подхватывает, и каждый повтор начинается с тишины. Полсекунды до
	// конца и полсекунды после начала должны быть сопоставимы по громкости.
	s := compose("forest", 0.72)
	w := int(0.5 * sampleRate)
	head, tail := rms(s.l[:w]), rms(s.l[len(s.l)-w:])
	if head <= 0 || tail <= 0 {
		t.Fatalf("на стыке тишина: начало %.4f, конец %.4f", head, tail)
	}
	if ratio := math.Max(head, tail) / math.Min(head, tail); ratio > 3 {
		t.Errorf("громкость на стыке скачет в %.1f раза (начало %.4f, конец %.4f)",
			ratio, head, tail)
	}
}

func TestLoopSeamHasNoClick(t *testing.T) {
	// Разрыв формы волны между последним и первым отсчётом слышен как щелчок
	// раз в минуту — то есть ровно на каждом повторе.
	s := compose("forest", 0.72)
	n := len(s.l)
	peak := peakOf(s)
	for _, ch := range [][]float64{s.l, s.r} {
		// Сравниваем со средним шагом внутри материала: у музыки он не нулевой,
		// и требовать буквального совпадения краёв бессмысленно.
		step := 0.0
		for i := 1; i < 2000; i++ {
			step += math.Abs(ch[i] - ch[i-1])
		}
		step /= 1999
		seam := math.Abs(ch[0] - ch[n-1])
		if seam > math.Max(step*8, peak*0.05) {
			t.Errorf("разрыв на стыке %.4f при обычном шаге %.4f — будет щелчок",
				seam, step)
		}
	}
}

func TestNoClipping(t *testing.T) {
	s := compose("forest", 0.72)
	if p := peakOf(s); p > 0.72+1e-9 {
		t.Errorf("пик %.3f выше заданного 0.72 — при сведении с эффектами пойдёт клип", p)
	} else if p < 0.7 {
		t.Errorf("пик %.3f — нормализация не сработала", p)
	}
}

func TestTrackIsAudibleButNotLoud(t *testing.T) {
	s := compose("forest", 0.72)
	// Музыка должна быть подложкой, а не солистом: слишком плотный микс
	// задавит удары и шаги.
	if v := rms(s.l); v < 0.03 || v > 0.30 {
		t.Errorf("RMS %.3f вне разумного для фоновой музыки диапазона", v)
	}
}

func TestStereoIsNotCollapsed(t *testing.T) {
	// Если каналы совпадают, вся расстройка и панорама пропали, а трек
	// звучит как моно из одной точки.
	s := compose("forest", 0.72)
	diff := 0.0
	for i := range s.l {
		diff += math.Abs(s.l[i] - s.r[i])
	}
	diff /= float64(len(s.l))
	if diff < 0.005 {
		t.Errorf("каналы почти совпали (среднее расхождение %.5f) — стерео схлопнулось", diff)
	}
}

func TestRenderIsDeterministic(t *testing.T) {
	// Файл весит мегабайты: недетерминированный прогон переписывал бы его
	// каждый раз и раздувал историю репозитория.
	a := compose("forest", 0.72)
	b := compose("forest", 0.72)
	if len(a.l) != len(b.l) {
		t.Fatalf("длины расходятся: %d и %d", len(a.l), len(b.l))
	}
	for i := range a.l {
		if a.l[i] != b.l[i] || a.r[i] != b.r[i] {
			t.Fatalf("отсчёт %d разошёлся", i)
		}
	}
}

func TestLoopLengthMatchesScore(t *testing.T) {
	s := compose("forest", 0.72)
	want := bars * beatsBar * 60 / bpm // 16 тактов по 4 доли при 60 BPM
	if got := float64(len(s.l)) / sampleRate; math.Abs(got-float64(want)) > 0.01 {
		t.Errorf("луп длиной %.2f с, партитура задаёт %d с", got, want)
	}
}

func TestMelodyStaysInScale(t *testing.T) {
	// Пентатоника ля минор: A C D E G. Нота вне её столкнётся с гармонией —
	// на слух это ловится не сразу, а глазами по номеру MIDI не ловится вовсе.
	scale := map[int]bool{9: true, 0: true, 2: true, 4: true, 7: true} // A C D E G
	for _, n := range melody {
		if !scale[n.Pitch%12] {
			t.Errorf("нота %d вне пентатоники ля минор", n.Pitch)
		}
	}
}

func TestArpEntersAfterIntro(t *testing.T) {
	// Перебор должен входить не сразу: вход партии — это развитие, а трек,
	// который с первой секунды звучит всем составом, дальше не идёт никуда.
	s := compose("forest", 0.72)
	w := int(4 * 60 / bpm * sampleRate) // первые четыре такта
	intro, later := rms(s.l[:w]), rms(s.l[w:2*w])
	if later <= intro {
		t.Errorf("после вступления плотность не выросла (%.4f → %.4f)", intro, later)
	}
}
