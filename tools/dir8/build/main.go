// build достраивает листы пака до восьми направлений.
//
// Четыре исходных ряда пака (down, left, right, up) берутся как есть,
// две диагонали — down_right и up_right — собираются из текстовых сеток
// в dir8/<клип>/se и dir8/<клип>/ne, ещё две получаются их отражением.
// Порядок рядов: исходные четыре, потом down_right, down_left,
// up_right, up_left — так старые индексы направлений остаются валидными.
//
// Собираются те клипы, для которых есть каталог сеток; остальные
// пропускаются. Число кадров берётся из manifest.json, покадровые
// сдвиги головы и оружия — из poses.txt рядом с сетками (их пишет
// tools/dir8/trace). Ничего про конкретный пак здесь не зашито.
//
// usage: go run ./tools/dir8/build <packDir>
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vladislav/game/tools/dir8/internal/grid"
)

const frame = grid.FrameSize

// manifest — manifest.json пака. Поля перечислены в том же порядке, в
// каком их пишет assetnorm, и всё лишнее протаскивается как есть:
// сборка правит только directions и имена файлов анимаций.
type manifest struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Type       string          `json:"type"`
	Category   string          `json:"category"`
	SourcePack string          `json:"source_pack,omitempty"`
	Frame      json.RawMessage `json:"frame"`
	Directions []string        `json:"directions"`
	Animations map[string]anim `json:"animations"`
	BBox       json.RawMessage `json:"bbox,omitempty"`
	Anchor     json.RawMessage `json:"anchor,omitempty"`
}

type anim struct {
	File   string `json:"file"`
	Frames int    `json:"frames"`
	FPS    int    `json:"fps"`
	Loop   bool   `json:"loop"`
}

// dirs8 — порядок рядов в собранном листе.
var dirs8 = []string{
	"down", "left", "right", "up",
	"down_right", "down_left", "up_right", "up_left",
}

// pose — покадровые сдвиги слоёв, которые не перерисовываются, а ездят:
// голова (она нарисована одна на направление) и оружие в тех клипах, где
// оно не машет.
type pose struct{ headDX, headDY, swordDX, swordDY int }

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: build <packDir>")
		os.Exit(2)
	}
	pack := os.Args[1]
	check(grid.UsePalette(filepath.Join(pack, "dir8", "palette.txt")))

	var m manifest
	b, err := os.ReadFile(filepath.Join(pack, "manifest.json"))
	check(err)
	check(json.Unmarshal(b, &m))

	names := make([]string, 0, len(m.Animations))
	for n := range m.Animations {
		names = append(names, n)
	}
	sort.Strings(names)

	built := 0
	for _, n := range names {
		dir := filepath.Join(pack, "dir8", n)
		if _, err := os.Stat(filepath.Join(dir, "se")); err != nil {
			continue // сеток для этого клипа ещё нет
		}
		check(buildClip(pack, n, m.Animations[n].Frames))
		built++
	}
	if built == 0 {
		check(fmt.Errorf("в %s нет ни одного каталога сеток dir8/<клип>/se", pack))
	}
	check(retarget(pack, &m, b, built == len(names)))
}

// retarget переводит манифест на восьмирядные листы.
//
// Переключается весь пак разом или никак: directions в манифесте один
// на все анимации, и четырёхрядный лист рядом с восемью объявленными
// направлениями просто не загрузится. Пока собраны не все клипы,
// манифест остаётся на четырёх.
func retarget(pack string, m *manifest, orig []byte, all bool) error {
	if !all {
		fmt.Printf("  манифест не трогаю: собраны не все клипы пака\n")
		return nil
	}
	m.Directions = dirs8
	for name, a := range m.Animations {
		a.File = name + "8.png"
		m.Animations[name] = a
	}
	box, err := contentBox(pack, m)
	if err != nil {
		return err
	}
	m.BBox = box
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	if bytes.Equal(out, orig) {
		return nil
	}
	p := filepath.Join(pack, "manifest.json")
	if err := os.WriteFile(p, out, 0o644); err != nil {
		return err
	}
	fmt.Println("перевёл на восемь направлений", p)
	return nil
}

