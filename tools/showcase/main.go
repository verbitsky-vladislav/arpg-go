// Команда showcase собирает витринные картинки для README: героя по восьми
// направлениям, строй врагов, зверинец, каталог вещей и два вида карты — весь
// остров и кусок в игровом масштабе.
//
// Всё, что движется в игре, движется и на витрине: спрайты выкладываются
// анимациями (GIF), а не одним кадром. Кадр — это не спрайт-пак, а его обложка;
// по нему не видно ни походки, ни замаха, ради которых пак и покупался.
//
// Картинки собираются из тех же ассетов и тем же генератором, что и игра,
// поэтому витрина не врёт: переименовали пак или сломали манифест — прогон
// падает, а не рисует пустые клетки.
//
//	go run ./tools/showcase [-out docs/media] [-seed 3] [-size 160]
//
// Подписи латиницей нарочно: единственный встроенный в Go растровый шрифт
// (basicfont) кириллицы не знает, а тащить ttf ради витрины незачем — имена
// на английском в таблицах и так лежат (title.en).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/vladislav/game/internal/worldgen"
)

// Цвета витрины: тёмный фон под пиксель-арт, чуть светлее — клетка.
var (
	colBG    = color.RGBA{0x14, 0x17, 0x1c, 0xff}
	colCell  = color.RGBA{0x1d, 0x22, 0x29, 0xff}
	colLabel = color.RGBA{0x8a, 0x96, 0xa3, 0xff}
	colTitle = color.RGBA{0xd8, 0xe0, 0xe8, 0xff}
)

// Задержка кадра GIF в сотых долях секунды. 14 ≈ 7 кадров в секунду — тот же
// темп, который стоит в манифестах паков (поле fps).
const frameDelay = 14

func main() {
	out := flag.String("out", filepath.Join("docs", "media"), "каталог для картинок")
	seed := flag.Uint64("seed", 3, "сид карты для витрины")
	size := flag.Int("size", 160, "сторона карты в тайлах")
	flag.Parse()

	if err := os.MkdirAll(*out, 0o755); err != nil {
		fatal(err)
	}
	steps := []struct {
		name string
		fn   func(string) error
	}{
		{"hero", heroGIF},
		{"enemies", enemyGIF},
		{"animals", animalGIF},
		{"items", itemSheet},
	}
	for _, s := range steps {
		if err := s.fn(*out); err != nil {
			fatal(fmt.Errorf("%s: %w", s.name, err))
		}
	}
	if err := worldShots(*out, *seed, *size); err != nil {
		fatal(fmt.Errorf("world: %w", err))
	}
}

// ── спрайт-паки ───────────────────────────────────────────────────────────

// pack — то немногое из manifest.json, что нужно витрине: размер кадра,
// порядок направлений и список листов. Полный разбор живёт в internal/sprite,
// но он тянет ebiten, а витрине нужен обычный image.Image.
type pack struct {
	Frame      struct{ W, H int } `json:"frame"`
	Directions []string           `json:"directions"`
	Animations map[string]struct {
		File   string `json:"file"`
		Frames int    `json:"frames"`
	} `json:"animations"`

	dir    string
	sheets map[string]image.Image
}

func loadPack(dir string) (*pack, error) {
	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, err
	}
	p := &pack{dir: dir, sheets: map[string]image.Image{}}
	if err := json.Unmarshal(b, p); err != nil {
		return nil, fmt.Errorf("%s: %w", dir, err)
	}
	if p.Frame.W == 0 || p.Frame.H == 0 {
		return nil, fmt.Errorf("%s: пустой размер кадра", dir)
	}
	return p, nil
}

// clip выбирает первый существующий клип из списка предпочтений.
func (p *pack) clip(names ...string) (string, bool) {
	for _, n := range names {
		if _, ok := p.Animations[n]; ok {
			return n, true
		}
	}
	return "", false
}

