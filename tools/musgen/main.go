// Команда musgen — синтезатор музыки биомов.
//
// Как и звуки (tools/sfxgen), музыка здесь код: партитура лежит в этом файле,
// а WAV в assets/audio/music — производная, которую можно перегенерировать.
// Прогон детерминирован, поэтому повторный запуск не меняет ни байта.
//
// Отдельная команда, а не режим sfxgen: у музыки другой синтезатор (ноты и
// огибающая с удержанием вместо одного удара), другой формат (стерео) и
// другая задача — не отметить событие, а не надоесть за час.
//
//	go run ./tools/musgen
//	go run ./tools/musgen -only forest
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
)

// Темп и размер. 60 ударов в минуту — доля ровно в секунду, и вся партитура
// читается без пересчёта; для леса это к тому же примерно темп спокойного
// шага.
const (
	bpm      = 60
	beatsBar = 4
	bars     = 16
	// tailBeats — сколько досчитать за концом лупа, чтобы хвосты нот было чем
	// подмешать в начало (см. wrap). Пад с четырёхсекундным спадом требует
	// запаса больше собственного спада.
	tailBeats = 8
)

// chord — аккорд такта: бас и голоса пада.
type chord struct {
	name  string
	bass  int
	tones []int
}

// Ход i–VI–III–VII в ля миноре: Am – F – C – G.
//
// Выбран не случайно: в нём нет доминанты с ведущим тоном, то есть ни один
// такт не «просит» разрешения. Ход, который никуда не стремится, можно
// слушать по кругу час — а гармония с сильным тяготением на третьем повторе
// начинает раздражать.
var progression = []chord{
	{"Am", 45, []int{57, 60, 64}},
	{"F", 41, []int{53, 57, 60}},
	{"C", 48, []int{60, 64, 67}},
	{"G", 43, []int{55, 59, 62}},
}

// Партии. Тембры описаны здесь же: партитура и звук неразделимы, и держать
// их в разных местах значит править обе половины при каждой правке одной.

// padVoice — основа: две расстроенные пилы под фильтром, медленно
// закрывающимся к концу ноты.
var padVoice = Voice{
	Wave: WaveSaw, Unison: 2, Detune: 9,
	Attack: 1.6, Decay: 2.0, Sustain: 0.62, Release: 3.4,
	LP: 1500, LPEnd: 620, Q: 0.9,
	Gain: 0.30,
}

// bassVoice — синус: у баса не должно быть ничего, кроме основного тона,
// иначе он лезет туда же, где живут удары.
var bassVoice = Voice{
	Wave: WaveSine, Unison: 1,
	Attack: 0.06, Decay: 0.7, Sustain: 0.55, Release: 1.2,
	LP: 420, LPEnd: 260, Q: 0.8,
	Gain: 0.42,
}

// arpVoice — перебор: треугольник с быстрым спадом, тихий и почти без низа.
var arpVoice = Voice{
	Wave: WaveTri, Unison: 1,
	Attack: 0.004, Decay: 0.35, Sustain: 0.0, Release: 0.35,
	LP: 3200, LPEnd: 1200, Q: 0.9,
	Pan: -0.25, Gain: 0.20,
}

// leadVoice — редкая мелодия поверх. Держится дольше перебора и стоит в
// центре: это то, за чем следят ухом.
var leadVoice = Voice{
	Wave: WaveTri, Unison: 2, Detune: 5,
	Attack: 0.10, Decay: 0.5, Sustain: 0.55, Release: 1.1,
	LP: 2600, LPEnd: 1100, Q: 0.8,
	Gain: 0.24,
}

// windVoice — шум под низким фильтром: не мелодия, а воздух. Без него
// остальное висит в пустоте и слышно, что это синтезатор.
var windVoice = Voice{
	Wave: WaveNoise, Unison: 1,
	Attack: 3.0, Decay: 1.0, Sustain: 0.8, Release: 3.5,
	LP: 700, LPEnd: 280, Q: 1.1,
	Gain: 0.07,
}

// melody — фраза второй половины лупа: такт, доля в такте, нота, длительность.
// Пентатоника ля минор (A C D E G) — в ней нет полутоновых столкновений с
// гармонией, поэтому фраза ложится на любой такт хода.
var melody = []Note{
	{At: 8*beatsBar + 0, Dur: 2.0, Pitch: 69, Vel: 0.9},   // A4
	{At: 8*beatsBar + 2.5, Dur: 1.5, Pitch: 72, Vel: 0.8}, // C5
	{At: 9*beatsBar + 0.5, Dur: 1.0, Pitch: 69, Vel: 0.7},
	{At: 9*beatsBar + 2, Dur: 2.0, Pitch: 67, Vel: 0.85}, // G4
	{At: 10*beatsBar + 0, Dur: 1.5, Pitch: 64, Vel: 0.8}, // E4
	{At: 10*beatsBar + 2, Dur: 1.0, Pitch: 67, Vel: 0.7},
	{At: 10*beatsBar + 3, Dur: 1.0, Pitch: 69, Vel: 0.75},
	{At: 11*beatsBar + 0, Dur: 3.0, Pitch: 72, Vel: 0.9},
	{At: 12*beatsBar + 0, Dur: 2.0, Pitch: 76, Vel: 0.85},  // E5
	{At: 12*beatsBar + 2.5, Dur: 1.5, Pitch: 74, Vel: 0.8}, // D5
	{At: 13*beatsBar + 0, Dur: 2.0, Pitch: 72, Vel: 0.8},
	{At: 13*beatsBar + 2, Dur: 2.0, Pitch: 69, Vel: 0.75},
	{At: 14*beatsBar + 0, Dur: 1.0, Pitch: 67, Vel: 0.7},
	{At: 14*beatsBar + 1.5, Dur: 1.5, Pitch: 64, Vel: 0.7},
	{At: 14*beatsBar + 3, Dur: 1.0, Pitch: 62, Vel: 0.65}, // D4
	// Последний такт держит тонику через границу лупа: хвост этой ноты wrap
	// подмешает в начало, и повтор не будет слышен как повтор.
	{At: 15*beatsBar + 0, Dur: 4.0, Pitch: 69, Vel: 0.7},
}

