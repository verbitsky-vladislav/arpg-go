package audio

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"

	eaudio "github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/audio/vorbis"
	"github.com/hajimehoshi/ebiten/v2/audio/wav"

	"github.com/vladislav/game/internal/config"
)

// Музыка биомов: один играющий трек, зацикленный, со сменой через кроссфейд.
//
// Устроена иначе, чем эффекты, и это не случайность. Эффект — событие: он
// короткий, их много одновременно, у каждого своя точка в мире. Трек — фон:
// он один, он бесконечный, он не имеет позиции и не должен ни начинаться, ни
// обрываться заметно.

// fadeTicks — длительность кроссфейда. Полторы секунды: короче — смена
// биома слышна как переключение дорожки, длиннее — два трека успевают
// помешать друг другу.
const fadeTicks = 3 * config.TPS / 2

// musicTrack — трек, готовый к проигрыванию.
//
// Хранится сырым файлом, а не распакованным PCM. Минута стерео в распакованном
// виде — это 11 МБ в памяти и две секунды на приведение частоты при запуске
// игры; поток же разбирается лениво, по буферу за раз, и не стоит ничего.
// Эффекты, наоборот, распакованы заранее — им нужна нулевая задержка, а весит
// весь набор мегабайт.
type musicTrack struct {
	file string  // имя файла: по нему выбирается разборщик (см. decodeMusic)
	raw  []byte  // содержимое файла как есть
	size int64   // длина потока после приведения частоты, байт
	gain float64 // громкость трека из манифеста: треки сведены по-разному
}

// musicStream — то общее, что нужно от разобранного трека: читать, перематывать
// и знать свою длину (без неё нечем задать точку зацикливания). Ни у wav, ни у
// vorbis общего интерфейса в ebiten нет, хотя оба его и реализуют.
type musicStream interface {
	io.ReadSeeker
	Length() int64
}

// decodeMusic разбирает трек по расширению имени.
//
// В репозитории музыка лежит WAV — её пишет tools/musgen, и хранить исходник
// сжатым было бы враньём. В раздачу она едет OGG: минута стерео весит 5 МБ
// против одного, а это половина всего, что скачивает игрок (tools/pack).
// Формат читается из имени, а не угадывается по содержимому: имя всё равно
// записано в манифесте, и расхождение с ним — ошибка данных, а не догадка.
func decodeMusic(file string, raw []byte) (musicStream, error) {
	if strings.EqualFold(path.Ext(file), ".ogg") {
		return vorbis.DecodeWithSampleRate(SampleRate, bytes.NewReader(raw))
	}
	return wav.DecodeWithSampleRate(SampleRate, bytes.NewReader(raw))
}

// music — состояние музыкального слоя.
type music struct {
	tracks map[string]musicTrack

	cur  string         // что играет (или к чему ведём)
	now  *eaudio.Player // выходящий на передний план
	old  *eaudio.Player // затухающий
	oldG float64        // его громкость на момент начала ухода
	fade int            // тиков кроссфейда осталось
}

type musicManifest struct {
	Tracks map[string]struct {
		File string  `json:"file"`
		Gain float64 `json:"gain"`
	} `json:"tracks"`
}

// loadMusic читает манифест и декодирует треки.
//
// Отсутствие музыки — не ошибка: биомов много, треков пока один, и забег в
// биоме без музыки должен идти в тишине, а не падать.
func loadMusic(fsys fs.FS, dir string) (*music, error) {
	m := &music{tracks: map[string]musicTrack{}}
	b, err := fs.ReadFile(fsys, path.Join(dir, "music.json"))
	if err != nil {
		return m, nil // манифеста нет — играем без музыки
	}
	var mf musicManifest
	if err := json.Unmarshal(b, &mf); err != nil {
		return m, fmt.Errorf("манифест музыки: %w", err)
	}
	for id, e := range mf.Tracks {
		raw, err := fs.ReadFile(fsys, path.Join(dir, e.File))
		if err != nil {
			return m, fmt.Errorf("музыка %s: %w", id, err)
		}
		// Заголовок разбирается сразу, сэмплы — нет: так битый файл виден при
		// загрузке (а не тишиной посреди забега), и сразу известна длина
		// потока, без которой нечем задать точку зацикливания.
		st, err := decodeMusic(e.File, raw)
		if err != nil {
			return m, fmt.Errorf("музыка %s: %w", id, err)
		}
		g := e.Gain
		if g <= 0 {
			g = 1
		}
		m.tracks[id] = musicTrack{file: e.File, raw: raw, size: st.Length(), gain: g}
	}
	return m, nil
}

// play переводит музыку на трек id. Пустой или неизвестный id — уход в
// тишину. Повторный вызов с тем же id ничего не делает, поэтому сцене можно
// звать его хоть каждый кадр и не хранить состояние у себя.
func (m *music) play(id string) {
	if m == nil || id == m.cur {
		return
	}
	m.cur = id

	// Прошлый затухающий не дожидается своей очереди: держать больше двух
	// треков разом незачем, а на быстрых переходах их набежало бы сколько
	// угодно.
	if m.old != nil {
		_ = m.old.Close()
		m.old = nil
	}
	if m.now != nil {
		m.old, m.oldG = m.now, m.now.Volume()
		m.now = nil
	}
	m.fade = fadeTicks

	t, ok := m.tracks[id]
	if !ok {
		return // тишина: старый трек всё равно доиграет кроссфейд
	}
	ctx := eaudio.CurrentContext()
	if ctx == nil {
		return
	}
	st, err := decodeMusic(t.file, t.raw)
	if err != nil {
		return // при загрузке файл разбирался — сюда попасть неоткуда
	}
	// Зацикливание — на уровне потока, а не перезапуском плеера по концу:
	// перезапуск даёт паузу в кадр, и на стыке слышен провал.
	p, err := ctx.NewPlayer(eaudio.NewInfiniteLoop(st, t.size))
	if err != nil {
		return
	}
	p.SetVolume(0)
	p.Play()
	m.now = p
}

// update ведёт кроссфейд. Громкость пересчитывается каждый тик целиком, а не
// подкручивается шагами: так изменение настройки громкости подхватывается на
// лету и не спорит с фейдом.
func (m *music) update(vol float64) {
	if m == nil {
		return
	}
	if m.fade > 0 {
		m.fade--
	}
	k := 1 - float64(m.fade)/fadeTicks // 0 в начале перехода, 1 в конце

	if m.now != nil {
		m.now.SetVolume(vol * m.trackGain(m.cur) * k)
	}
	if m.old != nil {
		if m.fade == 0 {
			_ = m.old.Close()
			m.old = nil
		} else {
			m.old.SetVolume(m.oldG * (1 - k))
		}
	}
}

func (m *music) trackGain(id string) float64 {
	if t, ok := m.tracks[id]; ok {
		return t.gain
	}
	return 1
}
