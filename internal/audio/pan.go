package audio

import "io"

// panReader — стерео-PCM с независимой громкостью каналов.
//
// Нужен потому, что у ebiten-плеера есть только общая громкость, а разложить
// звук по сторонам нечем: моно-файл декодер уже развернул в два одинаковых
// канала. Масштабировать их на лету дешевле, чем держать в памяти по копии
// буфера на каждое положение источника.
type panReader struct {
	pcm         []byte
	pos         int
	left, right float64
}

func newPanReader(pcm []byte, left, right float64) *panReader {
	return &panReader{pcm: pcm, left: left, right: right}
}

// frame — стерео-кадр: два 16-битных отсчёта.
const frame = 4

func (p *panReader) Read(b []byte) (int, error) {
	if p.pos >= len(p.pcm) {
		return 0, io.EOF
	}
	n := len(b)
	if avail := len(p.pcm) - p.pos; n > avail {
		n = avail
	}
	// Читаем только целыми кадрами: половина кадра сдвинула бы каналы местами
	// на всём остатке звука.
	n -= n % frame
	if n == 0 {
		return 0, nil
	}
	copy(b[:n], p.pcm[p.pos:p.pos+n])
	for i := 0; i < n; i += frame {
		scale(b[i:i+2], p.left)
		scale(b[i+2:i+4], p.right)
	}
	p.pos += n
	return n, nil
}

// scale умножает один 16-битный отсчёт (little-endian) на k.
func scale(b []byte, k float64) {
	v := float64(int16(uint16(b[0]) | uint16(b[1])<<8))
	s := int32(v * k)
	// Ограничение на всякий случай: k не больше единицы, но клип здесь дешевле
	// переполнения, которое слышно как треск.
	if s > 32767 {
		s = 32767
	} else if s < -32768 {
		s = -32768
	}
	b[0], b[1] = byte(uint16(s)), byte(uint16(s)>>8)
}
