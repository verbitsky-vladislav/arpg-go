package scene

import (
	"fmt"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/vladislav/game/internal/anim"
	"github.com/vladislav/game/internal/assets"
	"github.com/vladislav/game/internal/character"
	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/mob"
	"github.com/vladislav/game/internal/sprite"
	"github.com/vladislav/game/internal/ui"
)

// Геометрия книги (логическое разрешение 640×360). Разворот: две страницы
// по бокам от корешка, закладки разделов торчат из левого и правого обрезов.
const (
	bookX, bookY = 30, 12
	bookW, bookH = 580, 336
	coverPad     = 7  // поля обложки вокруг бумаги
	spineW       = 8  // корешок между страницами
	pagePad      = 8  // поля страницы вокруг содержимого
	headH        = 26 // шапка страницы (название раздела)
	footH        = 26 // подвал книги (листалка)
	tabW, tabH   = 22, 104
	tabGap       = 16

	paperX = bookX + coverPad
	paperY = bookY + coverPad
	paperW = bookW - 2*coverPad
	paperH = bookH - 2*coverPad
	pageW  = (paperW - spineW) / 2
)

// Цвета: тёмный стол, кожаная обложка, желтоватая бумага, коричневые чернила.
var (
	bkDesk     = color.RGBA{0x0b, 0x0e, 0x18, 0xff}
	bkCover    = color.RGBA{0x5a, 0x3a, 0x24, 0xff}
	bkCoverHi  = color.RGBA{0x7d, 0x54, 0x33, 0xff}
	bkCoverLo  = color.RGBA{0x2e, 0x1c, 0x11, 0xff}
	bkPaper    = color.RGBA{0xe8, 0xda, 0xb6, 0xff}
	bkPaperDim = color.RGBA{0xd2, 0xc0, 0x95, 0xff}
	bkFrame    = color.RGBA{0xa8, 0x8e, 0x63, 0xff}
	bkInk      = color.RGBA{0x4a, 0x38, 0x26, 0xff}
	bkInkDim   = color.RGBA{0x8a, 0x76, 0x5a, 0xff}
	bkTabOn    = color.RGBA{0xd8, 0xae, 0x54, 0xff}
	bkTabOff   = color.RGBA{0x6e, 0x4a, 0x2e, 0xff}
)

// bestEntry — одна карточка книги: существо со своей анимацией ходьбы.
type bestEntry struct {
	title string   // подпись в рамке
	sub   string   // вторая строка (мелким)
	facts []string // строки-характеристики (страница персонажа)
	pack  *sprite.Pack
	play  *anim.Player
	refH  int    // высота, по которой подбирается масштаб (см. drawFrameFit)
	err   string // пак не загрузился — карточка пустая с пометкой
}

// bestSection — раздел книги (закладка). Загружается лениво, при первом
// открытии: входить в бестиарий не должно стоить чтения всех паков сразу.
type bestSection struct {
	title   string
	side    int                          // 0 — закладка слева, 1 — справа
	perPage int                          // существ на страницу (у персонажа — 1)
	note    string                       // что писать на пустой странице
	load    func() ([]bestEntry, string) // ленивая загрузка; вторая строка — причина пустоты

	entries []bestEntry
	loaded  bool
	spread  int // текущий разворот
}

func (s *bestSection) ensure() {
	if s.loaded || s.load == nil {
		return
	}
	s.loaded = true
	s.entries, s.note = s.load()
}

// perSpread — сколько существ помещается на развороте.
func (s *bestSection) perSpread() int { return s.perPage * 2 }

// spreads — число разворотов (минимум один, пусть и пустой).
func (s *bestSection) spreads() int {
	if len(s.entries) == 0 {
		return 1
	}
	return (len(s.entries) + s.perSpread() - 1) / s.perSpread()
}

// Bestiary — книга существ: разделы-закладки по бокам, листалка снизу,
// на развороте — карточки с анимацией ходьбы.
type Bestiary struct {
	back Scene // куда возвращаться по ESC
	secs []*bestSection
	cur  int
}