// track — что генерировать. Пока один биом; остальные добавляются сюда.
type track struct {
	Level float64 // пик после нормализации
	Gain  float64 // громкость в игре
}

var tracks = map[string]track{
	// Лес: 64 секунды. Короче — слышно, что это луп; длиннее — файл в
	// репозитории растёт быстрее, чем добавляется музыки.
	"forest": {Level: 0.72, Gain: 0.55},
}

type manifest struct {
	Tracks map[string]manifestEntry `json:"tracks"`
}

type manifestEntry struct {
	File string  `json:"file"`
	Gain float64 `json:"gain"`
}

func main() {
	out := flag.String("out", filepath.Join("assets", "audio", "music"), "каталог для WAV и манифеста")
	only := flag.String("only", "", "сгенерировать только этот биом")
	flag.Parse()

	ids := make([]string, 0, len(tracks))
	for id := range tracks {
		if *only != "" && id != *only {
			continue
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		log.Fatalf("нечего генерировать: биома %q нет в наборе", *only)
	}
	sort.Strings(ids)

	m := manifest{Tracks: map[string]manifestEntry{}}
	mPath := filepath.Join(*out, "music.json")
	if *only != "" {
		if b, err := os.ReadFile(mPath); err == nil {
			_ = json.Unmarshal(b, &m)
			if m.Tracks == nil {
				m.Tracks = map[string]manifestEntry{}
			}
		}
	}

	for _, id := range ids {
		t := tracks[id]
		buf := compose(id, t.Level)
		name := id + ".wav"
		if err := writeWAV(filepath.Join(*out, name), buf); err != nil {
			log.Fatalf("%s: %v", name, err)
		}
		m.Tracks[id] = manifestEntry{File: name, Gain: t.Gain}
		fmt.Printf("%-8s %.1f с, %.1f МБ\n", id,
			float64(len(buf.l))/sampleRate, float64(len(buf.l)*4)/(1<<20))
	}

	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(mPath, append(b, '\n'), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("\n%d треков → %s\n", len(ids), *out)
}

// compose собирает партитуру биома и сводит её в бесшовный луп.
func compose(id string, level float64) stereo {
	spb := 60.0 / bpm // секунд на долю
	loopBeats := float64(bars * beatsBar)
	loopN := int(loopBeats * spb * sampleRate)
	fullN := int((loopBeats + tailBeats) * spb * sampleRate)

	var pad, bass, arp []Note
	for b := 0; b < bars; b++ {
		c := progression[b%len(progression)]
		at := float64(b * beatsBar)
		for _, p := range c.tones {
			pad = append(pad, Note{At: at, Dur: beatsBar, Pitch: p, Vel: 0.8})
		}
		bass = append(bass, Note{At: at, Dur: beatsBar, Pitch: c.bass, Vel: 0.9})

		// Перебор входит с пятого такта: первые четыре — только пад и бас,
		// и вход партии слышен как развитие, а не как «включили дорожку».
		if b < 4 {
			continue
		}
		// Восьмые вверх-вниз по тонам аккорда. Ноты через одну тише: без
		// этого ровная цепочка звучит как машина, а не как перебор.
		pattern := []int{0, 1, 2, 1}
		for i := 0; i < beatsBar*2; i++ {
			p := c.tones[pattern[i%len(pattern)]] + 12
			vel := 0.55
			if i%2 == 1 {
				vel = 0.34
			}
			arp = append(arp, Note{At: at + float64(i)*0.5, Dur: 0.45, Pitch: p, Vel: vel})
		}
	}

	// Ветер: длинные перекрывающиеся вздохи по сторонам. Ноты «без высоты»
	// (шум), поэтому важны только их длина и место в стерео.
	var wind []Note
	for i := 0; i < 6; i++ {
		wind = append(wind, Note{
			At:    float64(i) * (loopBeats / 6),
			Dur:   loopBeats/6 + 4,
			Pitch: 40 + i*3, // задаёт «размер» шума, а не тон
			Vel:   0.7 + 0.3*float64(i%2),
		})
	}

	rng := newPRNG(seedFor(id))
	parts := []stereo{
		renderTrack(Track{Voice: padVoice, Notes: pad}, fullN, spb, rng),
		renderTrack(Track{Voice: bassVoice, Notes: bass}, fullN, spb, rng),
		renderTrack(Track{Voice: arpVoice, Notes: arp, Delay: 1.5, Feedback: 0.28}, fullN, spb, rng),
		renderTrack(Track{Voice: leadVoice, Notes: melody, Delay: 2, Feedback: 0.22}, fullN, spb, rng),
		renderTrack(Track{Voice: windVoice, Notes: wind}, fullN, spb, rng),
	}
	return wrap(mix(parts, fullN, level), loopN)
}
