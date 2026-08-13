package main

// atlas.go — загрузка атласов тайлсета и нарезка отдельных 16px тайлов по id.
// Локальный id тайла нумеруется слева-направо, сверху-вниз (как в Tiled):
// row = id / columns, col = id % columns.

import (
	"fmt"
	"image"
	_ "image/png"
	"os"
	"path/filepath"
)

// AtlasSet — все атласы биома, загруженные как image.Image, + порядок листов.
type AtlasSet struct {
	m        *Manifest
	tileSize int
	imgs     map[string]image.Image // по имени листа
	order    []string               // фиксированный порядок листов → индекс SheetRef
	firstgid map[string]int
	cache    map[string]*image.RGBA // кэш нарезанных тайлов "sheet#id"
}

// NewAtlasSet загружает все листы из манифеста и раскладывает firstgid по порядку.
func NewAtlasSet(m *Manifest) (*AtlasSet, error) {
	a := &AtlasSet{
		m:        m,
		tileSize: m.TileSize,
		imgs:     map[string]image.Image{},
		firstgid: map[string]int{},
		cache:    map[string]*image.RGBA{},
	}
	// стабильный порядок: по возрастанию, как в Forest.tmx исходно
	a.order = sheetOrder(m)
	gid := 1
	for _, name := range a.order {
		sh := m.Sheets[name]
		img, err := loadImage(filepath.Join(m.dir, sh.File))
		if err != nil {
			return nil, fmt.Errorf("лист %q: %w", name, err)
		}
		a.imgs[name] = img
		a.firstgid[name] = gid
		gid += sh.Count
	}
	return a, nil
}

// sheetOrder — детерминированный порядок листов (по имени).
func sheetOrder(m *Manifest) []string {
	names := make([]string, 0, len(m.Sheets))
	for n := range m.Sheets {
		names = append(names, n)
	}
	// простая сортировка вставками, чтобы не тянуть sort (хотя он в stdlib) —
	// но sort в stdlib, используем его
	sortStrings(names)
	return names
}

func loadImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("декод %s: %w", path, err)
	}
	return img, nil
}

// sheetIndex — индекс листа в порядке order (для SparseTile.Sheet и SheetRef).
func (a *AtlasSet) sheetIndex(name string) int {
	for i, n := range a.order {
		if n == name {
			return i
		}
	}
	return -1
}

// tileRect — прямоугольник тайла id в атласе листа.
func (a *AtlasSet) tileRect(sheet string, id int) image.Rectangle {
	cols := a.m.Sheets[sheet].Columns
	if cols <= 0 {
		cols = 1
	}
	cx := (id % cols) * a.tileSize
	cy := (id / cols) * a.tileSize
	return image.Rect(cx, cy, cx+a.tileSize, cy+a.tileSize)
}

// Tile возвращает 16px тайл листа как RGBA (с кэшированием).
func (a *AtlasSet) Tile(sheet string, id int) *image.RGBA {
	key := sheet + "#" + itoa(id)
	if t, ok := a.cache[key]; ok {
		return t
	}
	src := a.imgs[sheet]
	dst := image.NewRGBA(image.Rect(0, 0, a.tileSize, a.tileSize))
	if src != nil {
		r := a.tileRect(sheet, id)
		b := src.Bounds()
		for y := 0; y < a.tileSize; y++ {
			for x := 0; x < a.tileSize; x++ {
				sx, sy := b.Min.X+r.Min.X+x, b.Min.Y+r.Min.Y+y
				if sx < b.Max.X && sy < b.Max.Y {
					dst.Set(x, y, src.At(sx, sy))
				}
			}
		}
	}
	a.cache[key] = dst
	return dst
}