// frames режет ряд листа: строка — направление, столбцы — кадры клипа.
//
// Пустые кадры выбрасываются. Ряд бывает короче заявленного в манифесте числа
// кадров (у покоя «вверх» их четыре из двенадцати), и витрине незачем моргать
// этой дырой — но и молча дорисовывать её она не станет: пустой ряд целиком
// это ошибка.
func (p *pack) frames(clip, dir string) ([]image.Image, error) {
	a, ok := p.Animations[clip]
	if !ok {
		return nil, fmt.Errorf("%s: нет клипа %s", p.dir, clip)
	}
	row := 0
	for i, d := range p.Directions {
		if d == dir {
			row = i
			break
		}
	}
	sheet, err := p.sheet(a.File)
	if err != nil {
		return nil, err
	}
	sub, ok := sheet.(interface {
		SubImage(image.Rectangle) image.Image
	})
	if !ok {
		return nil, fmt.Errorf("%s: лист не режется", a.File)
	}
	var out []image.Image
	for i := range a.Frames {
		r := image.Rect(i*p.Frame.W, row*p.Frame.H, (i+1)*p.Frame.W, (row+1)*p.Frame.H)
		if !r.In(sheet.Bounds()) {
			return nil, fmt.Errorf("%s/%s: кадр %d/%s вне листа", p.dir, a.File, i, dir)
		}
		f := sub.SubImage(r)
		if !blank(f) {
			out = append(out, f)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: %s/%s пустой", p.dir, clip, dir)
	}
	return out, nil
}

func (p *pack) sheet(file string) (image.Image, error) {
	if img, ok := p.sheets[file]; ok {
		return img, nil
	}
	f, err := os.Open(filepath.Join(p.dir, file))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s/%s: %w", p.dir, file, err)
	}
	p.sheets[file] = img
	return img, nil
}

func blank(img image.Image) bool {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a > 0 {
				return false
			}
		}
	}
	return true
}

// ── герой ─────────────────────────────────────────────────────────────────

// heroGIF — мужской пак с мечом: строка на клип, столбец на направление, и всё
// это в движении. Четыре диагонали дорисованы (tools/dir8) — на неподвижной
// картинке отличить их от рисованных четырёх нельзя, в движении видно сразу.
func heroGIF(outDir string) error {
	p, err := loadPack(filepath.Join("assets", "character", "male", "sword"))
	if err != nil {
		return err
	}
	clips := []string{"idle", "walk", "run", "attack"}
	dirs := p.Directions
	if len(dirs) == 0 {
		return fmt.Errorf("пак без направлений")
	}

	const k = 2 // масштаб рендера у игры такой же
	cw, ch := p.Frame.W*k, p.Frame.H*k
	const padL, padT, gap = 76, 26, 4

	strips := make([][][]image.Image, len(clips))
	n := 1
	for ri, cl := range clips {
		strips[ri] = make([][]image.Image, len(dirs))
		for ci, d := range dirs {
			fr, err := p.frames(cl, d)
			if err != nil {
				return err
			}
			strips[ri][ci] = fr
			n = max(n, len(fr))
		}
	}

	w := padL + len(dirs)*(cw+gap) + gap
	h := padT + len(clips)*(ch+gap) + gap
	out := make([]*image.RGBA, n)
	for f := range n {
		dst := newCanvas(w, h)
		for ci, d := range dirs {
			text(dst, padL+ci*(cw+gap)+(cw-textW(d))/2, padT-8, d, colLabel)
		}
		for ri, cl := range clips {
			y := padT + ri*(ch+gap)
			text(dst, gap+4, y+ch/2+4, cl, colTitle)
			for ci := range dirs {
				fr := strips[ri][ci]
				x := padL + ci*(cw+gap)
				fillRect(dst, image.Rect(x, y, x+cw, y+ch), colCell)
				blit(dst, scaleNN(fr[f%len(fr)], k), x, y)
			}
		}
		out[f] = dst
	}
	return writeGIF(filepath.Join(outDir, "hero.gif"), out)
}