// NewBestiary собирает книгу. Данных может не быть (нет файла, битый пак) —
// это не ошибка сцены: раздел просто окажется пустым с пояснением на странице.
func NewBestiary(l *assets.Loader, back Scene) *Bestiary {
	return &Bestiary{
		back: back,
		secs: []*bestSection{
			{title: "ПЕРСОНАЖИ", side: 0, perPage: 1, load: func() ([]bestEntry, string) { return loadPersons(l) }},
			{title: "ЖИВОТНЫЕ", side: 0, perPage: 4, load: func() ([]bestEntry, string) { return loadAnimals(l) }},
			// Мобов и боссов ещё нет в данных: раздел листается и открывается,
			// но вместо карточек честно пишет, что содержимое впереди.
			{title: "МОБЫ", side: 1, perPage: 4, note: "СКОРО"},
			{title: "БОССЫ", side: 1, perPage: 4, note: "СКОРО"},
		},
	}
}

// loadPersons — карточки персонажа: по одной на пару «тело × лоадаут».
func loadPersons(l *assets.Loader) ([]bestEntry, string) {
	cat, err := character.Load(l.FS(), "character/character.json")
	if err != nil {
		return nil, "НЕТ ДАННЫХ ПЕРСОНАЖА"
	}
	var out []bestEntry
	for _, bid := range cat.BodyIDs() {
		for _, lid := range cat.LoadoutIDs() {
			b, lo := cat.Body(bid), cat.Loadout(lid)
			e := bestEntry{
				title: b.Title.RU,
				sub:   lo.Title.RU,
				facts: []string{
					fmt.Sprintf("ЗДОРОВЬЕ    %d", int(float64(cat.Base.HP)*b.HPScale)),
					fmt.Sprintf("ШАГ / БЕГ   %.0f / %.0f",
						cat.Base.Speed.Walk*b.SpeedScale*lo.SpeedScale,
						cat.Base.Speed.Run*b.SpeedScale*lo.SpeedScale),
					fmt.Sprintf("УРОН        %d", lo.Attack.Damage),
					fmt.Sprintf("РАЗМАХ      %.0f ПКС / %.0f ГРАД", lo.Attack.Reach, lo.Attack.Arc),
					fmt.Sprintf("ЗАМАХ       %.2f С", float64(lo.Attack.SwingTicks)/config.TPS),
				},
			}
			p, err := character.LoadPack(l, "character", b, lo)
			if err != nil {
				e.err = "НЕТ СПРАЙТОВ"
			} else {
				// Рост меряем по кадру, а не по рамке пикселей: у пака sword
				// рамка выше на замах клинка, и герой из-за оружия ужимался бы.
				e.pack, e.refH = p, p.Frame.H
				e.play = anim.NewPlayer(walkClip(p))
			}
			out = append(out, e)
		}
	}
	return out, ""
}

// loadAnimals — карточки животных по таблице видов.
func loadAnimals(l *assets.Loader) ([]bestEntry, string) {
	cat, err := mob.LoadSpecies(l.FS(), "mobs/animals/species.json")
	if err != nil {
		return nil, "НЕТ ТАБЛИЦЫ ВИДОВ"
	}
	var out []bestEntry
	for _, id := range cat.IDs() {
		sp := cat.Get(id)
		kind := "ДОМАШНЕЕ"
		if sp.Wild() {
			kind = "ДИКОЕ"
		}
		e := bestEntry{
			title: sp.Title.RU,
			sub:   fmt.Sprintf("%s  HP %d", kind, sp.Stats.HP),
		}
		p, err := sprite.Load(l, "mobs/animals/"+sp.Art)
		if err != nil {
			e.err = "НЕТ СПРАЙТОВ"
		} else {
			e.pack, e.refH = p, p.Bounds().H
			e.play = anim.NewPlayer(walkClip(p))
		}
		out = append(out, e)
	}
	return out, ""
}

// walkClip — клип ходьбы лицом к читателю. Если ходьбы в паке нет (птица,
// рыба, недоделанный пак) — стойка, а нет и её — первая попавшаяся анимация:
// пустая рамка в книге хуже, чем не та анимация.
func walkClip(p *sprite.Pack) *anim.Clip {
	for _, name := range []string{"walk", "idle"} {
		if c := p.Clip(name, sprite.Down); c.Valid() {
			return c
		}
	}
	for _, name := range p.Anims() {
		if c := p.Clip(name, sprite.Down); c.Valid() {
			return c
		}
	}
	return nil
}

func (b *Bestiary) section() *bestSection { return b.secs[b.cur] }

