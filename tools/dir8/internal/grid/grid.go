// Package grid читает текстовые сетки пикселей и кладёт их в кадр.
//
// Диагональные направления персонажа нарисованы вручную, но не в
// графическом редакторе, а текстом: один символ — один пиксель, файл —
// один слой кадра (тень, тело, меч, голова). Так правка адресуется
// конкретной клеткой, а не «перерисуй ещё раз», и весь спрайт лежит в
// git как текст, который видно в диффе.
package grid

import (
	"bufio"
	"fmt"
	"image"
	"image/color"
	"os"
	"strconv"
	"strings"
)

// FrameSize — сторона кадра пака female/sword.
const FrameSize = 64

// Palette — цвета пака. Своих оттенков тут нет намеренно: новый
// цвет выбился бы из растяжки, которой нарисован весь персонаж.
var Palette = map[byte]color.NRGBA{
	'o': {0x59, 0x31, 0x2E, 0xff}, // контур и тёмные волосы
	'h': {0x7E, 0x56, 0x55, 0xff}, // глубокая тень, одежда
	'e': {0xA5, 0x74, 0x6C, 0xff}, // тень волос
	's': {0xC0, 0x8A, 0x7B, 0xff}, // кожа в тени
	'S': {0xE9, 0xB3, 0x8E, 0xff}, // кожа
	'L': {0xFF, 0xCA, 0x96, 0xff}, // кожа на свету
	'g': {0x11, 0x0B, 0x00, 0xff}, // ресница
	'k': {0x00, 0x00, 0x00, 0xff}, // уголок глаза
	'i': {0x25, 0xAA, 0x53, 0xff}, // радужка
	'j': {0x22, 0x7D, 0x56, 0xff}, // радужка в тени
	'w': {0xD2, 0xDD, 0xE8, 0xff}, // блик глаза
	'1': {0x41, 0x31, 0x26, 0xff}, // рукоять тёмная
	'2': {0x82, 0x63, 0x44, 0xff}, // рукоять
	'3': {0xB6, 0x93, 0x4E, 0xff}, // гарда тёмная
	'4': {0xE9, 0xC7, 0x61, 0xff}, // гарда
	'5': {0x39, 0x38, 0x45, 0xff}, // клинок в тени
	'6': {0x61, 0x63, 0x79, 0xff},
	'7': {0x71, 0x78, 0x8F, 0xff},
	'8': {0x74, 0x85, 0x97, 0xff},
	'9': {0x84, 0xA4, 0xBA, 0xff},
	'a': {0x8F, 0xAD, 0xC2, 0xff},
	'b': {0xAB, 0xE1, 0xF2, 0xff}, // блик клинка
	'W': {0xE1, 0xEA, 0xF2, 0xff}, // росчерк замаха
	'M': {0x7D, 0x2E, 0x3E, 0xff}, // тёмная кромка вспышки
	',': {0x28, 0x29, 0x3F, 0x59}, // тень на земле
	'R': {0xC7, 0x15, 0x2F, 0x4C}, // вспышка попадания, слабая
	'r': {0xC7, 0x15, 0x2F, 0x99}, // она же в полную силу
	'D': {0x9C, 0x35, 0x35, 0xff}, // тёмно-красный на голове в кадрах урона
}

// UsePalette подмешивает в палитру цвета конкретного пака.
//
// Мужские паки нарисованы другой растяжкой кожи и с синими глазами
// вместо зелёных, а силуэт головы у них тот же. Раз цвет привязан к
// паку, а не к сетке, одна и та же нарисованная голова годится обоим.
// Формат строки: "<символ> #RRGGBB" или "#RRGGBBAA"; отсутствие файла
// не ошибка — значит, пак рисуется палитрой по умолчанию.
func UsePalette(path string) error {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || len(fields[0]) != 1 {
			return fmt.Errorf("%s:%d: жду строку вида \"<символ> #RRGGBB[AA]\", а не %q", path, n, line)
		}
		c, err := parseHex(fields[1])
		if err != nil {
			return fmt.Errorf("%s:%d: %w", path, n, err)
		}
		Palette[fields[0][0]] = c
	}
	return sc.Err()
}

