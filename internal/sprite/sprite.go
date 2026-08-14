// Package sprite — загрузка спрайт-пака по его manifest.json.
//
// Пак — это каталог вида assets/mobs/animals/fox: по PNG на анимацию плюс
// манифест с размером кадра, списком направлений и списком анимаций. Каждый
// PNG — сетка «строка = направление, столбец = кадр». Здесь эта сетка режется
// на anim.Clip'ы: по клипу на пару (анимация, направление).
//
// Формат одинаков у животных, врагов, боссов и героя, поэтому пакет ничего не
// знает про животных — он знает только про манифест.
package sprite

import (
	"encoding/json"
	"fmt"
	"image"
	"io/fs"
	"math"
	"path"
	"sort"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/vladislav/game/internal/anim"
	"github.com/vladislav/game/internal/assets"
	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/engine"
)

// Dir — направление взгляда. Привязка к строкам листа идёт по именам из
// манифеста, а не по номеру строки, — пак с другим порядком загрузится верно.
//
// Четыре стороны света идут первыми и в прежнем порядке: диагонали
// дописаны в хвост, чтобы паки и сохранения со старой нумерацией не поехали.
type Dir uint8

const (
	Down Dir = iota
	Up
	Left
	Right
	DownRight
	DownLeft
	UpRight
	UpLeft
	DirCount = 8
)

var dirNames = [DirCount]string{
	"down", "up", "left", "right",
	"down_right", "down_left", "up_right", "up_left",
}

// dirVecs — единичный вектор взгляда. Диагонали нормированы, иначе сектор
// удара по диагонали был бы длиннее, чем по стороне света.
var dirVecs = [DirCount]engine.Vec2{
	Down:      {X: 0, Y: 1},
	Up:        {X: 0, Y: -1},
	Left:      {X: -1, Y: 0},
	Right:     {X: 1, Y: 0},
	DownRight: {X: diag, Y: diag},
	DownLeft:  {X: -diag, Y: diag},
	UpRight:   {X: diag, Y: -diag},
	UpLeft:    {X: -diag, Y: -diag},
}

const diag = math.Sqrt2 / 2

// fallback — чем заменить направление, которого в паке нет. Сначала
// боковое: сбоку персонаж нарисован в развороте примерно на 45°, и из
// четырёх сторон света именно оно ближе всего к диагонали.
var fallback = [DirCount][]Dir{
	DownRight: {Right, Down},
	DownLeft:  {Left, Down},
	UpRight:   {Right, Up},
	UpLeft:    {Left, Up},
}

func (d Dir) String() string {
	if int(d) >= DirCount {
		return "?"
	}
	return dirNames[d]
}

// Vec — направление взгляда единичным вектором.
func (d Dir) Vec() engine.Vec2 {
	if int(d) >= DirCount {
		return dirVecs[Down]
	}
	return dirVecs[d]
}

// DirFrom переводит вектор движения в одну из четырёх сторон света: что
// больше по модулю, то и решает. Нулевой вектор — Down (поза «лицом к
// камере»). Для тех, у кого в паке четыре ряда, — то есть для всех мобов.
func DirFrom(v engine.Vec2) Dir {
	if v.X == 0 && v.Y == 0 {
		return Down
	}
	if math.Abs(v.X) > math.Abs(v.Y) {
		if v.X < 0 {
			return Left
		}
		return Right
	}
	if v.Y < 0 {
		return Up
	}
	return Down
}

// DirFrom8 переводит вектор движения в одно из восьми направлений: круг
// делится на октанты по 45°, границы посередине между направлениями.
//
// Отдельно от DirFrom, потому что переводить на восемь сторон всех разом
// нельзя: у мобов в паках четыре ряда, и диагональ у них подменялась бы
// боковым видом — движение вниз-чуть-вправо разворачивало бы их боком.
func DirFrom8(v engine.Vec2) Dir {
	if v.X == 0 && v.Y == 0 {
		return Down
	}
	// Октант от «вниз» против часовой стрелки; +0.5 сдвигает границы на
	// середину между направлениями, чтобы точное «вниз» не дрожало.
	oct := int(math.Floor(math.Atan2(-v.X, v.Y)/(math.Pi/4) + 0.5))
	return [8]Dir{Down, DownLeft, Left, UpLeft, Up, UpRight, Right, DownRight}[((oct%8)+8)%8]
}

// Size — размер кадра в пикселях.
type Size struct {
	W int `json:"w"`
	H int `json:"h"`
}

// Point — точка внутри кадра.
type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// Rect — рамка внутри кадра.
type Rect struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

// AnimDef — описание одной анимации в манифесте.
type AnimDef struct {
	File   string `json:"file"`
	Frames int    `json:"frames"`
	FPS    int    `json:"fps"`
	Loop   bool   `json:"loop"`
}

// Manifest — содержимое manifest.json спрайт-пака (генерируется assetnorm).
type Manifest struct {
	ID         string             `json:"id"`
	Name       string             `json:"name"`
	Type       string             `json:"type"`
	Category   string             `json:"category"`
	Frame      Size               `json:"frame"`
	Directions []string           `json:"directions"`
	Animations map[string]AnimDef `json:"animations"`
	// BBox и Anchor считает tools/spriteanchor по непрозрачным пикселям.
	// Могут отсутствовать (старый манифест) — тогда работают запасные значения.
	BBox   *Rect  `json:"bbox,omitempty"`
	Anchor *Point `json:"anchor,omitempty"`
}

