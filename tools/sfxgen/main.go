// Команда sfxgen — синтезатор звуковых эффектов игры.
//
// Звук здесь не ассет, а код: каждый эффект описан параметрами слоёв (см.
// таблицу sounds ниже), а WAV'ы — производная, которую в любой момент можно
// перегенерировать. Это отличает аудио от графики, которую мы покупаем: удар,
// шаг и клик меню — короткие абстрактные звуки, синтез даёт их лучше и
// правится одной цифрой, а не заходом в редактор.
//
// Вариации (_00, _01, _02) — не роскошь: один и тот же семпл, услышанный
// десять раз подряд в драке, начинает звучать как заедание. Варианты
// получаются детерминированным разбросом параметров, поэтому повторный запуск
// не меняет ни байта.
//
//	go run ./tools/sfxgen                 # перегенерировать assets/audio/sfx
//	go run ./tools/sfxgen -only ui_move   # только один звук
//	go run ./tools/sfxgen -variants 5     # больше вариантов
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

// Sound — эффект целиком: слои, категория (она же подкаталог) и то, как игра
// должна с ним обращаться в бою.
type Sound struct {
	Dir    string  // подкаталог внутри assets/audio/sfx
	Level  float64 // пик после нормализации: относительная громкость набора
	Layers []Layer

	// Cooldown — минимальный интервал между запусками одного id, мс. Пять
	// мобов, ударивших в один кадр, дают не «мощно», а щелчок и кашу; интервал
	// склеивает их в один удар.
	Cooldown int
	// Voices — потолок одновременных голосов id. 0 → без ограничения.
	Voices int
	// Jitter — разброс громкости при проигрывании (0.1 → ±10%). Догоняет то,
	// чего не даёт конечное число вариантов: две одинаковые копии подряд
	// перестают быть одинаковыми.
	Jitter float64
}

