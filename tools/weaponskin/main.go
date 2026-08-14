// Command weaponskin делает из пака персонажа с оружием такой же пак с другим
// оружием, перекрашивая слои оружия в другой материал.
//
// Зачем. Купленные паки CraftPix дают герою один клинок, а игре нужна лестница
// оружия: палка — ржавый клинок — что дальше. Рисовать второго героя целиком
// ради палки незачем: у пака есть Parts/, где оружие лежит отдельными слоями
// (sword_back, sword_front), и достаточно перекрасить их.
//
// Как. Слои складываются в лист дважды — как есть и с перекрашенным оружием, —
// и в готовый лист попадают только те пиксели, которые от перекраски
// изменились. Так лист остаётся ровно тем, что нарисовал художник: собственная
// сборка слоёв повторяет его не всюду (у death/hurt поверх лежит красная
// вспышка, которую пак сводит по-своему), и подменять ею весь лист было бы
// враньём.
//
// Цвет берётся не из исходного, а из трёхточечной рампы по яркости: сталь
// клинка светлая, рукоять тёмная, и одна и та же рампа делает из них ствол и
// более тёмный хват. Оттенок исходника при этом отбрасывается целиком —
// перекрашивать сталь «в коричневое» домножением бесполезно, она серая.
//
//	go run ./tools/weaponskin -src assets/character/male/sword -out assets/character/male/stick
//	go run ./tools/weaponskin -src assets/character/female/sword -out assets/character/female/stick
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

func main() {
	var (
		src   = flag.String("src", "assets/character/male/sword", "пак-источник")
		out   = flag.String("out", "assets/character/male/stick", "куда положить перекрашенный пак")
		layer = flag.String("layer", "sword", "подстрока в имени слоя, по которой узнаётся оружие")
		ramp  = flag.String("ramp", "#2b1c10,#6b4a27,#b98a4e", "рампа материала: тень, полутон, свет")
		trail = flag.String("trail", "#d8c39a", "цвет следа замаха (слой swing); пусто — не трогать")
	)
	flag.Parse()

	stops, err := parseRamp(*ramp)
	if err != nil {
		die(err)
	}
	var trailC *color.NRGBA
	if *trail != "" {
		c, err := parseHex(*trail)
		if err != nil {
			die(err)
		}
		trailC = &c
	}
	if err := skin(*src, *out, *layer, stops, trailC); err != nil {
		die(err)
	}
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "weaponskin:", err)
	os.Exit(1)
}

// part — один слой одной анимации.
type part struct {
	file  string // полный путь
	anim  string // idle, walk_attack, ...
	order int    // номер слоя в имени файла: порядок наложения
	layer string // shadow, body, sword_front, swing, ...
}

// partName разбирает имя вида "<пак>_<анимация><номер>_<слой>.png".
var partName = regexp.MustCompile(`^(.*?)(\d+)_(.+)$`)

func skin(src, out, weapon string, ramp [3]color.NRGBA, trail *color.NRGBA) error {
	man, err := os.ReadFile(filepath.Join(src, "manifest.json"))
	if err != nil {
		return err
	}
	var m struct {
		Animations map[string]struct {
			File string `json:"file"`
		} `json:"animations"`
	}
	if err := json.Unmarshal(man, &m); err != nil {
		return fmt.Errorf("манифест %s: %w", src, err)
	}

	srcID, outID := filepath.Base(src), filepath.Base(out)
	files, err := filepath.Glob(filepath.Join(src, "parts", "*.png"))
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("в %s/parts нет слоёв: перекрашивать нечего", src)
	}
	byAnim := map[string][]part{}
	for _, f := range files {
		p, ok := parsePart(f, srcID)
		if !ok {
			return fmt.Errorf("слой %s: имя не разбирается", filepath.Base(f))
		}
		byAnim[p.anim] = append(byAnim[p.anim], p)
	}

	if err := os.MkdirAll(filepath.Join(out, "parts"), 0o755); err != nil {
		return err
	}
	for anim, a := range m.Animations {
		parts := byAnim[anim]
		if len(parts) == 0 {
			return fmt.Errorf("у анимации %q нет слоёв в %s/parts", anim, src)
		}
		sort.Slice(parts, func(i, j int) bool { return parts[i].order < parts[j].order })

		sheet, err := loadNRGBA(filepath.Join(src, a.File))
		if err != nil {
			return err
		}
		plain, painted := image.NewNRGBA(sheet.Bounds()), image.NewNRGBA(sheet.Bounds())
		touched := false
		for _, p := range parts {
			img, err := loadNRGBA(p.file)
			if err != nil {
				return err
			}
			draw.Draw(plain, img.Bounds(), img, img.Bounds().Min, draw.Over)
			skinned := img
			switch {
			case strings.Contains(p.layer, weapon):
				skinned, touched = repaint(img, ramp), true
			case trail != nil && strings.Contains(p.layer, "swing"):
				skinned = tint(img, *trail)
			}
			draw.Draw(painted, img.Bounds(), skinned, img.Bounds().Min, draw.Over)
			if err := save(filepath.Join(out, "parts",
				outID+"_"+anim+strconv.Itoa(p.order)+"_"+strings.ReplaceAll(p.layer, weapon, outID)+".png"),
				skinned); err != nil {
				return err
			}
		}
		if !touched {
			return fmt.Errorf("у анимации %q нет слоя оружия (%q)", anim, weapon)
		}
		if err := save(filepath.Join(out, a.File), merge(sheet, plain, painted)); err != nil {
			return err
		}
	}
	// Манифест остаётся тем же: кадровка не изменилась, а bbox и anchor
	// считаются по непрозрачным пикселям — перекраска их не двигает.
	return os.WriteFile(filepath.Join(out, "manifest.json"),
		retitle(man, srcID, outID), 0o644)
}

