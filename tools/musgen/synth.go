package main

import "math"

// Частота музыки — половина от частоты эффектов. Компромисс осознанный:
// минута стерео на 44100 весит 10 МБ, и такие файлы в репозитории копятся
// быстрее, чем слушаются. Материал биома — мягкие пады, бас и редкие
// переборы, у него почти нет содержания выше 11 кГц, поэтому потолок
// незаметен; ebiten поднимет частоту до контекстной один раз при загрузке.
const sampleRate = 22050

// Wave — форма волны голоса.
type Wave int

const (
	WaveSine Wave = iota
	WaveTri
	WaveSaw
	WaveSquare
	WaveNoise
)

// Voice — тембр: осциллятор, огибающая, фильтр и место в стереокартине.
//
// Отличается от Layer в tools/sfxgen тем, что играет ноты, а не один удар:
// огибающая разделена на удержание (пока нота звучит) и спад (после её
// конца), а расстройка размазывает голос по стерео. Эффекту это не нужно, а
// без этого пад звучит как сигнал будильника.
type Voice struct {
	Wave Wave

	// Attack/Decay/Sustain/Release — классическая огибающая. Sustain — доля
	// от пика (0..1), остальные — секунды.
	Attack, Decay, Sustain, Release float64

	// Detune — расстройка в центах между копиями голоса, Unison — сколько их.
	// Две слегка расстроенные копии, разведённые по сторонам, дают ширину,
	// которой у одного осциллятора не бывает: биения между ними и есть то,
	// что слышится как «живой» пад.
	Detune float64
	Unison int

	// LP/LPEnd — срез фильтра в начале и в конце ноты, Гц. Медленное движение
	// среза заменяет паду развитие: статичный пад надоедает за два повтора.
	LP, LPEnd float64
	Q         float64

	Pan  float64 // -1 слева, +1 справа
	Gain float64
}

// Note — нота: доля от начала лупа в долях (beat), длительность в долях,
// высота в MIDI и сила нажатия.
type Note struct {
	At, Dur float64
	Pitch   int
	Vel     float64
}

// Track — партия: чем играем и что.
type Track struct {
	Voice Voice
	Notes []Note
	// Delay/Feedback — эхо партии: задержка в долях и сколько от неё
	// возвращается. Переборы без эха звучат голо, а с эхом — как будто у
	// биома есть глубина.
	Delay    float64
	Feedback float64
}

// hz — частота ноты MIDI. 69 — это ля первой октавы, 440 Гц.
func hz(pitch int) float64 { return 440 * math.Pow(2, float64(pitch-69)/12) }

// cents сдвигает частоту на c центов (сотых полутона).
func cents(f, c float64) float64 { return f * math.Pow(2, c/1200) }

// svf — резонансный фильтр нижних частот (Chamberlin), тот же, что в sfxgen:
// двухполюсный, с резонансом на срезе.
type svf struct{ low, band float64 }

func (f *svf) run(x, fc, q float64) float64 {
	fc = math.Max(20, math.Min(fc, sampleRate*0.45))
	k := math.Min(2*math.Sin(math.Pi*fc/sampleRate), 1.4)
	damp := 1.0 / math.Max(0.5, q)
	f.low += k * f.band
	high := x - f.low - damp*f.band
	f.band += k * high
	return f.low
}

// osc — осциллятор с накоплением фазы.
type osc struct {
	phase float64
	noise [64]float64
	init  bool
	rng   *prng
}

func (o *osc) sample(w Wave, freq float64) float64 {
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
	case WaveTri:
		return 4*math.Abs(p-0.5) - 1
	case WaveSaw:
		return 2*p - 1
	case WaveSquare:
		if p < 0.5 {
			return 1
		}
		return -1
	default:
		return o.noise[int(p*float64(len(o.noise)))%len(o.noise)]
	}
}

func (o *osc) refill() {
	for i := range o.noise {
		o.noise[i] = o.rng.float()*2 - 1
	}
}

// env — громкость ноты в момент t от её начала; hold — сколько нота держится.
func (v Voice) env(t, hold float64) float64 {
	switch {
	case t < 0:
		return 0
	case t < v.Attack:
		return t / math.Max(v.Attack, 1e-9)
	case t < v.Attack+v.Decay:
		u := (t - v.Attack) / math.Max(v.Decay, 1e-9)
		return 1 + (v.Sustain-1)*u
	case t < hold:
		return v.Sustain
	default:
		u := (t - hold) / math.Max(v.Release, 1e-9)
		if u >= 1 {
			return 0
		}
		return v.Sustain * (1 - u) * (1 - u)
	}
}

