// Command iconnorm normalizes CraftPix icon / object / UI packs into a uniform
// layout with a manifest.json next to each pack — the same contract the biome
// manifests use (sheets + tile grid), extended with sprite rects.
//
// Source layout (as shipped by CraftPix, already stripped of PSD/ASEPRITE):
//
//	<pack>/*.png                — атласы вперемешку с отдельными иконками
//	<pack>/Separately/*.png     — те же иконки поштучно (есть не везде)
//	<pack>/*.tmx                — разметка Tiled: сетка и АНИМАЦИИ объектов
//
// Output layout (per pack):
//
//	<pack>/sheets/*.png         — атласы
//	<pack>/icons/*.png          — отдельные иконки (если пак их даёт)
//	<pack>/authoring/*.tmx      — разметка Tiled, source= переписан на ../sheets/
//	<pack>/manifest.json
//
// Геометрия берётся из данных, а не из названия пака: спрайты режутся по
// полностью пустым строкам/столбцам, анимации — из .tmx (шаг tileid между
// кадрами даёт размер кадра). Имена спрайтов настоящие там, где пак дал
// иконки поштучно: файл ищется в атласе точным совпадением пикселей.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"hash/fnv"
	"image"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
)

type Mapping struct {
	TileSize int    `json:"tileSize"`
	FPSms    int    `json:"defaultMs"`
	Packs    []Pack `json:"packs"`
}

type Pack struct {
	Dir        string `json:"dir"`
	ID         string `json:"id"`
	Category   string `json:"category"`
	SourcePack string `json:"source_pack"`
	// IconDirs — подпапки исходника, где лежат ОТДЕЛЬНЫЕ иконки, а не атласы.
	// Всё остальное считается атласом и уезжает в sheets/.
	IconDirs []string `json:"icon_dirs,omitempty"`
}

// ---------- манифест ----------

type Manifest struct {
	ID         string            `json:"id"`
	Category   string            `json:"category"`
	SourcePack string            `json:"source_pack"`
	TileSize   int               `json:"tile_size"`
	Icons      string            `json:"icons,omitempty"`
	Sheets     map[string]*Sheet `json:"sheets"`
}

type Size struct {
	W int `json:"w"`
	H int `json:"h"`
}

type Sheet struct {
	File      string `json:"file"`
	Size      Size   `json:"size"`
	Columns   int    `json:"columns,omitempty"`
	Tilecount int    `json:"tilecount,omitempty"`
	Anim      *Anim  `json:"anim,omitempty"`
	// SpritesFrom — откуда взялись рамки и имена: exact_match (сверка с
	// поштучными иконками пака), gap_slice (нарезка по пустым промежуткам).
	SpritesFrom string   `json:"sprites_from"`
	Sprites     []Sprite `json:"sprites"`
}

// Anim описывает, как лист сложен из кадров: кадр — это блок frame внутри
// листа, спрайты перечислены в координатах ПЕРВОГО кадра.
type Anim struct {
	Frames   int   `json:"frames"`
	Frame    Size  `json:"frame"`
	MS       int   `json:"ms"`
	Sequence []int `json:"sequence,omitempty"` // порядок кадров при проигрывании
}

type Sprite struct {
	ID string `json:"id"`
	X  int    `json:"x"`
	Y  int    `json:"y"`
	W  int    `json:"w"`
	H  int    `json:"h"`
}

var (
	assetsDir = flag.String("assets", "assets", "корень assets/")
	mapPath   = flag.String("mapping", "tools/iconnorm/mapping.json", "файл раскладки паков")
	dry       = flag.Bool("dry", false, "только показать, ничего не двигать")
	overlay   = flag.String("overlay", "", "после сборки нарисовать рамки спрайтов в указанную папку")
	contact   = flag.String("contact", "", "после сборки выложить спрайты сеткой с номерами в указанную папку")
)

func main() {
	flag.Parse()
	var m Mapping
	raw, err := os.ReadFile(*mapPath)
	if err != nil {
		fatal(err)
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		fatal(err)
	}
	if m.TileSize == 0 {
		m.TileSize = 16
	}
	if m.FPSms == 0 {
		m.FPSms = 150
	}
	for _, p := range m.Packs {
		if err := process(m, p); err != nil {
			fmt.Printf("ERR %s: %v\n", p.Dir, err)
		}
	}
}

func process(m Mapping, p Pack) error {
	dir := filepath.Join(*assetsDir, p.Dir)
	if _, err := os.Stat(dir); err != nil {
		return err
	}
	if err := layout(dir, p); err != nil {
		return err
	}
	if *dry {
		// В -dry файлы не переехали, собирать манифест не из чего.
		return nil
	}
	man, err := build(m, p, dir)
	if err != nil {
		return err
	}
	out := filepath.Join(dir, "manifest.json")
	buf, err := json.MarshalIndent(man, "", " ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(out, append(buf, '\n'), 0o644); err != nil {
		return err
	}
	if *overlay != "" {
		if err := overlayPack(dir, *overlay); err != nil {
			return err
		}
	}
	if *contact != "" {
		return contactPack(dir, *contact)
	}
	return nil
}