// ── враги и звери ─────────────────────────────────────────────────────────

type titled struct {
	Art   string `json:"art"`
	Title struct {
		EN string `json:"en"`
	} `json:"title"`
}

// enemyGIF — по одной особи на тип, средний тир (t2), покой.
func enemyGIF(outDir string) error {
	var doc struct {
		Types map[string]struct {
			titled
			Tiers map[string]json.RawMessage `json:"tiers"`
		} `json:"types"`
	}
	if err := readJSON(filepath.Join("assets", "mobs", "enemies", "enemies.json"), &doc); err != nil {
		return err
	}
	keys := sortedKeys(doc.Types)
	cells := make([]cell, 0, len(keys))
	for _, k := range keys {
		t := doc.Types[k]
		tiers := sortedKeys(t.Tiers)
		if len(tiers) == 0 {
			return fmt.Errorf("%s: без тиров", k)
		}
		tier := tiers[len(tiers)/2]
		for _, want := range tiers {
			if want == "t2" {
				tier = want
			}
		}
		c, err := mobCell(filepath.Join("assets", "mobs", "enemies", t.Art, tier), t.Title.EN)
		if err != nil {
			return err
		}
		cells = append(cells, c)
	}
	return writeGIF(filepath.Join(outDir, "enemies.gif"), grid(cells, 6))
}

// animalGIF — зверинец: все виды из species.json.
func animalGIF(outDir string) error {
	var doc struct {
		Species map[string]titled `json:"species"`
	}
	if err := readJSON(filepath.Join("assets", "mobs", "animals", "species.json"), &doc); err != nil {
		return err
	}
	keys := sortedKeys(doc.Species)
	cells := make([]cell, 0, len(keys))
	for _, k := range keys {
		s := doc.Species[k]
		c, err := mobCell(filepath.Join("assets", "mobs", "animals", s.Art), s.Title.EN)
		if err != nil {
			return err
		}
		cells = append(cells, c)
	}
	return writeGIF(filepath.Join(outDir, "animals.gif"), grid(cells, 6))
}

func mobCell(dir, label string) (cell, error) {
	p, err := loadPack(dir)
	if err != nil {
		return cell{}, err
	}
	cl, ok := p.clip("idle", "walk", "run")
	if !ok {
		return cell{}, fmt.Errorf("%s: нечего показать", dir)
	}
	fr, err := p.frames(cl, "down")
	if err != nil {
		return cell{}, err
	}
	k := 2
	if p.Frame.W > 80 {
		k = 1
	}
	out := cell{label: label, frames: make([]image.Image, len(fr))}
	for i, f := range fr {
		out.frames[i] = scaleNN(f, k)
	}
	return out, nil
}

// ── вещи ──────────────────────────────────────────────────────────────────

// itemSheet — каталог предметов иконками; единственная витрина без движения,
// потому что иконка и в игре не анимируется. Иконка живёт не файлом, а
// прямоугольником в листе пака (tools/iconnorm), поэтому нужен ещё манифест пака.
func itemSheet(outDir string) error {
	var doc struct {
		Items []struct {
			ID   string `json:"id"`
			Pack string `json:"pack"`
			Icon string `json:"icon"`
		} `json:"items"`
	}
	if err := readJSON(filepath.Join("assets", "items", "items.json"), &doc); err != nil {
		return err
	}
	packs := map[string]*iconPack{}
	cells := make([]cell, 0, len(doc.Items))
	for _, it := range doc.Items {
		ip, ok := packs[it.Pack]
		if !ok {
			var err error
			if ip, err = loadIconPack(filepath.Join("assets", it.Pack)); err != nil {
				return err
			}
			packs[it.Pack] = ip
		}
		img, err := ip.icon(it.Icon)
		if err != nil {
			return fmt.Errorf("%s: %w", it.ID, err)
		}
		cells = append(cells, cell{frames: []image.Image{scaleNN(img, 3)}})
	}
	return writePNG(filepath.Join(outDir, "items.png"), grid(cells, 10)[0])
}