// stereo — буфер левого и правого канала.
type stereo struct{ l, r []float64 }

func newStereo(n int) stereo { return stereo{l: make([]float64, n), r: make([]float64, n)} }

func (s stereo) add(i int, v, pan float64) {
	// Равномощная панорама: на краях один канал не пустеет полностью, иначе
	// в наушниках слышно дырку.
	a := (math.Max(-1, math.Min(1, pan)) + 1) * math.Pi / 4
	s.l[i] += v * math.Cos(a) * math.Sqrt2 / 2
	s.r[i] += v * math.Sin(a) * math.Sqrt2 / 2
}

// renderTrack играет партию в буфер длиной n отсчётов. spb — секунд на долю.
func renderTrack(t Track, n int, spb float64, rng *prng) stereo {
	out := newStereo(n)
	v := t.Voice
	uni := max(v.Unison, 1)

	for _, note := range t.Notes {
		start := int(note.At * spb * sampleRate)
		hold := note.Dur * spb
		total := hold + v.Release
		freq := hz(note.Pitch)

		for u := 0; u < uni; u++ {
			// Копии расстраиваются симметрично и разводятся по сторонам:
			// центр остаётся на месте, а звук становится шире.
			off := 0.0
			pan := v.Pan
			if uni > 1 {
				k := float64(u)/float64(uni-1)*2 - 1 // -1..+1
				off = k * v.Detune
				pan = math.Max(-1, math.Min(1, v.Pan+k*0.6))
			}
			o := osc{rng: rng}
			var lp svf
			f := cents(freq, off)
			for i := start; i < n; i++ {
				t0 := float64(i-start) / sampleRate
				if t0 > total {
					break
				}
				s := o.sample(v.Wave, f)
				if v.LP > 0 {
					end := v.LPEnd
					if end <= 0 {
						end = v.LP
					}
					q := v.Q
					if q <= 0 {
						q = 0.7
					}
					cut := v.LP * math.Pow(end/v.LP, math.Min(1, t0/math.Max(total, 1e-9)))
					s = lp.run(s, cut, q)
				}
				out.add(i, s*v.env(t0, hold)*note.Vel*v.Gain/float64(uni), pan)
			}
		}
	}

	if t.Delay > 0 && t.Feedback > 0 {
		echo(out, int(t.Delay*spb*sampleRate), t.Feedback)
	}
	return out
}

// echo — эхо с перекрёстными каналами: повтор уходит на противоположную
// сторону. Так партия занимает ширину, не размазывая при этом свой центр.
func echo(s stereo, delay int, fb float64) {
	if delay <= 0 || delay >= len(s.l) {
		return
	}
	for i := delay; i < len(s.l); i++ {
		s.l[i] += s.r[i-delay] * fb
		s.r[i] += s.l[i-delay] * fb
	}
}

// mix складывает партии и приводит пик к level.
func mix(parts []stereo, n int, level float64) stereo {
	out := newStereo(n)
	for _, p := range parts {
		for i := 0; i < n; i++ {
			out.l[i] += p.l[i]
			out.r[i] += p.r[i]
		}
	}
	peak := 0.0
	for i := 0; i < n; i++ {
		peak = math.Max(peak, math.Max(math.Abs(out.l[i]), math.Abs(out.r[i])))
	}
	if peak > 0 {
		k := level / peak
		for i := 0; i < n; i++ {
			out.l[i] *= k
			out.r[i] *= k
		}
	}
	return out
}

// wrap делает луп бесшовным: хвост, вышедший за конец, подмешивается в
// начало и отрезается.
//
// Без этого на стыке слышен щелчок и обрыв: пад с четырёхсекундным спадом
// доигрывает уже за границей файла, и при повторе его никто не подхватывает.
// Отсюда же требование к самой партитуре — последний такт должен вести
// обратно к первому, иначе бесшовным будет только звук, но не музыка.
func wrap(s stereo, loop int) stereo {
	if loop >= len(s.l) {
		return s
	}
	for i := loop; i < len(s.l); i++ {
		s.l[i-loop] += s.l[i]
		s.r[i-loop] += s.r[i]
	}
	return stereo{l: s.l[:loop], r: s.r[:loop]}
}