// Pack — загруженный спрайт-пак: манифест плюс нарезанные клипы.
type Pack struct {
	Manifest
	Path  string // каталог пака внутри fs
	clips map[string][]*anim.Clip
	rowOf [DirCount]int // Dir → индекс строки в листе (-1 — направления нет)
}

// Load читает манифест из каталога dir и режет все его листы на клипы.
//
// Нарезка бесплатная: кадр — это SubImage исходного листа, то есть окно в тот
// же атлас, без копирования пикселей. Геометрия листа проверяется — лист, не
// сходящийся с манифестом, это ошибка загрузки, а не кривая картинка в игре.
func Load(l *assets.Loader, dir string) (*Pack, error) {
	mp := path.Join(dir, "manifest.json")
	b, err := fs.ReadFile(l.FS(), mp)
	if err != nil {
		return nil, fmt.Errorf("sprite: чтение %q: %w", mp, err)
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("sprite: разбор %q: %w", mp, err)
	}
	if m.Frame.W <= 0 || m.Frame.H <= 0 {
		return nil, fmt.Errorf("sprite: %q: не задан размер кадра", mp)
	}
	if len(m.Directions) == 0 {
		return nil, fmt.Errorf("sprite: %q: не заданы направления", mp)
	}

	p := &Pack{Manifest: m, Path: dir, clips: make(map[string][]*anim.Clip, len(m.Animations))}
	for i := range p.rowOf {
		p.rowOf[i] = -1
	}
	for row, name := range m.Directions {
		for d, dn := range dirNames {
			if name == dn {
				p.rowOf[d] = row
			}
		}
	}
	// Пак с четырьмя рядами обязан отвечать и на диагональный запрос:
	// решать, чем заменить недостающее, тут дешевле, чем в каждом
	// вызывающем. Мобы диагоналей не просят, но бестиарий и charview
	// перебирают все направления подряд.
	for d, chain := range fallback {
		if p.rowOf[d] >= 0 {
			continue
		}
		for _, alt := range chain {
			if p.rowOf[alt] >= 0 {
				p.rowOf[d] = p.rowOf[alt]
				break
			}
		}
	}

	rows := len(m.Directions)
	fw, fh := m.Frame.W, m.Frame.H
	for name, def := range m.Animations {
		if def.Frames <= 0 {
			return nil, fmt.Errorf("sprite: %q: анимация %q без кадров", dir, name)
		}
		sheet, err := l.Image(path.Join(dir, def.File))
		if err != nil {
			return nil, err
		}
		sb := sheet.Bounds()
		if sb.Dx() < def.Frames*fw || sb.Dy() < rows*fh {
			return nil, fmt.Errorf("sprite: %q/%s: лист %dx%d мал для %d кадров %dx%d в %d строк",
				dir, name, sb.Dx(), sb.Dy(), def.Frames, fw, fh, rows)
		}
		clips := make([]*anim.Clip, rows)
		for row := range rows {
			frames := make([]*ebiten.Image, def.Frames)
			for i := range frames {
				r := image.Rect(i*fw, row*fh, (i+1)*fw, (row+1)*fh).Add(sb.Min)
				frames[i] = sheet.SubImage(r).(*ebiten.Image)
			}
			clips[row] = &anim.Clip{Frames: frames, FrameTicks: frameTicks(def.FPS), Loop: def.Loop}
		}
		p.clips[name] = clips
	}
	return p, nil
}

// Clip — клип анимации name для направления d. nil, если такой анимации (или
// такого направления) в паке нет: наборы анимаций у паков рваные, и решение,
// чем заменить недостающее, принимает вызывающий, а не загрузчик.
func (p *Pack) Clip(name string, d Dir) *anim.Clip {
	cs, ok := p.clips[name]
	if !ok || int(d) >= DirCount {
		return nil
	}
	row := p.rowOf[d]
	if row < 0 || row >= len(cs) {
		return nil
	}
	return cs[row]
}

// Foot — точка опоры кадра: смещение внутри кадра, которое ставится на позицию
// существа в мире. Считается по пикселям (tools/spriteanchor), потому что кадр
// 32×32 не значит, что зверь занимает все 32 пикселя: сверху пустота, снизу тень
// под лапами. Если манифест старый и якоря нет — низ и середина кадра.
func (p *Pack) Foot() Point {
	if p.Anchor != nil {
		return *p.Anchor
	}
	return Point{X: p.Frame.W / 2, Y: p.Frame.H}
}

// Bounds — рамка непрозрачных пикселей в координатах кадра. Нужна для отсечения
// и для оценки размеров существа; при отсутствии — весь кадр.
func (p *Pack) Bounds() Rect {
	if p.BBox != nil {
		return *p.BBox
	}
	return Rect{W: p.Frame.W, H: p.Frame.H}
}

// Has сообщает, есть ли в паке анимация name.
func (p *Pack) Has(name string) bool { _, ok := p.clips[name]; return ok }

// Anims — имена анимаций пака в стабильном (алфавитном) порядке.
func (p *Pack) Anims() []string {
	names := make([]string, 0, len(p.clips))
	for n := range p.clips {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// frameTicks переводит fps манифеста в тики логики на кадр.
func frameTicks(fps int) int {
	if fps <= 0 {
		fps = 7
	}
	return max(config.TPS/fps, 1)
}