func parseHex(s string) (color.NRGBA, error) {
	v := strings.TrimPrefix(s, "#")
	if len(v) != 6 && len(v) != 8 {
		return color.NRGBA{}, fmt.Errorf("цвет %q: жду 6 или 8 шестнадцатеричных цифр", s)
	}
	n, err := strconv.ParseUint(v, 16, 32)
	if err != nil {
		return color.NRGBA{}, fmt.Errorf("цвет %q: %w", s, err)
	}
	if len(v) == 6 {
		return color.NRGBA{R: uint8(n >> 16), G: uint8(n >> 8), B: uint8(n), A: 0xff}, nil
	}
	return color.NRGBA{R: uint8(n >> 24), G: uint8(n >> 16), B: uint8(n >> 8), A: uint8(n)}, nil
}

// Layer — один слой кадра: куда класть окно и что в нём.
type Layer struct {
	Origin image.Point
	Rows   []string
}

// Read читает сетку. Первая непустая строка вида "@ x y" задаёт левый
// верхний угол окна в координатах кадра, строки с '#' — комментарии.
func Read(path string) (Layer, error) {
	f, err := os.Open(path)
	if err != nil {
		return Layer{}, err
	}
	defer f.Close()

	var l Layer
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), " \t\r")
		switch {
		case strings.HasPrefix(line, "#"):
		case strings.HasPrefix(line, "@"):
			p := strings.Fields(line)
			if len(p) != 3 {
				return Layer{}, fmt.Errorf("%s: строка @ должна быть вида \"@ x y\", а не %q", path, line)
			}
			x, err := strconv.Atoi(p[1])
			if err != nil {
				return Layer{}, fmt.Errorf("%s: %w", path, err)
			}
			y, err := strconv.Atoi(p[2])
			if err != nil {
				return Layer{}, fmt.Errorf("%s: %w", path, err)
			}
			l.Origin = image.Pt(x, y)
		default:
			l.Rows = append(l.Rows, line)
		}
	}
	return l, sc.Err()
}

// Draw рисует слой поверх картинки со сдвигом. Точка и пробел —
// прозрачно, остальные символы обязаны быть в палитре.
//
// Полупрозрачные цвета смешиваются с тем, что уже лежит под ними, а не
// затирают его: вспышка попадания — это плёнка поверх силуэта, и если
// её просто записать, она вырежет из персонажа дыру.
func (l Layer) Draw(dst *image.NRGBA, shift image.Point, name string) error {
	o := l.Origin.Add(shift)
	for y, line := range l.Rows {
		for x := 0; x < len(line); x++ {
			ch := line[x]
			if ch == '.' || ch == ' ' {
				continue
			}
			c, ok := Palette[ch]
			if !ok {
				return fmt.Errorf("%s:%d:%d — цвета %q нет в палитре", name, y, x, ch)
			}
			px, py := o.X+x, o.Y+y
			if c.A < 0xff {
				c = blend(c, dst.NRGBAAt(px, py))
			}
			dst.SetNRGBA(px, py, c)
		}
	}
	return nil
}

// blend кладёт src на dst по обычному «source over».
func blend(src, dst color.NRGBA) color.NRGBA {
	sa := float64(src.A) / 255
	da := float64(dst.A) / 255 * (1 - sa)
	a := sa + da
	if a == 0 {
		return color.NRGBA{}
	}
	mix := func(s, d uint8) uint8 {
		return uint8((float64(s)*sa + float64(d)*da) / a)
	}
	return color.NRGBA{
		R: mix(src.R, dst.R),
		G: mix(src.G, dst.G),
		B: mix(src.B, dst.B),
		A: uint8(a*255 + 0.5),
	}
}
