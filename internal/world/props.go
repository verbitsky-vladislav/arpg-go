package world

// props.go — объекты карты (деревья, камни, руины) в рантайме.
//
// В отличие от тайловых слоёв объекты НЕ запекаются в холст: они обязаны
// уходить за спину персонажу и зверям, а значит рисуются в общем проходе по
// глубине. Ключ глубины — sort_y объекта, то есть клетка, на которой он стоит,
// а не верх его спрайта: у дерева крона на четыре клетки выше ствола.
//
// Часть объектов покачивается на ветру: у них File — вертикальная полоса кадров
// высотой в один спрайт каждый, кадр переключается по тикам карты.

import (
	"image"
	"path"
	"sort"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/vladislav/game/internal/assets"
	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/engine"
	"github.com/vladislav/game/internal/worldgen"
)

// prop — объект, готовый к отрисовке.
type prop struct {
	img   *ebiten.Image
	x, y  float64 // левый верхний угол спрайта в мире
	depth float64 // ключ сортировки: низ объекта
	w, h  int     // размер кадра в пикселях
	// baseX/baseY — точка, которой объект касается земли, в мире. Барьер физики
	// стоит здесь же; по ней и сверяются картинка с физикой.
	baseX, baseY float64
	frames       int // кадров покачивания (0/1 — статичный)
	ticks        int // тиков на кадр
}

// loadProps грузит спрайты объектов и раскладывает их по глубине.
func (m *Map) loadProps(l *assets.Loader, mv *worldgen.MapV1) error {
	root := path.Join("biomes", mv.Biome)
	for _, p := range mv.Props {
		img, err := l.Image(path.Join(root, p.File))
		if err != nil {
			// Объект без картинки — не повод рушить забег: он просто не виден,
			// а его физика уже в сетке. Молча пропускаем.
			continue
		}
		w, h := p.W*m.ts, p.H*m.ts
		// СДВИГ НА ПОЛТАЙЛА. Объект приходит из карты в клеточных координатах, а
		// содержимое клетки (x,y) НАРИСОВАНО в квадрате [x*ts+ts/2, x*ts+3*ts/2)
		// — этот сдвиг dual-grid запечён в сетку физики (buildNav, map_format.md).
		// Без поправки картинка и барьер расходятся: барьер оказывается на тайл
		// правее и на полтайла ниже ствола и касается спрайта углом.
		//
		// Опора берётся из РИСУНКА (p.Base), а не из края холста: до низа холста
		// рисунок почти никогда не доходит — у лежащего бревна он кончается на
		// 46 px выше и на 5 px левее центра. По краю холста барьер уезжал ниже и
		// правее самого бревна.
		cx, cy := p.X+p.Anchor[0], p.Y+p.Anchor[1]-1 // клетка, на которой стоит
		baseX := float64(cx*m.ts + m.ts)             // центр нарисованной клетки
		baseY := float64(cy*m.ts + m.ts + m.ts/2)    // её низ
		pr := prop{
			img: img,
			x:   baseX - float64(p.Base[0]),
			y:   baseY - float64(p.Base[1]),
			// глубина — низ нарисованной клетки, на которой объект стоит: так
			// дерево на ряд выше игрока уходит за спину, а на ряд ниже закрывает его
			depth:  baseY,
			w:      w,
			h:      h,
			frames: p.Frames,
			baseX:  baseX,
			baseY:  baseY,
		}
		if p.Frames > 1 {
			pr.ticks = max(p.MS*config.TPS/1000, 1)
		}
		m.props = append(m.props, pr)
	}
	sort.SliceStable(m.props, func(i, j int) bool { return m.props[i].depth < m.props[j].depth })
	return nil
}

// PropCount — сколько объектов на карте.
func (m *Map) PropCount() int { return len(m.props) }

// PropX — левый край спрайта объекта i в мире.
func (m *Map) PropX(i int) float64 { return m.props[i].x }

// PropBase — точка, которой объект i касается земли (мировые пиксели). Здесь же
// стоит его барьер в сетке физики.
func (m *Map) PropBase(i int) (x, y float64) { return m.props[i].baseX, m.props[i].baseY }

// PropDepth — ключ глубины объекта i (мировые пиксели).
func (m *Map) PropDepth(i int) float64 { return m.props[i].depth }

// DrawProp рисует объект i. Вызывается игровым слоем в общем проходе по
// глубине, вперемежку с героем и зверями.
func (m *Map) DrawProp(dst *ebiten.Image, cam *engine.Camera, i int) {
	p := m.props[i]
	left := cam.Pos.X - cam.ViewW/2
	top := cam.Pos.Y - cam.ViewH/2
	// отсечение по экрану: объектов на карте под тысячу, рисовать невидимые незачем
	if p.x+float64(p.w) < left || p.x > left+cam.ViewW ||
		p.y+float64(p.h) < top || p.y > top+cam.ViewH {
		return
	}
	img := p.img
	if p.frames > 1 && p.ticks > 0 {
		f := (m.tick / p.ticks) % p.frames
		img = img.SubImage(image.Rect(0, f*p.h, p.w, (f+1)*p.h)).(*ebiten.Image)
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(p.x-left, p.y-top)
	dst.DrawImage(img, op)
}