type iconPack struct {
	Sheets map[string]struct {
		File    string `json:"file"`
		Sprites []struct {
			ID         string `json:"id"`
			X, Y, W, H int
		} `json:"sprites"`
	} `json:"sheets"`

	dir   string
	cache map[string]image.Image
}

func loadIconPack(dir string) (*iconPack, error) {
	p := &iconPack{dir: dir, cache: map[string]image.Image{}}
	if err := readJSON(filepath.Join(dir, "manifest.json"), p); err != nil {
		return nil, err
	}
	return p, nil
}

// icon находит спрайт по ссылке из items.json. Ссылка бывает двух видов:
// "Icons_012" — искать по всем листам пака, "Gui_icons_items/Sword1_2" — лист
// назван явно (одинаковые имена спрайтов в разных листах пака легальны).
func (p *iconPack) icon(id string) (image.Image, error) {
	sheet := ""
	if i := strings.IndexByte(id, '/'); i >= 0 {
		sheet, id = id[:i], id[i+1:]
	}
	for name, sh := range p.Sheets {
		if sheet != "" && name != sheet {
			continue
		}
		for _, s := range sh.Sprites {
			if s.ID != id {
				continue
			}
			img, ok := p.cache[name]
			if !ok {
				f, err := os.Open(filepath.Join(p.dir, sh.File))
				if err != nil {
					return nil, err
				}
				img, err = png.Decode(f)
				f.Close()
				if err != nil {
					return nil, err
				}
				p.cache[name] = img
			}
			sub, ok := img.(interface {
				SubImage(image.Rectangle) image.Image
			})
			if !ok {
				return nil, fmt.Errorf("%s: лист не режется", sh.File)
			}
			return sub.SubImage(image.Rect(s.X, s.Y, s.X+s.W, s.Y+s.H)), nil
		}
	}
	return nil, fmt.Errorf("иконки %s нет в паке %s", id, p.dir)
}

// ── карта ─────────────────────────────────────────────────────────────────

// worldShots рисует остров целиком (уменьшённым) и кусок в игровом масштабе.
func worldShots(outDir string, seed uint64, size int) error {
	m, err := worldgen.LoadManifest(filepath.Join("assets", "biomes", "forest"))
	if err != nil {
		return err
	}
	atlas, err := worldgen.NewAtlasSet(m)
	if err != nil {
		return err
	}
	mp := worldgen.Generate(m, seed, size)
	img := worldgen.RenderMap(mp, atlas, m.RenderScale)

	full := img.Bounds().Dx()
	k := max(1, full/1024)
	if err := writePNG(filepath.Join(outDir, "world.png"), downscale(img, k)); err != nil {
		return err
	}
	// Кусок берём размером с окно игры (1280×720 при масштабе 2) и выбираем
	// самый разнообразный: пустая поляна витрину не украшает.
	return writePNG(filepath.Join(outDir, "world-detail.png"), bestCrop(img, 1280, 720))
}