func (b *Bestiary) Update() (Scene, error) {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		if b.back != nil {
			return b.back, nil
		}
		return NewMenu(), nil
	}

	sec := b.section()
	sec.ensure()

	// Закладки: мышь по всему ярлыку, клавиши — перебор разделов по кругу.
	mx, my := ebiten.CursorPosition()
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		for i := range b.secs {
			if x, y, w, h := b.tabRect(i); inRect(mx, my, x, y, w, h) {
				b.cur = i
				b.section().ensure()
			}
		}
	}
	if keyPressed(ebiten.KeyDown, ebiten.KeyS) {
		b.selectStep(1)
	}
	if keyPressed(ebiten.KeyUp, ebiten.KeyW) {
		b.selectStep(-1)
	}

	// Листалка: стрелки, колесо и кнопки в подвале.
	turn := 0
	if keyPressed(ebiten.KeyRight, ebiten.KeyD) {
		turn++
	}
	if keyPressed(ebiten.KeyLeft, ebiten.KeyA) {
		turn--
	}
	if _, wy := ebiten.Wheel(); wy != 0 {
		turn -= int(math.Copysign(1, wy))
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		x, y, w, h := arrowRect(-1)
		if inRect(mx, my, x, y, w, h) {
			turn--
		}
		if x, y, w, h := arrowRect(1); inRect(mx, my, x, y, w, h) {
			turn++
		}
	}
	sec = b.section()
	sec.spread = clampInt(sec.spread+turn, 0, sec.spreads()-1)

	// Крутим только то, что видно на развороте.
	for _, e := range b.visible() {
		if e.play == nil {
			continue
		}
		e.play.Update()
		if e.play.Finished() { // незацикленный клип (idle без loop) — по кругу
			e.play.Play(e.play.Clip())
		}
	}
	return b, nil
}

// selectStep переводит выбор на соседний раздел (по кругу).
func (b *Bestiary) selectStep(d int) {
	b.cur = ((b.cur+d)%len(b.secs) + len(b.secs)) % len(b.secs)
	b.section().ensure()
}

// visible — карточки текущего разворота.
func (b *Bestiary) visible() []*bestEntry {
	sec := b.section()
	from := sec.spread * sec.perSpread()
	var out []*bestEntry
	for i := from; i < from+sec.perSpread() && i < len(sec.entries); i++ {
		out = append(out, &sec.entries[i])
	}
	return out
}

func (b *Bestiary) Draw(screen *ebiten.Image) {
	screen.Fill(bkDesk)
	drawBook(screen)

	sec := b.section()
	for page := range 2 {
		b.drawPage(screen, sec, page)
	}
	b.drawFooter(screen, sec)

	for i, s := range b.secs {
		b.drawTab(screen, i, s)
	}
}

// drawBook — обложка, обрез страниц и корешок.
func drawBook(dst *ebiten.Image) {
	vector.FillRect(dst, bookX, bookY, bookW, bookH, bkCover, false)
	vector.StrokeRect(dst, bookX+0.5, bookY+0.5, bookW-1, bookH-1, 1, bkCoverLo, false)
	vector.StrokeRect(dst, bookX+2.5, bookY+2.5, bookW-5, bookH-5, 1, bkCoverHi, false)

	for page := range 2 {
		x := float32(pageX(page))
		vector.FillRect(dst, x, paperY, pageW, paperH, bkPaper, false)
		// Тень у корешка: страница уходит в сгиб.
		if page == 0 {
			vector.FillRect(dst, x+pageW-4, paperY, 4, paperH, bkPaperDim, false)
		} else {
			vector.FillRect(dst, x, paperY, 4, paperH, bkPaperDim, false)
		}
	}
	cx := float32(bookX + bookW/2)
	vector.FillRect(dst, cx-spineW/2, paperY, spineW, paperH, bkCoverLo, false)
	vector.FillRect(dst, cx-1, paperY, 2, paperH, bkCover, false)
}

// pageX — левый край страницы (0 — левая, 1 — правая).
func pageX(page int) float64 {
	if page == 0 {
		return paperX
	}
	return paperX + pageW + spineW
}