// ---------- раскладка ----------

func layout(dir string, p Pack) error {
	iconDir := map[string]bool{"icons": true}
	for _, d := range p.IconDirs {
		iconDir[strings.ToLower(d)] = true
	}
	var moves [][2]string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		parts := strings.Split(rel, string(os.PathSeparator))
		parent := ""
		if len(parts) > 1 {
			parent = strings.ToLower(parts[len(parts)-2])
		}
		base := filepath.Base(path)
		var dst string
		switch strings.ToLower(filepath.Ext(base)) {
		case ".png":
			if iconDir[parent] {
				dst = filepath.Join(dir, "icons", base)
			} else if parent != "sheets" {
				dst = filepath.Join(dir, "sheets", base)
			}
		case ".tmx", ".tsx":
			if parent != "authoring" {
				dst = filepath.Join(dir, "authoring", base)
			}
		case ".json":
			return nil // manifest.json
		}
		if dst != "" && dst != path {
			moves = append(moves, [2]string{path, dst})
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, mv := range moves {
		if _, err := os.Stat(mv[1]); err == nil {
			return fmt.Errorf("конфликт имён: %s уже существует", mv[1])
		}
		if *dry {
			fmt.Printf("  mv %s -> %s\n", mv[0], mv[1])
			continue
		}
		if err := os.MkdirAll(filepath.Dir(mv[1]), 0o755); err != nil {
			return err
		}
		if err := os.Rename(mv[0], mv[1]); err != nil {
			return err
		}
	}
	if *dry {
		return nil
	}
	if err := fixTMX(filepath.Join(dir, "authoring")); err != nil {
		return err
	}
	return pruneEmpty(dir)
}

// fixTMX переписывает ссылки на картинки: после переезда .tmx лежит в
// authoring/, а листы — в sheets/, голое имя файла больше не резолвится.
func fixTMX(dir string) error {
	ents, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range ents {
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext != ".tmx" && ext != ".tsx" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		s := string(raw)
		out := rewriteSources(s)
		if out != s {
			if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

func rewriteSources(s string) string {
	const marker = `source="`
	var b strings.Builder
	for {
		i := strings.Index(s, marker)
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		j := strings.Index(s[i+len(marker):], `"`)
		if j < 0 {
			b.WriteString(s)
			return b.String()
		}
		val := s[i+len(marker) : i+len(marker)+j]
		b.WriteString(s[:i+len(marker)])
		if strings.HasSuffix(strings.ToLower(val), ".png") && !strings.Contains(val, "/") {
			b.WriteString("../sheets/" + val)
		} else {
			b.WriteString(val)
		}
		b.WriteString(`"`)
		s = s[i+len(marker)+j+1:]
	}
}

func pruneEmpty(root string) error {
	for pass := 0; pass < 4; pass++ {
		var empty []string
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || !info.IsDir() || path == root {
				return nil
			}
			ents, err := os.ReadDir(path)
			if err == nil && len(ents) == 0 {
				empty = append(empty, path)
			}
			return nil
		})
		if len(empty) == 0 {
			return nil
		}
		for _, d := range empty {
			_ = os.Remove(d)
		}
	}
	return nil
}

// ---------- сборка манифеста ----------

func build(m Mapping, p Pack, dir string) (*Manifest, error) {
	man := &Manifest{
		ID:         p.ID,
		Category:   p.Category,
		SourcePack: p.SourcePack,
		TileSize:   m.TileSize,
		Sheets:     map[string]*Sheet{},
	}
	anims, err := tmxAnims(filepath.Join(dir, "authoring"))
	if err != nil {
		return nil, err
	}
	names, err := iconHashes(filepath.Join(dir, "icons"))
	if err != nil {
		return nil, err
	}
	if len(names) > 0 {
		man.Icons = "icons/"
	}
	ents, err := os.ReadDir(filepath.Join(dir, "sheets"))
	if err != nil {
		return nil, err
	}
	for _, e := range ents {
		if strings.ToLower(filepath.Ext(e.Name())) != ".png" {
			continue
		}
		path := filepath.Join(dir, "sheets", e.Name())
		img, err := decode(path)
		if err != nil {
			return nil, err
		}
		key := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		w, h := img.Bounds().Dx(), img.Bounds().Dy()
		sh := &Sheet{File: "sheets/" + e.Name(), Size: Size{w, h}}
		if w%m.TileSize == 0 && h%m.TileSize == 0 {
			sh.Columns = w / m.TileSize
			sh.Tilecount = (w / m.TileSize) * (h / m.TileSize)
		}
		// Кадры анимации знает только .tmx; без него лист считаем статичным.
		fw, fh := w, h
		if cands := anims[strings.ToLower(key)]; len(cands) > 0 {
			if a := pickAnim(cands, w, h); a != nil {
				sh.Anim = a.manifest(m.FPSms)
				fw, fh = sh.Anim.Frame.W, sh.Anim.Frame.H
			} else {
				fmt.Printf("  ! %s: ни одна раскладка кадров из tmx не сходится с размером листа\n", e.Name())
			}
		}
		mask, mw, _ := alphaMask(img)
		rects, specks := gapSlice(mask, mw, fw, fh)
		sh.SpritesFrom = "gap_slice"
		matched := 0
		for i, r := range rects {
			id := fmt.Sprintf("%s_%03d", key, i+1)
			if n, ok := names[hashRegion(img, r)]; ok {
				id, matched = n, matched+1
			}
			sh.Sprites = append(sh.Sprites, Sprite{id, r.x0, r.y0, r.x1 - r.x0, r.y1 - r.y0})
		}
		if matched > 0 && matched == len(rects) {
			sh.SpritesFrom = "exact_match"
		} else if matched > 0 {
			sh.SpritesFrom = "gap_slice+exact_match"
		}
		note := ""
		if sh.Anim != nil {
			note = fmt.Sprintf(" anim=%dx%dx%d", sh.Anim.Frame.W, sh.Anim.Frame.H, sh.Anim.Frames)
		}
		if matched > 0 {
			note += fmt.Sprintf(" named=%d", matched)
		}
		if specks > 0 {
			note += fmt.Sprintf(" (отброшено пылинок: %d)", specks)
		}
		fmt.Printf("  %-24s %4dx%-4d спрайтов=%-4d%s\n", e.Name(), w, h, len(sh.Sprites), note)
		man.Sheets[key] = sh
	}
	return man, nil
}

func decode(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	return img, err
}

// ---------- пиксели ----------

type rect struct{ x0, y0, x1, y1 int } // x1,y1 — за границей

func alphaMask(img image.Image) ([]bool, int, int) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	m := make([]bool, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			_, _, _, a := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			m[y*w+x] = a > 2048
		}
	}
	return m, w, h
}