// sounds — весь звуковой набор игры. Числа подобраны так: сначала форма
// (шум или тон), затем огибающая (насколько резко), и в конце свип фильтра —
// именно он превращает шум в «замах» или «шаг», а не сама волна.
var sounds = map[string]Sound{
	// ── Бой ─────────────────────────────────────────────────────────────
	// Замах — воздух, а не предмет: широкий шум, у которого срез фильтра
	// быстро уезжает вниз. Восходящей высоты нет намеренно, иначе получается
	// свист хлыста, а не движение оружия.
	"swing_light": {
		Dir: "combat", Level: 0.55, Cooldown: 60, Voices: 3, Jitter: 0.10,
		Layers: []Layer{{
			Wave: WaveNoise, Freq: 9000,
			Attack: 0.012, Hold: 0.02, Decay: 0.11,
			LP: 5200, LPEnd: 700, Q: 1.1, HP: 320, Gain: 1,
		}},
	},
	"swing_heavy": {
		Dir: "combat", Level: 0.62, Cooldown: 70, Voices: 3, Jitter: 0.10,
		Layers: []Layer{
			{
				Wave: WaveNoise, Freq: 6000,
				Attack: 0.02, Hold: 0.03, Decay: 0.18,
				LP: 3000, LPEnd: 320, Q: 1.3, HP: 180, Gain: 1,
			},
			// Низкий призвук — вес оружия. Слышен не как тон, а как «тяжесть».
			{
				Wave: WaveSine, Freq: 150, FreqEnd: 70,
				Attack: 0.01, Hold: 0.02, Decay: 0.16, Gain: 0.35,
			},
		},
	},
	// Попадание по плоти: короткий сырой удар без звона. Punch на теле — то,
	// что отличает удар от «пшика».
	"hit_flesh": {
		Dir: "combat", Level: 0.80, Cooldown: 45, Voices: 4, Jitter: 0.12,
		Layers: []Layer{
			{
				Wave: WaveNoise, Freq: 3200,
				Attack: 0.001, Hold: 0.012, Punch: 0.5, Decay: 0.10,
				LP: 1100, LPEnd: 190, Q: 1.4, HP: 90, Gain: 1,
			},
			{
				Wave: WaveSine, Freq: 165, FreqEnd: 62,
				Attack: 0.001, Hold: 0.008, Punch: 0.4, Decay: 0.075, Gain: 0.75,
			},
		},
	},
	// Клинок о доспех: тот же удар плюс две несозвучные синусоиды — из-за
	// того, что их частоты не в гармонии, ухо слышит металл, а не ноту.
	"hit_metal": {
		Dir: "combat", Level: 0.78, Cooldown: 45, Voices: 4, Jitter: 0.12,
		Layers: []Layer{
			{
				Wave: WaveNoise, Freq: 11000,
				Attack: 0.001, Hold: 0.006, Punch: 0.6, Decay: 0.07,
				LP: 8000, LPEnd: 2200, Q: 1.0, HP: 700, Gain: 0.9,
			},
			{Wave: WaveSine, Freq: 2540, Attack: 0.001, Hold: 0.004, Decay: 0.26, Gain: 0.45},
			{Wave: WaveSine, Freq: 3810, Attack: 0.001, Hold: 0.004, Decay: 0.19, Gain: 0.30},
			{Wave: WaveSine, Freq: 190, FreqEnd: 120, Attack: 0.001, Hold: 0.006, Decay: 0.06, Gain: 0.5},
		},
	},
	// Дубина: почти весь звук — низ, верх только чтобы удар «щёлкнул».
	"hit_blunt": {
		Dir: "combat", Level: 0.82, Cooldown: 50, Voices: 3, Jitter: 0.12,
		Layers: []Layer{
			{
				Wave: WaveSine, Freq: 120, FreqEnd: 45,
				Attack: 0.001, Hold: 0.014, Punch: 0.55, Decay: 0.16, Gain: 1,
			},
			{
				Wave: WaveNoise, Freq: 2000,
				Attack: 0.001, Hold: 0.008, Decay: 0.06,
				LP: 900, LPEnd: 260, Q: 1.2, Gain: 0.55,
			},
		},
	},
	// Смерть моба: падающая пила — самый узнаваемый «конец» в пиксельных
	// играх, плюс шумовой выдох, чтобы не звучало как выключенный прибор.
	"enemy_death": {
		Dir: "combat", Level: 0.70, Cooldown: 80, Voices: 3, Jitter: 0.10,
		Layers: []Layer{
			{
				Wave: WaveSaw, Freq: 420, FreqEnd: 78,
				Attack: 0.005, Hold: 0.05, Decay: 0.30,
				LP: 3000, LPEnd: 600, Q: 1.1, Gain: 0.8,
			},
			{
				Wave: WaveNoise, Freq: 4000,
				Attack: 0.01, Hold: 0.04, Decay: 0.26,
				LP: 2400, LPEnd: 400, Q: 0.9, HP: 200, Gain: 0.45,
			},
		},
	},

	// ── Игрок ───────────────────────────────────────────────────────────
	// Урон игроку намеренно громче и «неприятнее» прочего: это единственный
	// звук, который обязан пробиться через кашу боя.
	"hurt": {
		Dir: "player", Level: 0.90, Cooldown: 120, Voices: 2, Jitter: 0.06,
		Layers: []Layer{
			{
				Wave: WaveSquare, Freq: 240, FreqEnd: 96, Duty: 0.35,
				Attack: 0.002, Hold: 0.03, Punch: 0.4, Decay: 0.22,
				LP: 2600, LPEnd: 700, Q: 1.2, Gain: 0.85,
			},
			{
				Wave: WaveNoise, Freq: 3000,
				Attack: 0.001, Hold: 0.01, Decay: 0.12,
				LP: 1800, LPEnd: 400, Q: 1.0, Gain: 0.5,
			},
		},
	},
	// Взятый уровень: восходящее трезвучие через Delay. Единственное место в
	// наборе, где звук длиннее полусекунды — событие забега того стоит.
	"level_up": {
		Dir: "player", Level: 0.85, Cooldown: 400, Voices: 1,
		Layers: []Layer{
			{Wave: WaveTri, Freq: 523, Attack: 0.004, Hold: 0.03, Decay: 0.16, Gain: 0.7},
			{Wave: WaveTri, Freq: 659, Delay: 0.09, Attack: 0.004, Hold: 0.03, Decay: 0.16, Gain: 0.7},
			{Wave: WaveTri, Freq: 784, Delay: 0.18, Attack: 0.004, Hold: 0.03, Decay: 0.18, Gain: 0.75},
			{Wave: WaveTri, Freq: 1046, Delay: 0.27, Attack: 0.004, Hold: 0.06, Decay: 0.40, Gain: 0.9},
			{Wave: WaveSine, Freq: 1568, Delay: 0.27, Attack: 0.004, Hold: 0.04, Decay: 0.36, Gain: 0.3},
		},
	},
	"pickup": {
		Dir: "player", Level: 0.60, Cooldown: 60, Voices: 3, Jitter: 0.08,
		Layers: []Layer{
			{Wave: WaveTri, Freq: 880, Attack: 0.002, Hold: 0.02, Decay: 0.06, Gain: 0.8},
			{Wave: WaveTri, Freq: 1320, Delay: 0.055, Attack: 0.002, Hold: 0.02, Decay: 0.11, Gain: 0.9},
		},
	},
	// Отказ: сумка полна, ячейки заняты, действие не прошло. Нисходящая
	// секунда — самый читаемый «нет» из коротких, и его ни с чем не спутать.
	"ui_denied": {
		Dir: "ui", Level: 0.45, Cooldown: 200, Voices: 1, Jitter: 0.05,
		Layers: []Layer{
			{Wave: WaveTri, Freq: 330, FreqEnd: 247, Attack: 0.003, Hold: 0.02, Decay: 0.14, Gain: 0.8},
		},
	},

	// ── Шаги ────────────────────────────────────────────────────────────
	// Самый частый звук игры, поэтому все они тише остального набора: то, что
	// на одном шаге кажется «слишком тихо», через минуту ходьбы оказывается
	// ровно тем, что нужно.
	"step_grass": {
		Dir: "steps", Level: 0.30, Cooldown: 90, Voices: 4, Jitter: 0.15,
		Layers: []Layer{{
			Wave: WaveNoise, Freq: 14000,
			Attack: 0.001, Hold: 0.008, Decay: 0.055,
			LP: 6000, LPEnd: 2600, Q: 0.8, HP: 900, Gain: 1,
		}},
	},
	"step_stone": {
		Dir: "steps", Level: 0.34, Cooldown: 90, Voices: 4, Jitter: 0.15,
		Layers: []Layer{{
			Wave: WaveNoise, Freq: 9000,
			Attack: 0.001, Hold: 0.004, Punch: 0.3, Decay: 0.045,
			LP: 3400, LPEnd: 800, Q: 1.3, HP: 260, Gain: 1,
		}},
	},
	"step_wood": {
		Dir: "steps", Level: 0.32, Cooldown: 90, Voices: 4, Jitter: 0.15,
		Layers: []Layer{
			{
				Wave: WaveNoise, Freq: 7000,
				Attack: 0.001, Hold: 0.005, Decay: 0.05,
				LP: 2400, LPEnd: 700, Q: 1.1, HP: 200, Gain: 1,
			},
			{Wave: WaveSine, Freq: 210, FreqEnd: 150, Attack: 0.001, Hold: 0.004, Decay: 0.05, Gain: 0.4},
		},
	},
	"step_water": {
		Dir: "steps", Level: 0.36, Cooldown: 110, Voices: 3, Jitter: 0.18,
		Layers: []Layer{{
			Wave: WaveNoise, Freq: 5000,
			Attack: 0.004, Hold: 0.012, Decay: 0.16,
			LP: 1600, LPEnd: 3800, Q: 1.0, HP: 300, Gain: 1,
		}},
	},

	// ── Интерфейс ───────────────────────────────────────────────────────
	// Перебор пунктов слышен чаще всего: он предельно короткий и без низа,
	// иначе быстрый прогон списка превращается в барабанную дробь.
	"ui_move": {
		Dir: "ui", Level: 0.30, Cooldown: 35, Voices: 2,
		Layers: []Layer{{
			Wave: WaveSquare, Freq: 1100, Duty: 0.3,
			Attack: 0.001, Hold: 0.004, Decay: 0.022,
			LP: 5000, LPEnd: 2600, Q: 0.8, HP: 400, Gain: 1,
		}},
	},
	"ui_confirm": {
		Dir: "ui", Level: 0.42, Cooldown: 60, Voices: 2,
		Layers: []Layer{
			{Wave: WaveTri, Freq: 660, Attack: 0.002, Hold: 0.014, Decay: 0.05, Gain: 0.8},
			{Wave: WaveTri, Freq: 990, Delay: 0.04, Attack: 0.002, Hold: 0.016, Decay: 0.08, Gain: 0.9},
		},
	},
	"ui_cancel": {
		Dir: "ui", Level: 0.38, Cooldown: 60, Voices: 2,
		Layers: []Layer{
			{Wave: WaveTri, Freq: 520, Attack: 0.002, Hold: 0.012, Decay: 0.05, Gain: 0.8},
			{Wave: WaveTri, Freq: 350, Delay: 0.04, Attack: 0.002, Hold: 0.016, Decay: 0.09, Gain: 0.85},
		},
	},
	// Надеть вещь: короткий кожано-металлический шорох, а не тон — иначе
	// снаряжение путается на слух с подтверждением в меню.
	"ui_equip": {
		Dir: "ui", Level: 0.45, Cooldown: 80, Voices: 2, Jitter: 0.08,
		Layers: []Layer{
			{
				Wave: WaveNoise, Freq: 8000,
				Attack: 0.002, Hold: 0.01, Decay: 0.09,
				LP: 4200, LPEnd: 1200, Q: 1.0, HP: 500, Gain: 1,
			},
			{Wave: WaveSine, Freq: 1900, Delay: 0.03, Attack: 0.001, Hold: 0.004, Decay: 0.10, Gain: 0.28},
		},
	},
	// Сундук: скрип крышки (медленный свип вверх) и глухой стук об упор.
	"chest_open": {
		Dir: "ui", Level: 0.60, Cooldown: 200, Voices: 1, Jitter: 0.06,
		Layers: []Layer{
			{
				Wave: WaveSaw, Freq: 90, FreqEnd: 150, Vibrato: 0.6, VibratoHz: 22,
				Attack: 0.02, Hold: 0.10, Decay: 0.18,
				LP: 900, LPEnd: 1800, Q: 1.6, Gain: 0.5,
			},
			{
				Wave: WaveNoise, Freq: 2600, Delay: 0.28,
				Attack: 0.001, Hold: 0.01, Punch: 0.4, Decay: 0.12,
				LP: 1200, LPEnd: 260, Q: 1.2, Gain: 0.8,
			},
		},
	},
}