// drawPage рисует шапку страницы и её карточки.
func (b *Bestiary) drawPage(dst *ebiten.Image, sec *bestSection, page int) {
	px := float32(pageX(page))
	cx := float64(px) + pageW/2

	ui.PixelTextCentered(dst, sec.title, cx, paperY+5, 2, bkInk)
	vector.FillRect(dst, px+pagePad, paperY+headH-4, pageW-2*pagePad, 1, bkFrame, false)

	from := sec.spread*sec.perSpread() + page*sec.perPage
	// Раздел закрыт или пуст — вместо карточек пояснение по центру страницы.
	if len(sec.entries) == 0 {
		note := sec.note
		if note == "" {
			note = "ПУСТО"
		}
		ui.PixelTextCentered(dst, note, cx, paperY+paperH/2-7, 2, bkInkDim)
		return
	}

	for i := range sec.perPage {
		if from+i >= len(sec.entries) {
			return
		}
		x, y, w, h := cellRect(page, i, sec.perPage)
		drawCard(dst, &sec.entries[from+i], x, y, w, h, sec.perPage == 1)
	}
}

// cellRect — рамка i-й карточки на странице page (сетка 2×2, либо одна на всю
// страницу).
func cellRect(page, i, perPage int) (x, y, w, h float32) {
	ix := float32(pageX(page)) + pagePad
	iy := float32(paperY + headH)
	iw := float32(pageW - 2*pagePad)
	ih := float32(paperH - headH - footH)
	if perPage == 1 {
		return ix, iy, iw, ih
	}
	w, h = (iw-6)/2, (ih-6)/2
	return ix + float32(i%2)*(w+6), iy + float32(i/2)*(h+6), w, h
}

// drawCard — карточка существа: рамка, кадр анимации и подписи. big — страница
// целиком под одного (персонаж), тогда справа от портрета помещаются числа.
func drawCard(dst *ebiten.Image, e *bestEntry, x, y, w, h float32, big bool) {
	vector.FillRect(dst, x, y, w, h, bkPaper, false)
	vector.StrokeRect(dst, x+0.5, y+0.5, w-1, h-1, 1, bkFrame, false)
	vector.StrokeRect(dst, x+2.5, y+2.5, w-5, h-5, 1, bkPaperDim, false)

	nameScale, subScale := 2.0, 1.0
	nameY := float64(y+h) - 27 // ниже — вторая строка, обе внутри рамки
	if big {
		nameScale, subScale = 3.0, 2.0
		nameY = float64(y) + 10
	}

	// Портрет: «пол» карточки — над подписями (или под ними на большой странице).
	base := nameY - 6
	top := float64(y) + 6
	if big {
		top = nameY + 34
		base = float64(y+h) - float64(len(e.facts))*10 - 16
	}
	// Рост, к которому приводятся все существа: кадры в паках от 16 до 64 px,
	// и без общей цели цыплёнок вышел бы с буйвола.
	target := 52.0
	if big {
		target = 190
	}
	if e.err != "" {
		ui.PixelTextCentered(dst, e.err, float64(x+w/2), (top+base)/2, 1, bkInkDim)
	} else if img := e.play.Frame(); img != nil {
		drawFrameFit(dst, img, e.pack.Bounds(), e.refH, float64(x+w/2), base, target, float64(w)-14, base-top)
	}

	ui.PixelTextCentered(dst, ui.PixelTextFit(e.title, float64(w)-10, nameScale), float64(x+w/2), nameY, nameScale, bkInk)
	if e.sub != "" {
		ui.PixelTextCentered(dst, ui.PixelTextFit(e.sub, float64(w)-10, subScale),
			float64(x+w/2), nameY+ui.PixelTextHeight(nameScale)+3, subScale, bkInkDim)
	}
	if big {
		vector.FillRect(dst, x+20, y+52, w-40, 1, bkFrame, false)
	}
	for i, f := range e.facts {
		ui.PixelText(dst, f, float64(x)+14, float64(y+h)-float64(len(e.facts)-i)*10-6, 1, bkInk)
	}
}

