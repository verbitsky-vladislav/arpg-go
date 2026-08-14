package main

import "math"

// Частота дискретизации всех звуков. 44100 — не потому, что ухо слышит выше
// 20 кГц, а потому, что ebiten приводит всё к частоте аудиоконтекста, и
// совпадение избавляет от ресемплинга на каждом проигрывании.
const sampleRate = 44100

// Wave — форма волны голоса.
type Wave int

const (
	WaveSine   Wave = iota // чистый тон: блипы меню, звон металла
	WaveSquare             // жёсткий тон: урон, ретро-сигналы
	WaveSaw                // яркий тон: смерть, падающие завывания
	WaveTri                // мягкий тон: подбор, негромкие подсказки
	WaveNoise              // шум: удары, шаги, всё «материальное»
)

// Layer — один голос звука: осциллятор, огибающая и пара фильтров.
//
// Слои нужны потому, что почти ни один игровой звук не является одним
// осциллятором: попадание — это шумовой удар плюс низкий тон корпуса, а взятый
// уровень — три блипа подряд. Один слой такое не изобразит, а полноценный
// синтезатор с матрицей модуляции здесь избыточен.
type Layer struct {
	Wave  Wave
	Delay float64 // задержка старта от начала звука, сек

	// Freq/FreqEnd — высота в начале и в конце слоя, Гц. Скольжение
	// экспоненциальное (равномерное по нотам, а не по герцам), поэтому падение
	// 400→100 слышится таким же плавным, как 4000→1000. FreqEnd == 0 — без
	// скольжения.
	Freq, FreqEnd float64
	Duty          float64 // скважность WaveSquare (0..1), 0 → 0.5
	Vibrato       float64 // глубина вибрато в полутонах
	VibratoHz     float64

	// Огибающая: атака, удержание, затухание (сек). Punch — надбавка к
	// громкости в начале удержания, спадающая к его концу; именно она делает
	// удар ударом, а не «пшиком».
	Attack, Hold, Decay float64
	Punch               float64

	// LP/LPEnd — срез резонансного фильтра нижних частот в начале и в конце,
	// Гц; 0 — фильтр выключен. Свип вниз — главный приём всего набора: так
	// шум превращается в замах, а щелчок — в шаг по камню.
	LP, LPEnd float64
	Q         float64 // резонанс, 0 → 0.7 (без подъёма на срезе)
	HP        float64 // срез фильтра верхних частот, Гц; 0 — выключен

	Gain float64 // вклад слоя в микс до нормализации
}

// dur — полная длительность слоя вместе с задержкой.
func (l Layer) dur() float64 { return l.Delay + l.Attack + l.Hold + l.Decay }

// env — громкость слоя в момент t (отсчёт от старта слоя, без Delay).
func (l Layer) env(t float64) float64 {
	switch {
	case t < 0:
		return 0
	case t < l.Attack:
		return t / l.Attack
	case t < l.Attack+l.Hold:
		if l.Hold <= 0 {
			return 1
		}
		u := (t - l.Attack) / l.Hold
		return 1 + l.Punch*(1-u)
	default:
		if l.Decay <= 0 {
			return 0
		}
		v := (t - l.Attack - l.Hold) / l.Decay
		if v >= 1 {
			return 0
		}
		// Квадратичный спад: к концу тише, чем линейный, и без щелчка на хвосте.
		return (1 - v) * (1 - v)
	}
}