// bestCrop ищет окно с наибольшим числом разных цветов — так в кадр попадают
// берег, обрыв и деревья, а не однородная трава.
func bestCrop(src image.Image, w, h int) *image.RGBA {
	b := src.Bounds()
	if b.Dx() < w || b.Dy() < h {
		w, h = b.Dx(), b.Dy()
	}
	best, bestScore := b.Min, -1
	for y := b.Min.Y; y+h <= b.Max.Y; y += 160 {
		for x := b.Min.X; x+w <= b.Max.X; x += 160 {
			seen := map[uint32]struct{}{}
			for sy := y; sy < y+h; sy += 8 {
				for sx := x; sx < x+w; sx += 8 {
					r, g, bl, _ := src.At(sx, sy).RGBA()
					key := (r>>12)<<8 | (g>>12)<<4 | (bl >> 12)
					seen[key] = struct{}{}
				}
			}
			if len(seen) > bestScore {
				best, bestScore = image.Pt(x, y), len(seen)
			}
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(dst, dst.Bounds(), src, best, draw.Src)
	return dst
}

// ── композиция ────────────────────────────────────────────────────────────

// cell — одна клетка витрины: кадры её анимации и подпись. Один кадр — это
// законная анимация из одного кадра (иконки вещей).
type cell struct {
	frames []image.Image
	label  string
}

// grid раскладывает клетки в сетку и отдаёт кадры анимации целиком: клетка
// одна на всех по размеру самой крупной, подпись — под картинкой.
//
// Клипы у разных существ разной длины, и подгонять их друг под друга (общее
// кратное) незачем: кадров вышло бы под сотню ради того, чтобы гоблин моргал
// с крысой в такт. Каждая клетка крутит свой клип по кругу, длина витрины —
// самый длинный из них.
func grid(cells []cell, cols int) []*image.RGBA {
	cw, chh, labeled, n := 0, 0, false, 1
	for _, c := range cells {
		for _, f := range c.frames {
			cw = max(cw, f.Bounds().Dx())
			chh = max(chh, f.Bounds().Dy())
		}
		labeled = labeled || c.label != ""
		cw = max(cw, textW(c.label))
		n = max(n, len(c.frames))
	}
	const pad, gap = 6, 6
	lab := 0
	if labeled {
		lab = 14
	}
	cw += pad * 2
	ch := chh + pad*2 + lab
	rows := (len(cells) + cols - 1) / cols

	out := make([]*image.RGBA, n)
	for f := range n {
		dst := newCanvas(gap+cols*(cw+gap), gap+rows*(ch+gap))
		for i, c := range cells {
			x := gap + (i%cols)*(cw+gap)
			y := gap + (i/cols)*(ch+gap)
			fillRect(dst, image.Rect(x, y, x+cw, y+ch), colCell)
			img := c.frames[f%len(c.frames)]
			b := img.Bounds()
			blit(dst, img, x+(cw-b.Dx())/2, y+pad+(chh-b.Dy())/2)
			if c.label != "" {
				text(dst, x+(cw-textW(c.label))/2, y+ch-pad, c.label, colLabel)
			}
		}
		out[f] = dst
	}
	return out
}

func newCanvas(w, h int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	fillRect(dst, dst.Bounds(), colBG)
	return dst
}

func fillRect(dst *image.RGBA, r image.Rectangle, c color.Color) {
	draw.Draw(dst, r, image.NewUniform(c), image.Point{}, draw.Src)
}

func blit(dst *image.RGBA, src image.Image, x, y int) {
	b := src.Bounds()
	draw.Draw(dst, image.Rect(x, y, x+b.Dx(), y+b.Dy()), src, b.Min, draw.Over)
}

// scaleNN — увеличение «ближайшим соседом»: пиксель-арт другого не терпит.
func scaleNN(src image.Image, k int) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx()*k, b.Dy()*k))
	for y := range b.Dy() * k {
		for x := range b.Dx() * k {
			dst.Set(x, y, src.At(b.Min.X+x/k, b.Min.Y+y/k))
		}
	}
	return dst
}

// downscale — уменьшение усреднением по блоку k×k: для обзорной карты честнее
// прореживания, иначе с острова пропадают тропы шириной в тайл.
func downscale(src image.Image, k int) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx()/k, b.Dy()/k))
	for y := range dst.Bounds().Dy() {
		for x := range dst.Bounds().Dx() {
			var sr, sg, sb, sa uint32
			for oy := range k {
				for ox := range k {
					r, g, bl, a := src.At(b.Min.X+x*k+ox, b.Min.Y+y*k+oy).RGBA()
					sr, sg, sb, sa = sr+r, sg+g, sb+bl, sa+a
				}
			}
			n := uint32(k * k * 257)
			dst.Set(x, y, color.RGBA{uint8(sr / n), uint8(sg / n), uint8(sb / n), uint8(sa / n)})
		}
	}
	return dst
}

