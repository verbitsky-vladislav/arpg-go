package main

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
)

// writeWAV сохраняет моно 16-бит PCM. Формат выбран под ebiten: WAV
// декодируется в память один раз и играет без задержки, а моно нужно потому,
// что панораму по позиции источника считает игра, и готовое стерео ей только
// мешало бы.
func writeWAV(path string, samples []float64) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var data bytes.Buffer
	for _, v := range samples {
		// Клип на всякий случай: нормализация держит пик, но сумма слоёв с
		// резонансом может дать выброс в один отсчёт.
		s := int16(math.Round(math.Max(-1, math.Min(1, v)) * 32767))
		binary.Write(&data, binary.LittleEndian, s)
	}

	var buf bytes.Buffer
	buf.WriteString("RIFF")
	binary.Write(&buf, binary.LittleEndian, uint32(36+data.Len()))
	buf.WriteString("WAVEfmt ")
	binary.Write(&buf, binary.LittleEndian, uint32(16))         // размер fmt
	binary.Write(&buf, binary.LittleEndian, uint16(1))          // PCM
	binary.Write(&buf, binary.LittleEndian, uint16(1))          // каналов
	binary.Write(&buf, binary.LittleEndian, uint32(sampleRate)) //
	binary.Write(&buf, binary.LittleEndian, uint32(sampleRate*2))
	binary.Write(&buf, binary.LittleEndian, uint16(2))  // выравнивание блока
	binary.Write(&buf, binary.LittleEndian, uint16(16)) // бит на отсчёт
	buf.WriteString("data")
	binary.Write(&buf, binary.LittleEndian, uint32(data.Len()))
	buf.Write(data.Bytes())

	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// prng — детерминированный генератор: вариации звука должны воспроизводиться
// побайтово, иначе каждый прогон sfxgen переписывал бы все ассеты и засорял
// историю. Xorshift64* — короткий и без зависимостей.
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

// spread — множитель в пределах ±amount (0.06 → ±6%).
func (r *prng) spread(amount float64) float64 { return 1 + (r.float()*2-1)*amount }

// seedFor — стабильный сид по имени звука и номеру варианта (FNV-1a).
func seedFor(id string, variant int) uint64 {
	h := uint64(14695981039346656037)
	for _, b := range []byte(id) {
		h ^= uint64(b)
		h *= 1099511628211
	}
	h ^= uint64(variant + 1)
	return h * 1099511628211
}