// parsePart разбирает имя файла слоя.
func parsePart(f, pack string) (part, bool) {
	b := strings.TrimSuffix(filepath.Base(f), ".png")
	b = strings.TrimPrefix(b, pack+"_")
	mm := partName.FindStringSubmatch(b)
	if mm == nil {
		return part{}, false
	}
	n, err := strconv.Atoi(mm[2])
	if err != nil {
		return part{}, false
	}
	return part{file: f, anim: strings.TrimSuffix(mm[1], "_"), order: n, layer: mm[3]}, true
}

// merge переносит в лист художника те пиксели, которые изменила перекраска.
func merge(sheet, plain, painted *image.NRGBA) *image.NRGBA {
	out := image.NewNRGBA(sheet.Bounds())
	copy(out.Pix, sheet.Pix)
	for i := 0; i < len(plain.Pix); i += 4 {
		if plain.Pix[i] == painted.Pix[i] && plain.Pix[i+1] == painted.Pix[i+1] &&
			plain.Pix[i+2] == painted.Pix[i+2] && plain.Pix[i+3] == painted.Pix[i+3] {
			continue
		}
		copy(out.Pix[i:i+4], painted.Pix[i:i+4])
	}
	return out
}

// repaint красит непрозрачные пиксели по рампе, сохраняя яркость и прозрачность.
func repaint(src *image.NRGBA, ramp [3]color.NRGBA) *image.NRGBA {
	out := image.NewNRGBA(src.Bounds())
	for i := 0; i < len(src.Pix); i += 4 {
		a := src.Pix[i+3]
		if a == 0 {
			continue
		}
		l := lum(src.Pix[i], src.Pix[i+1], src.Pix[i+2])
		c := ramp2(ramp, l)
		out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = c.R, c.G, c.B, a
	}
	return out
}

// tint красит слой в один цвет, оставляя ему яркость и прозрачность: след
// замаха — это свет, а не материал, и рампа ему не годится.
func tint(src *image.NRGBA, c color.NRGBA) *image.NRGBA {
	out := image.NewNRGBA(src.Bounds())
	for i := 0; i < len(src.Pix); i += 4 {
		a := src.Pix[i+3]
		if a == 0 {
			continue
		}
		l := lum(src.Pix[i], src.Pix[i+1], src.Pix[i+2])
		out.Pix[i] = mul(c.R, l)
		out.Pix[i+1] = mul(c.G, l)
		out.Pix[i+2] = mul(c.B, l)
		out.Pix[i+3] = a
	}
	return out
}

func lum(r, g, b uint8) float64 {
	return (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)) / 255
}

func mul(v uint8, k float64) uint8 { return uint8(min(max(float64(v)*k, 0), 255)) }

// ramp2 — цвет рампы по яркости: тень → полутон → свет.
func ramp2(r [3]color.NRGBA, l float64) color.NRGBA {
	l = min(max(l, 0), 1)
	if l < 0.5 {
		return lerp(r[0], r[1], l*2)
	}
	return lerp(r[1], r[2], (l-0.5)*2)
}

func lerp(a, b color.NRGBA, k float64) color.NRGBA {
	f := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*k) }
	return color.NRGBA{f(a.R, b.R), f(a.G, b.G), f(a.B, b.B), 255}
}

// retitle правит в манифесте только имя пака: всё остальное описывает кадры,
// а кадры те же.
func retitle(man []byte, from, to string) []byte {
	var m map[string]any
	if json.Unmarshal(man, &m) != nil {
		return man
	}
	if s, ok := m["id"].(string); ok {
		m["id"] = strings.ReplaceAll(s, from, to)
	}
	if s, ok := m["name"].(string); ok && s == from {
		m["name"] = to
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return man
	}
	return append(b, '\n')
}

func parseRamp(s string) ([3]color.NRGBA, error) {
	var out [3]color.NRGBA
	p := strings.Split(s, ",")
	if len(p) != 3 {
		return out, fmt.Errorf("рампа %q: нужно три цвета через запятую", s)
	}
	for i, h := range p {
		c, err := parseHex(strings.TrimSpace(h))
		if err != nil {
			return out, err
		}
		out[i] = c
	}
	return out, nil
}

func parseHex(s string) (color.NRGBA, error) {
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		return color.NRGBA{}, fmt.Errorf("цвет %q не в виде #rrggbb", s)
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return color.NRGBA{}, fmt.Errorf("цвет %q: %w", s, err)
	}
	return color.NRGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}, nil
}

func loadNRGBA(p string) (*image.NRGBA, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p, err)
	}
	if n, ok := img.(*image.NRGBA); ok {
		return n, nil
	}
	n := image.NewNRGBA(img.Bounds())
	draw.Draw(n, img.Bounds(), img, img.Bounds().Min, draw.Src)
	return n, nil
}

func save(p string, img image.Image) error {
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