// buildClip дописывает к четырём рядам исходного листа четыре
// диагональных и кладёт результат рядом под именем <клип>8.png.
func buildClip(pack, clip string, frames int) error {
	base, err := loadPNG(filepath.Join(pack, clip+".png"))
	if err != nil {
		return err
	}
	if base.Bounds().Dy() != 4*frame {
		return fmt.Errorf("%s.png: ожидал 4 ряда направлений, а высота %d", clip, base.Bounds().Dy())
	}
	if got := base.Bounds().Dx(); got != frames*frame {
		return fmt.Errorf("%s.png: манифест обещает %d кадров, а ширина %d", clip, frames, got)
	}

	dir := filepath.Join(pack, "dir8", clip)
	// Спереди оружие перед корпусом, со спины — за ним: в паке это
	// разные слои, front у бокового ряда и back у верхнего.
	se, err := renderRow(filepath.Join(dir, "se"), frames, false)
	if err != nil {
		return err
	}
	ne, err := renderRow(filepath.Join(dir, "ne"), frames, true)
	if err != nil {
		return err
	}

	rows := []*image.NRGBA{se, mirror(se), ne, mirror(ne)}
	out := image.NewNRGBA(image.Rect(0, 0, frames*frame, (4+len(rows))*frame))
	draw.Draw(out, base.Bounds(), base, image.Point{}, draw.Src)
	for i, r := range rows {
		at := image.Rect(0, (4+i)*frame, frames*frame, (5+i)*frame)
		draw.Draw(out, at, r, image.Point{}, draw.Src)
	}

	dst := filepath.Join(pack, clip+"8.png")
	if err := writePNG(dst, out); err != nil {
		return err
	}
	fmt.Println("написал", dst)
	return nil
}

// renderRow собирает кадры направления в один ряд.
// swordBehind кладёт оружие под корпус — так оно выглядит со спины.
func renderRow(dir string, frames int, swordBehind bool) (*image.NRGBA, error) {
	poses, err := readPoses(filepath.Join(dir, "poses.txt"))
	if err != nil {
		return nil, err
	}
	if len(poses) < frames {
		return nil, fmt.Errorf("%s/poses.txt: %d строк на %d кадров", dir, len(poses), frames)
	}
	shadow, err := grid.Read(filepath.Join(dir, "shadow.txt"))
	if err != nil {
		return nil, err
	}
	head, err := grid.Read(filepath.Join(dir, "head.txt"))
	if err != nil {
		return nil, err
	}
	// В ходьбе и беге оружие одно на весь клип и ездит сдвигом; в
	// атаках оно машет, там на каждый кадр своя форма в sword<N>.txt.
	oneSword, noSharedSword := grid.Read(filepath.Join(dir, "sword.txt"))

	type step struct {
		layer grid.Layer
		shift image.Point
		name  string
	}
	read := func(name string) (step, error) {
		l, err := grid.Read(filepath.Join(dir, name))
		return step{l, image.Point{}, name}, err
	}

	row := image.NewNRGBA(image.Rect(0, 0, frames*frame, frame))
	for i := range frames {
		p := poses[i]
		bodyStep, err := read(fmt.Sprintf("body%d.txt", i))
		if err != nil {
			return nil, err
		}
		swordStep := step{oneSword, image.Pt(p.swordDX, p.swordDY), "sword.txt"}
		haveSword := noSharedSword == nil
		if !haveSword {
			if s, err := read(fmt.Sprintf("sword%d.txt", i)); err == nil {
				swordStep, haveSword = s, true
			}
		}

		// Порядок слоёв тот же, что в parts исходного пака: тень под
		// ногами, корпус, оружие с той или другой его стороны, голова,
		// поверх — росчерк замаха и вспышка попадания, если они есть.
		steps := []step{{shadow, image.Point{}, "shadow.txt"}}
		switch {
		case !haveSword:
			steps = append(steps, bodyStep)
		case swordBehind:
			steps = append(steps, swordStep, bodyStep)
		default:
			steps = append(steps, bodyStep, swordStep)
		}
		// Обычно голова одна на направление и ездит сдвигом; но если для
		// кадра лежит своя (упавшая в смерти, зажмуренная в уроне) —
		// берётся она, уже на своём месте.
		headStep := step{head, image.Pt(p.headDX, p.headDY), "head.txt"}
		if s, err := read(fmt.Sprintf("head%d.txt", i)); err == nil {
			headStep = s
		}
		steps = append(steps, headStep)
		for _, extra := range []string{"swing%d.txt", "red%d.txt"} {
			if s, err := read(fmt.Sprintf(extra, i)); err == nil {
				steps = append(steps, s)
			}
		}

		f := image.NewNRGBA(image.Rect(0, 0, frame, frame))
		for _, l := range steps {
			if err := l.layer.Draw(f, l.shift, filepath.Join(dir, l.name)); err != nil {
				return nil, err
			}
		}
		draw.Draw(row, image.Rect(i*frame, 0, (i+1)*frame, frame), f, image.Point{}, draw.Src)
	}
	return row, nil
}