// drawFrameFit рисует кадр целым масштабом: середина непрозрачной части кадра
// приходится на cx, низ — на base, а высота refH стремится к targetH. Дробный масштаб
// испортил бы пиксели, поэтому берётся ближайший целый, при котором существо
// ещё влезает в рамку.
func drawFrameFit(dst *ebiten.Image, img *ebiten.Image, bb sprite.Rect, refH int, cx, base, targetH, maxW, maxH float64) {
	bw, bh := float64(max(bb.W, 1)), float64(max(bb.H, 1))
	sc := math.Round(targetH / float64(max(refH, 1)))
	sc = math.Max(1, math.Min(4, sc))
	for sc > 1 && (bw*sc > maxW || bh*sc > maxH) {
		sc--
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(sc, sc)
	op.GeoM.Translate(cx-(float64(bb.X)+bw/2)*sc, base-float64(bb.Y+bb.H)*sc)
	dst.DrawImage(img, op)
}

// drawFooter — листалка и подсказки в подвале книги.
func (b *Bestiary) drawFooter(dst *ebiten.Image, sec *bestSection) {
	y := float32(paperY + paperH - footH + 4)
	for _, d := range []int{-1, 1} {
		x, ay, w, h := arrowRect(d)
		on := (d < 0 && sec.spread > 0) || (d > 0 && sec.spread < sec.spreads()-1)
		col, ink := bkPaperDim, bkInkDim
		if on {
			col, ink = bkTabOn, bkInk
		}
		vector.FillRect(dst, x, ay, w, h, col, false)
		vector.StrokeRect(dst, x+0.5, ay+0.5, w-1, h-1, 1, bkFrame, false)
		s := ">"
		if d < 0 {
			s = "<"
		}
		ui.PixelTextCentered(dst, s, float64(x+w/2), float64(ay)+4, 2, ink)
	}
	// Счётчик разворотов — на поле левой страницы: по центру его разрезал бы
	// корешок.
	ui.PixelText(dst, fmt.Sprintf("%d / %d", sec.spread+1, sec.spreads()),
		pageX(0)+pagePad, float64(y)+5, 1, bkInkDim)

	ui.PixelTextCentered(dst, "ESC - НАЗАД,  СТРЕЛКИ - ЛИСТАТЬ,  ЗАКЛАДКИ - РАЗДЕЛЫ",
		config.ScreenW/2, config.ScreenH-9, 1, bkFrame)
}

// arrowRect — кнопка листалки: d<0 — назад, d>0 — вперёд.
func arrowRect(d int) (x, y, w, h float32) {
	const aw, ah = 20, 16
	cx := float32(bookX + bookW/2)
	y = float32(paperY + paperH - footH + 2)
	if d < 0 {
		return cx - 44 - aw, y, aw, ah
	}
	return cx + 44, y, aw, ah
}

// tabRect — ярлык закладки i-го раздела. Разделы делятся по обрезам книги:
// side 0 — левый, side 1 — правый; порядок внутри стороны — сверху вниз.
func (b *Bestiary) tabRect(i int) (x, y, w, h float32) {
	s := b.secs[i]
	n := 0 // номер закладки на своей стороне
	for j := range i {
		if b.secs[j].side == s.side {
			n++
		}
	}
	total := 0
	for _, o := range b.secs {
		if o.side == s.side {
			total++
		}
	}
	top := float32(bookY) + (bookH-float32(total*tabH+(total-1)*tabGap))/2
	y = top + float32(n)*(tabH+tabGap)
	x = float32(bookX - tabW + 4)
	if s.side == 1 {
		x = float32(bookX + bookW - 4)
	}
	return x, y, tabW, tabH
}

// drawTab рисует закладку: выбранная выезжает наружу, закрытая — тусклая.
func (b *Bestiary) drawTab(dst *ebiten.Image, i int, s *bestSection) {
	x, y, w, h := b.tabRect(i)
	sel := i == b.cur
	col, ink := bkTabOff, bkPaper
	if sel {
		col, ink = bkTabOn, bkCoverLo
	}
	if sel { // выбранная закладка торчит из книги сильнее
		if s.side == 0 {
			x -= 3
		}
		w += 3
	}
	vector.FillRect(dst, x, y, w, h, col, false)
	vector.StrokeRect(dst, x+0.5, y+0.5, w-1, h-1, 1, bkCoverLo, false)

	label := s.title
	tx := float64(x) + (float64(w)-ui.PixelTextHeight(1))/2
	ty := float64(y+h)/1 - (float64(h)-ui.PixelTextWidth(label, 1))/2
	ui.PixelTextRot90(dst, label, tx, ty, 1, ink)
}

func inRect(mx, my int, x, y, w, h float32) bool {
	fx, fy := float32(mx), float32(my)
	return fx >= x && fx < x+w && fy >= y && fy < y+h
}

func clampInt(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	return min(max(v, lo), hi)
}