// manifest — то, что читает игра: id → файлы и правила проигрывания.
// Параметры синтеза сюда не попадают намеренно: движку они не нужны, а
// дублирование одних и тех же чисел в двух местах рано или поздно разъезжается.
type manifest struct {
	Sounds map[string]manifestEntry `json:"sounds"`
}

type manifestEntry struct {
	Files    []string `json:"files"`
	Cooldown int      `json:"cooldown_ms,omitempty"`
	Voices   int      `json:"voices,omitempty"`
	Jitter   float64  `json:"jitter,omitempty"`
}

func main() {
	out := flag.String("out", filepath.Join("assets", "audio", "sfx"), "каталог для WAV и манифеста")
	variants := flag.Int("variants", 3, "вариантов на звук")
	only := flag.String("only", "", "сгенерировать только этот id")
	flag.Parse()

	if *variants < 1 {
		log.Fatal("вариантов должно быть хотя бы 1")
	}

	ids := make([]string, 0, len(sounds))
	for id := range sounds {
		if *only != "" && id != *only {
			continue
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		log.Fatalf("нечего генерировать: звука %q нет в наборе", *only)
	}
	sort.Strings(ids)

	m := manifest{Sounds: map[string]manifestEntry{}}
	mPath := filepath.Join(*out, "sfx.json")
	// Старый манифест подхватывается только при -only: там мы переписываем один
	// звук, и остальные записи должны уцелеть. На полном прогоне он собирается
	// заново — иначе переименованный звук остался бы в манифесте навсегда.
	if *only != "" {
		if b, err := os.ReadFile(mPath); err == nil {
			_ = json.Unmarshal(b, &m)
			if m.Sounds == nil {
				m.Sounds = map[string]manifestEntry{}
			}
		}
	}

	totalBytes := 0
	for _, id := range ids {
		s := sounds[id]
		files := make([]string, 0, *variants)
		for v := 0; v < *variants; v++ {
			rng := newPRNG(seedFor(id, v))
			buf := render(vary(s.Layers, v, rng), s.Level, rng)
			name := filepath.Join(s.Dir, fmt.Sprintf("%s_%02d.wav", id, v))
			path := filepath.Join(*out, name)
			if err := writeWAV(path, buf); err != nil {
				log.Fatalf("%s: %v", name, err)
			}
			totalBytes += len(buf) * 2
			files = append(files, filepath.ToSlash(name))
		}
		m.Sounds[id] = manifestEntry{
			Files: files, Cooldown: s.Cooldown, Voices: s.Voices, Jitter: s.Jitter,
		}
		fmt.Printf("%-14s %d × %.2f с\n", id, *variants, longest(s.Layers))
	}

	if *only == "" {
		sweep(*out, m)
	}

	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(mPath, append(b, '\n'), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("\n%d звуков, %d файлов, %.1f КБ → %s\n",
		len(ids), len(ids)**variants, float64(totalBytes)/1024, *out)
}

// sweep убирает WAV'ы, которых больше нет в наборе: переименовали звук или
// уменьшили число вариантов — старые файлы иначе остаются в репозитории
// навсегда и выглядят как используемые ассеты.
//
// Трогает только .wav внутри выходного каталога, то есть ровно то, что сам и
// пишет; всё остальное там ему не принадлежит.
func sweep(out string, m manifest) {
	keep := map[string]bool{}
	for _, e := range m.Sounds {
		for _, f := range e.Files {
			keep[filepath.Join(out, filepath.FromSlash(f))] = true
		}
	}
	_ = filepath.WalkDir(out, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".wav" || keep[path] {
			return nil
		}
		if err := os.Remove(path); err != nil {
			log.Printf("не удалось убрать лишний %s: %v", path, err)
			return nil
		}
		fmt.Printf("убран лишний файл: %s\n", path)
		return nil
	})
}

// vary разбрасывает параметры варианта. Вариант 0 остаётся эталонным: если
// звук не нравится, крутить надо таблицу, и удобно, когда хотя бы один файл
// отвечает ей ровно.
func vary(layers []Layer, variant int, rng *prng) []Layer {
	if variant == 0 {
		return layers
	}
	out := make([]Layer, len(layers))
	copy(out, layers)
	for i := range out {
		l := &out[i]
		// Высота и срез фильтра гуляют вместе: разъехавшись, они дают не
		// «другой удар», а другой материал.
		k := rng.spread(0.07)
		l.Freq *= k
		l.FreqEnd *= k
		l.LP *= rng.spread(0.10)
		l.LPEnd *= rng.spread(0.10)
		l.Decay *= rng.spread(0.12)
		l.Hold *= rng.spread(0.10)
	}
	return out
}

func longest(layers []Layer) float64 {
	d := 0.0
	for _, l := range layers {
		if v := l.dur(); v > d {
			d = v
		}
	}
	return d
}