// readPoses читает poses.txt: по строке на кадр, четыре числа.
func readPoses(path string) ([]pose, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []pose
	for i, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var p pose
		if _, err := fmt.Sscan(line, &p.headDX, &p.headDY, &p.swordDX, &p.swordDY); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, i+1, err)
		}
		out = append(out, p)
	}
	return out, nil
}

// contentBox — рамка непрозрачных пикселей в координатах кадра, общая на
// весь пак. Пересчитывается здесь, потому что диагонали могут её
// расширить, а spriteanchor до паков персонажа не доходит: он обходит
// только мобов. Отсечение по устаревшей рамке срезало бы край спрайта.
func contentBox(pack string, m *manifest) (json.RawMessage, error) {
	var fr struct{ W, H int }
	if err := json.Unmarshal(m.Frame, &fr); err != nil {
		return nil, err
	}
	minX, minY, maxX, maxY := fr.W, fr.H, -1, -1
	for name := range m.Animations {
		im, err := loadPNG(filepath.Join(pack, name+"8.png"))
		if err != nil {
			return nil, err
		}
		b := im.Bounds()
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				if im.NRGBAAt(x, y).A == 0 {
					continue
				}
				fx, fy := x%fr.W, y%fr.H
				minX, minY = min(minX, fx), min(minY, fy)
				maxX, maxY = max(maxX, fx), max(maxY, fy)
			}
		}
	}
	if maxX < 0 {
		return nil, fmt.Errorf("%s: все листы пустые", pack)
	}
	return json.Marshal(struct {
		X int `json:"x"`
		Y int `json:"y"`
		W int `json:"w"`
		H int `json:"h"`
	}{minX, minY, maxX - minX + 1, maxY - minY + 1})
}

// mirror отражает ряд по горизонтали внутри каждого кадра. Левый и
// правый ряды пака связаны именно так, рамка ложится сама на себя.
func mirror(row *image.NRGBA) *image.NRGBA {
	b := row.Bounds()
	out := image.NewNRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			f := x / frame
			mx := f*frame + (frame - 1 - x%frame)
			out.SetNRGBA(mx, y, row.NRGBAAt(x, y))
		}
	}
	return out
}

func loadPNG(p string) (*image.NRGBA, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	src, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	dst := image.NewNRGBA(src.Bounds())
	draw.Draw(dst, src.Bounds(), src, src.Bounds().Min, draw.Src)
	return dst, nil
}

func writePNG(p string, im image.Image) error {
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, im)
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "dir8:", err)
		os.Exit(1)
	}
}