// glide — экспоненциальная интерполяция from→to по доле пути u (0..1).
// Ноль в to означает «не скользим», ноль во from — «слой молчит».
func glide(from, to, u float64) float64 {
	if to <= 0 || from <= 0 {
		return from
	}
	return from * math.Pow(to/from, clamp01(u))
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

// svf — резонансный фильтр нижних частот (Chamberlin state-variable).
// Двухполюсный: даёт и наклон, и резонанс на срезе, чего однополюсному не
// хватает — без резонанса свип вниз звучит как «выключили», а не как удар.
type svf struct{ low, band float64 }

func (f *svf) run(x, fc, q float64) float64 {
	fc = math.Max(20, math.Min(fc, sampleRate*0.45))
	// Коэффициент Chamberlin теряет устойчивость выше sr/6, поэтому режем.
	k := math.Min(2*math.Sin(math.Pi*fc/sampleRate), 1.4)
	damp := 1.0 / math.Max(0.5, q)
	f.low += k * f.band
	high := x - f.low - damp*f.band
	f.band += k * high
	return f.low
}

// hpf — однополюсный фильтр верхних частот: убирает низовой гул у мелких
// звуков (шаги, клики меню), из-за которого они «пачкают» микс на колонках.
type hpf struct{ yPrev, xPrev float64 }

func (f *hpf) run(x, fc float64) float64 {
	rc := 1 / (2 * math.Pi * math.Max(1, fc))
	a := rc / (rc + 1/float64(sampleRate))
	y := a * (f.yPrev + x - f.xPrev)
	f.yPrev, f.xPrev = y, x
	return y
}

// osc — осциллятор с накоплением фазы. Для WaveNoise фаза задаёт не высоту
// тона, а скорость обновления шумовой таблицы: так шум получает «размер»
// (мелкая крошка листвы против гулкого удара) до всякой фильтрации.
type osc struct {
	phase float64
	noise [32]float64
	init  bool
	rng   *prng
}

func (o *osc) sample(w Wave, freq, duty float64) float64 {
	if !o.init {
		o.refill()
		o.init = true
	}
	prev := o.phase
	o.phase += freq / sampleRate
	if math.Floor(o.phase) > math.Floor(prev) {
		o.refill()
	}
	p := o.phase - math.Floor(o.phase)
	switch w {
	case WaveSine:
		return math.Sin(2 * math.Pi * p)
	case WaveSquare:
		if duty <= 0 {
			duty = 0.5
		}
		if p < duty {
			return 1
		}
		return -1
	case WaveSaw:
		return 2*p - 1
	case WaveTri:
		return 4*math.Abs(p-0.5) - 1
	default:
		return o.noise[int(p*float64(len(o.noise)))%len(o.noise)]
	}
}

func (o *osc) refill() {
	for i := range o.noise {
		o.noise[i] = o.rng.float()*2 - 1
	}
}

// render собирает звук из слоёв: каждый слой считается отдельно и
// подмешивается со своим Gain, результат нормализуется по пику и получает
// микрофейды по краям (иначе обрыв волны на границе файла даёт щелчок).
func render(layers []Layer, level float64, rng *prng) []float64 {
	total := 0.0
	for _, l := range layers {
		total = math.Max(total, l.dur())
	}
	n := int(total*sampleRate) + 1
	out := make([]float64, n)

	for _, l := range layers {
		o := osc{rng: rng}
		var lp svf
		var hp hpf
		body := l.Attack + l.Hold + l.Decay
		start := int(l.Delay * sampleRate)
		for i := start; i < n; i++ {
			t := float64(i-start) / sampleRate
			if t > body {
				break
			}
			u := t / math.Max(body, 1e-9)

			f := glide(l.Freq, l.FreqEnd, u)
			if l.Vibrato > 0 {
				f *= math.Pow(2, l.Vibrato/12*math.Sin(2*math.Pi*l.VibratoHz*t))
			}
			s := o.sample(l.Wave, f, l.Duty)
			if l.LP > 0 {
				q := l.Q
				if q <= 0 {
					q = 0.7
				}
				s = lp.run(s, glide(l.LP, l.LPEnd, u), q)
			}
			if l.HP > 0 {
				s = hp.run(s, l.HP)
			}
			out[i] += s * l.env(t) * l.Gain
		}
	}

	peak := 0.0
	for _, v := range out {
		peak = math.Max(peak, math.Abs(v))
	}
	if peak > 0 {
		k := level / peak
		for i := range out {
			out[i] *= k
		}
	}
	fade(out)
	return out
}

// fade — 1 мс на входе и 4 мс на выходе. Слышно это только как отсутствие
// щелчка, но без фейдов щёлкает почти каждый звук с шумом.
func fade(buf []float64) {
	in := int(math.Round(0.001 * sampleRate))
	out := int(math.Round(0.004 * sampleRate))
	for i := 0; i < in && i < len(buf); i++ {
		buf[i] *= float64(i) / float64(in)
	}
	for i := 0; i < out && i < len(buf); i++ {
		buf[len(buf)-1-i] *= float64(i) / float64(out)
	}
}