// gapSlice режет прямоугольник fw x fh по полностью пустым строкам, затем
// каждую полосу — по пустым столбцам. stride — ширина всего листа.
// Возвращает рамки и число отброшенных пылинок (блик в 1-3 пикселя).
func gapSlice(m []bool, stride, fw, fh int) ([]rect, int) {
	at := func(x, y int) bool { return m[y*stride+x] }
	var out []rect
	specks := 0
	rows := runs(fh, func(y int) bool {
		for x := 0; x < fw; x++ {
			if at(x, y) {
				return true
			}
		}
		return false
	})
	for _, rb := range rows {
		cols := runs(fw, func(x int) bool {
			for y := rb[0]; y < rb[1]; y++ {
				if at(x, y) {
					return true
				}
			}
			return false
		})
		for _, cb := range cols {
			y0, y1, solid := rb[1], rb[0], 0
			for y := rb[0]; y < rb[1]; y++ {
				for x := cb[0]; x < cb[1]; x++ {
					if at(x, y) {
						solid++
						if y < y0 {
							y0 = y
						}
						if y+1 > y1 {
							y1 = y + 1
						}
					}
				}
			}
			if solid < 4 {
				specks++
				continue
			}
			out = append(out, rect{cb[0], y0, cb[1], y1})
		}
	}
	return out, specks
}

func runs(n int, occ func(int) bool) [][2]int {
	var out [][2]int
	s := -1
	for i := 0; i < n; i++ {
		if occ(i) {
			if s < 0 {
				s = i
			}
		} else if s >= 0 {
			out = append(out, [2]int{s, i})
			s = -1
		}
	}
	if s >= 0 {
		out = append(out, [2]int{s, n})
	}
	return out
}

func hashRegion(img image.Image, r rect) uint64 {
	b := img.Bounds()
	h := fnv.New64a()
	var buf [8]byte
	for y := r.y0; y < r.y1; y++ {
		for x := r.x0; x < r.x1; x++ {
			cr, cg, cb, ca := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			if ca == 0 {
				cr, cg, cb = 0, 0, 0
			}
			buf[0], buf[1] = byte(cr>>8), byte(cg>>8)
			buf[2], buf[3] = byte(cb>>8), byte(ca>>8)
			h.Write(buf[:4])
		}
	}
	// размер входит в хэш: иначе 1x4 и 4x1 из одинаковых пикселей совпадут
	fmt.Fprintf(h, "|%dx%d", r.x1-r.x0, r.y1-r.y0)
	return h.Sum64()
}

// iconHashes: хэш содержимого каждой поштучной иконки -> её имя.
func iconHashes(dir string) (map[uint64]string, error) {
	out := map[uint64]string{}
	ents, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	for _, e := range ents {
		if strings.ToLower(filepath.Ext(e.Name())) != ".png" {
			continue
		}
		img, err := decode(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		m, w, h := alphaMask(img)
		// иконка может лежать с полями — режем по содержимому, как атлас
		rs, _ := gapSlice(m, w, w, h)
		if len(rs) == 0 {
			continue
		}
		bb := rs[0]
		for _, r := range rs[1:] {
			bb = rect{min(bb.x0, r.x0), min(bb.y0, r.y0), max(bb.x1, r.x1), max(bb.y1, r.y1)}
		}
		out[hashRegion(img, bb)] = strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
	}
	return out, nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "iconnorm:", err)
	os.Exit(1)
}