func text(dst draw.Image, x, y int, s string, c color.Color) {
	d := &font.Drawer{Dst: dst, Src: image.NewUniform(c), Face: basicfont.Face7x13, Dot: fixed.P(x, y)}
	d.DrawString(s)
}

func textW(s string) int { return len(s) * 7 }

// ── запись ────────────────────────────────────────────────────────────────

// writeGIF собирает кадры в зацикленную анимацию.
//
// Палитра общая на весь GIF и снимается с самих кадров: пиксель-арт живёт в
// двух-трёх сотнях цветов, и готовые палитры (Plan9, WebSafe) на нём заметно
// врут — трава становится кислотной. Цветов больше 256 — берутся самые частые,
// остальные сводятся к ближайшему; поиск ближайшего кэшируется по цвету,
// иначе на каждый кадр приходится полмиллиона переборов палитры.
func writeGIF(path string, frames []*image.RGBA) error {
	if len(frames) == 0 {
		return fmt.Errorf("%s: нечего писать", path)
	}
	pal := palette(frames)
	g := &gif.GIF{LoopCount: 0}
	idx := map[uint32]uint8{}
	for _, f := range frames {
		b := f.Bounds()
		p := image.NewPaletted(b, pal)
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				c := f.RGBAAt(x, y)
				key := uint32(c.R)<<16 | uint32(c.G)<<8 | uint32(c.B)
				i, ok := idx[key]
				if !ok {
					i = uint8(pal.Index(c))
					idx[key] = i
				}
				p.SetColorIndex(x, y, i)
			}
		}
		g.Image = append(g.Image, p)
		g.Delay = append(g.Delay, frameDelay)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := gif.EncodeAll(f, g); err != nil {
		return err
	}
	b := frames[0].Bounds()
	fmt.Printf("%s (%d×%d, кадров %d)\n", path, b.Dx(), b.Dy(), len(frames))
	return nil
}

// palette — до 256 самых частых цветов кадров.
func palette(frames []*image.RGBA) color.Palette {
	count := map[color.RGBA]int{}
	for _, f := range frames {
		b := f.Bounds()
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				c := f.RGBAAt(x, y)
				c.A = 0xff // фон непрозрачный, прозрачность в GIF не нужна
				count[c]++
			}
		}
	}
	uniq := make([]color.RGBA, 0, len(count))
	for c := range count {
		uniq = append(uniq, c)
	}
	sort.Slice(uniq, func(i, j int) bool {
		if count[uniq[i]] != count[uniq[j]] {
			return count[uniq[i]] > count[uniq[j]]
		}
		return less(uniq[i], uniq[j])
	})
	if len(uniq) > 256 {
		uniq = uniq[:256]
	}
	pal := make(color.Palette, len(uniq))
	for i, c := range uniq {
		pal[i] = c
	}
	return pal
}

// less — порядок на цветах, чтобы палитра не зависела от обхода карты:
// одинаково частые цвета обязаны ложиться в одном и том же порядке, иначе
// перегенерация витрины меняет байты файла на ровном месте.
func less(a, b color.RGBA) bool {
	if a.R != b.R {
		return a.R < b.R
	}
	if a.G != b.G {
		return a.G < b.G
	}
	return a.B < b.B
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return err
	}
	b := img.Bounds()
	fmt.Printf("%s (%d×%d)\n", path, b.Dx(), b.Dy())
	return nil
}

// ── мелочи ────────────────────────────────────────────────────────────────

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func sortedKeys[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "showcase:", err)
	os.Exit(1)
}
