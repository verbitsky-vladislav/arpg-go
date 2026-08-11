// Package assets — загрузка графических ресурсов (PNG) в *ebiten.Image.
// Источник — абстрактная fs.FS: в проде это вшитая в бинарник embed.FS
// (см. main.go), в тестах может быть fstest.MapFS. Загрузчик кэширует
// результаты, поэтому один и тот же файл декодируется один раз.
package assets

import (
	"fmt"
	"image"
	_ "image/png" // регистрирует PNG-декодер для image.Decode
	"io/fs"
	"sort"
	"strings"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
)

// Loader грузит изображения из fs.FS и кэширует их. Потокобезопасен.
type Loader struct {
	fsys fs.FS
	mu   sync.Mutex
	imgs map[string]*ebiten.Image
	seqs map[string][]*ebiten.Image
}

// NewLoader создаёт загрузчик поверх файловой системы fsys.
func NewLoader(fsys fs.FS) *Loader {
	return &Loader{
		fsys: fsys,
		imgs: map[string]*ebiten.Image{},
		seqs: map[string][]*ebiten.Image{},
	}
}

// Image загружает одиночный PNG по пути внутри fs (с кэшированием).
func (l *Loader) Image(path string) (*ebiten.Image, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if img, ok := l.imgs[path]; ok {
		return img, nil
	}
	img, err := l.decode(path)
	if err != nil {
		return nil, err
	}
	l.imgs[path] = img
	return img, nil
}

// Frames загружает кадры анимации из каталога dir: все файлы вида frame_*.png,
// отсортированные по имени (frame_00, frame_01, ... — отсюда нулевые префиксы).
// Результат кэшируется по каталогу.
func (l *Loader) Frames(dir string) ([]*ebiten.Image, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if seq, ok := l.seqs[dir]; ok {
		return seq, nil
	}
	entries, err := fs.ReadDir(l.fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("assets: чтение каталога %q: %w", dir, err)
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if !e.IsDir() && strings.HasPrefix(n, "frame_") && strings.HasSuffix(n, ".png") {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, fmt.Errorf("assets: в %q нет кадров frame_*.png", dir)
	}
	seq := make([]*ebiten.Image, 0, len(names))
	for _, n := range names {
		img, err := l.decode(dir + "/" + n)
		if err != nil {
			return nil, err
		}
		seq = append(seq, img)
	}
	l.seqs[dir] = seq
	return seq, nil
}

func (l *Loader) decode(path string) (*ebiten.Image, error) {
	f, err := l.fsys.Open(path)
	if err != nil {
		return nil, fmt.Errorf("assets: открытие %q: %w", path, err)
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("assets: декодирование %q: %w", path, err)
	}
	return ebiten.NewImageFromImage(src), nil
}
