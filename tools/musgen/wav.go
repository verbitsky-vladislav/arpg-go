package main

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
)

// writeWAV сохраняет стерео 16 бит. Музыка, в отличие от эффектов, пишется
// сразу двумя каналами: ширина у неё своя и постоянная, панораму по позиции
// источника ей никто считать не будет.
func writeWAV(path string, s stereo) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var data bytes.Buffer
	for i := range s.l {
		binary.Write(&data, binary.LittleEndian, pcm16(s.l[i]))
		binary.Write(&data, binary.LittleEndian, pcm16(s.r[i]))
	}

	var buf bytes.Buffer
	buf.WriteString("RIFF")
	binary.Write(&buf, binary.LittleEndian, uint32(36+data.Len()))
	buf.WriteString("WAVEfmt ")
	binary.Write(&buf, binary.LittleEndian, uint32(16))         // размер fmt
	binary.Write(&buf, binary.LittleEndian, uint16(1))          // PCM
	binary.Write(&buf, binary.LittleEndian, uint16(2))          // каналов
	binary.Write(&buf, binary.LittleEndian, uint32(sampleRate)) //
	binary.Write(&buf, binary.LittleEndian, uint32(sampleRate*4))
	binary.Write(&buf, binary.LittleEndian, uint16(4))  // выравнивание блока
	binary.Write(&buf, binary.LittleEndian, uint16(16)) // бит на отсчёт
	buf.WriteString("data")
	binary.Write(&buf, binary.LittleEndian, uint32(data.Len()))
	buf.Write(data.Bytes())

	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func pcm16(v float64) int16 {
	return int16(math.Round(math.Max(-1, math.Min(1, v)) * 32767))
}

// prng — детерминированный генератор (xorshift64*). Шум ветра обязан
// воспроизводиться побайтово, иначе каждый прогон переписывал бы многомегабайтный
// файл и засорял историю репозитория.
type prng struct{ s uint64 }

func newPRNG(seed uint64) *prng {
	if seed == 0 {
		seed = 0x9e3779b97f4a7c15
	}
	return &prng{s: seed}
}

func (r *prng) next() uint64 {
	r.s ^= r.s >> 12
	r.s ^= r.s << 25
	r.s ^= r.s >> 27
	return r.s * 2685821657736338717
}

func (r *prng) float() float64 { return float64(r.next()>>11) / float64(1<<53) }

// seedFor — стабильный сид по имени трека (FNV-1a).
func seedFor(id string) uint64 {
	h := uint64(14695981039346656037)
	for _, b := range []byte(id) {
		h ^= uint64(b)
		h *= 1099511628211
	}
	return h
}
