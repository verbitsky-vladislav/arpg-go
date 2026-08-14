package audio

import (
	"encoding/binary"
	"io"
	"testing"
)

// stereo собирает PCM из пар отсчётов.
func stereo(frames ...[2]int16) []byte {
	b := make([]byte, 0, len(frames)*frame)
	for _, f := range frames {
		b = binary.LittleEndian.AppendUint16(b, uint16(f[0]))
		b = binary.LittleEndian.AppendUint16(b, uint16(f[1]))
	}
	return b
}

func samples(b []byte) [][2]int16 {
	out := make([][2]int16, 0, len(b)/frame)
	for i := 0; i+frame <= len(b); i += frame {
		out = append(out, [2]int16{
			int16(binary.LittleEndian.Uint16(b[i:])),
			int16(binary.LittleEndian.Uint16(b[i+2:])),
		})
	}
	return out
}

func TestPanReaderScalesChannelsIndependently(t *testing.T) {
	src := stereo([2]int16{1000, 1000}, [2]int16{-2000, -2000})
	r := newPanReader(src, 1, 0.5)
	buf := make([]byte, len(src))
	n, err := r.Read(buf)
	if err != nil {
		t.Fatalf("чтение: %v", err)
	}
	if n != len(src) {
		t.Fatalf("прочитано %d байт из %d", n, len(src))
	}
	got := samples(buf[:n])
	want := [][2]int16{{1000, 500}, {-2000, -1000}}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("кадр %d: получено %v, ожидалось %v", i, got[i], want[i])
		}
	}
}

func TestPanReaderDoesNotSplitFrames(t *testing.T) {
	// Отдать половину кадра — значит поменять каналы местами на всём остатке
	// звука. Читатель обязан отдавать только целые кадры.
	src := stereo([2]int16{100, 200}, [2]int16{300, 400})
	r := newPanReader(src, 1, 1)
	buf := make([]byte, 6) // полтора кадра
	n, err := r.Read(buf)
	if err != nil {
		t.Fatalf("чтение: %v", err)
	}
	if n%frame != 0 {
		t.Fatalf("отдано %d байт — не целое число кадров", n)
	}
	if n != frame {
		t.Fatalf("отдано %d байт, ожидался ровно один кадр", n)
	}
}

func TestPanReaderReachesEOF(t *testing.T) {
	src := stereo([2]int16{1, 1})
	r := newPanReader(src, 1, 1)
	buf := make([]byte, 64)
	if n, err := r.Read(buf); n != frame || err != nil {
		t.Fatalf("первое чтение: %d байт, %v", n, err)
	}
	if _, err := r.Read(buf); err != io.EOF {
		t.Fatalf("после конца данных ожидался io.EOF, получено %v", err)
	}
}

func TestPanReaderSilencesMutedChannel(t *testing.T) {
	src := stereo([2]int16{32767, 32767})
	r := newPanReader(src, 0, 1)
	buf := make([]byte, frame)
	if _, err := r.Read(buf); err != nil {
		t.Fatalf("чтение: %v", err)
	}
	got := samples(buf)[0]
	if got[0] != 0 {
		t.Errorf("заглушенный канал звучит: %d", got[0])
	}
	if got[1] != 32767 {
		t.Errorf("открытый канал ослаблен: %d", got[1])
	}
}
