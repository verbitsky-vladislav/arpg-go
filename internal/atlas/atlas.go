// Package atlas — чтение иконочных паков (manifest.json от tools/iconnorm).
//
// Отличие от internal/sprite: там пак — это одно существо с направлениями и
// анимациями, кадр берётся по (анимация, направление, номер). Здесь пак — это
// набор листов, на которых лежат сотни независимых картинок, и достать нужно
// одну по имени: "Icons_012".
//
// Нарезка бесплатная: спрайт — это SubImage листа, окно в тот же атлас без
// копирования пикселей.
package atlas

import (
	"encoding/json"
	"fmt"
	"image"
	"io/fs"
	"path"
	"sort"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/vladislav/game/internal/assets"
)

// Size — размер в пикселях.
type Size struct {
	W int `json:"w"`
	H int `json:"h"`
}

// Sprite — рамка одной картинки внутри листа.
type Sprite struct {
	ID string `json:"id"`
	X  int    `json:"x"`
	Y  int    `json:"y"`
	W  int    `json:"w"`
	H  int    `json:"h"`
}

// Anim — как лист сложен из кадров. Кадр это блок Frame; спрайты листа
// перечислены в координатах ПЕРВОГО кадра, остальные лежат со сдвигом.
type Anim struct {
	Frames   int   `json:"frames"`
	Frame    Size  `json:"frame"`
	MS       int   `json:"ms"`
	Sequence []int `json:"sequence,omitempty"`
}

// Sheet — один лист пака.
type Sheet struct {
	File      string   `json:"file"`
	Size      Size     `json:"size"`
	Columns   int      `json:"columns,omitempty"`
	Tilecount int      `json:"tilecount,omitempty"`
	Anim      *Anim    `json:"anim,omitempty"`
	From      string   `json:"sprites_from"`
	Sprites   []Sprite `json:"sprites"`
}

// Manifest — содержимое manifest.json иконочного пака.
type Manifest struct {
	ID         string            `json:"id"`
	Category   string            `json:"category"`
	SourcePack string            `json:"source_pack"`
	TileSize   int               `json:"tile_size"`
	Icons      string            `json:"icons,omitempty"`
	Sheets     map[string]*Sheet `json:"sheets"`
}

// Pack — загруженный иконочный пак.
type Pack struct {
	Manifest
	Path string

	imgs map[string]*ebiten.Image // лист -> картинка
	// index — ключ "лист/id": он уникален всегда. bare — тот же спрайт по
	// голому id, но только пока id не встретился на двух листах: в icons_16
	// один набор иконок лежит и в общем атласе, и в двух витринах магазина,
	// и «дай Sword1_2» там означает три разные картинки.
	index map[string]ref
	bare  map[string][]ref
}

type ref struct {
	sheet  string
	sprite Sprite
}

func (r ref) key() string { return r.sheet + "/" + r.sprite.ID }

// Load читает манифест пака из каталога dir и подтягивает его листы.
//
// Геометрия проверяется: спрайт, вылезающий за край своего листа, — это ошибка
// загрузки, а не обрезанная картинка в игре.
func Load(l *assets.Loader, dir string) (*Pack, error) {
	mp := path.Join(dir, "manifest.json")
	b, err := fs.ReadFile(l.FS(), mp)
	if err != nil {
		return nil, fmt.Errorf("atlas: чтение %q: %w", mp, err)
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("atlas: разбор %q: %w", mp, err)
	}
	p := &Pack{
		Manifest: m,
		Path:     dir,
		imgs:     make(map[string]*ebiten.Image, len(m.Sheets)),
		index:    map[string]ref{},
		bare:     map[string][]ref{},
	}
	for name, sh := range m.Sheets {
		img, err := l.Image(path.Join(dir, sh.File))
		if err != nil {
			return nil, fmt.Errorf("atlas: %q: лист %q: %w", dir, name, err)
		}
		b := img.Bounds()
		if b.Dx() != sh.Size.W || b.Dy() != sh.Size.H {
			return nil, fmt.Errorf("atlas: %q: лист %q: манифест обещает %dx%d, в файле %dx%d",
				dir, name, sh.Size.W, sh.Size.H, b.Dx(), b.Dy())
		}
		p.imgs[name] = img
		for _, s := range sh.Sprites {
			if s.W <= 0 || s.H <= 0 || s.X < 0 || s.Y < 0 || s.X+s.W > b.Dx() || s.Y+s.H > b.Dy() {
				return nil, fmt.Errorf("atlas: %q: спрайт %q вылезает за лист %q", dir, s.ID, name)
			}
			r := ref{sheet: name, sprite: s}
			if _, dup := p.index[r.key()]; dup {
				return nil, fmt.Errorf("atlas: %q: спрайт %q объявлен дважды на листе %q", dir, s.ID, name)
			}
			p.index[r.key()] = r
			p.bare[s.ID] = append(p.bare[s.ID], r)
		}
	}
	return p, nil
}

// find ищет спрайт по id — либо по полному ключу "лист/id", либо по голому id,
// если он в паке один. Ambiguous говорит, что имя есть, но их несколько:
// вызывающему нужно сказать не «нет такого», а «уточни лист».
func (p *Pack) find(id string) (r ref, ok bool, ambiguous []string) {
	if r, ok := p.index[id]; ok {
		return r, true, nil
	}
	rs := p.bare[id]
	switch len(rs) {
	case 0:
		return ref{}, false, nil
	case 1:
		return rs[0], true, nil
	}
	names := make([]string, 0, len(rs))
	for _, r := range rs {
		names = append(names, r.key())
	}
	sort.Strings(names)
	return ref{}, false, names
}

// Sprite отдаёт картинку спрайта по id ("Icons_012" или "Gui_icons_items/Sword1_2").
func (p *Pack) Sprite(id string) (*ebiten.Image, bool) {
	r, ok, _ := p.find(id)
	if !ok {
		return nil, false
	}
	img := p.imgs[r.sheet]
	return img.SubImage(image.Rect(r.sprite.X, r.sprite.Y,
		r.sprite.X+r.sprite.W, r.sprite.Y+r.sprite.H)).(*ebiten.Image), true
}

// Ambiguous — списком полные ключи, если голое имя id встречается не раз.
func (p *Pack) Ambiguous(id string) []string {
	_, _, amb := p.find(id)
	return amb
}

// Frame отдаёт спрайт id на кадре n анимированного листа. Для неанимированного
// листа кадр 0 — это сам спрайт, остальных нет.
func (p *Pack) Frame(id string, n int) (*ebiten.Image, bool) {
	r, ok, _ := p.find(id)
	if !ok {
		return nil, false
	}
	sh := p.Sheets[r.sheet]
	dx, dy := 0, 0
	if sh.Anim != nil {
		if n < 0 || n >= sh.Anim.Frames {
			return nil, false
		}
		// Кадры лежат либо в ряд, либо друг под другом — что именно, видно по
		// тому, какая сторона кадра совпадает со стороной листа.
		if sh.Anim.Frame.W < sh.Size.W {
			dx = n * sh.Anim.Frame.W
		} else {
			dy = n * sh.Anim.Frame.H
		}
	} else if n != 0 {
		return nil, false
	}
	s := r.sprite
	img := p.imgs[r.sheet]
	return img.SubImage(image.Rect(s.X+dx, s.Y+dy, s.X+dx+s.W, s.Y+dy+s.H)).(*ebiten.Image), true
}

// IDs — полные ключи всех спрайтов пака ("лист/id"), по алфавиту.
func (p *Pack) IDs() []string {
	out := make([]string, 0, len(p.index))
	for id := range p.index {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
