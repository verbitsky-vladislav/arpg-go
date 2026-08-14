package scene

// Экран персонажей: три слота книги сохранений.
//
// Три — это правило игры, а не размер экрана: у каждого персонажа свой мир,
// своя добыча и свой счёт, и заводить их без счёта означало бы, что ни один
// ничего не стоит. Поэтому слот либо пуст («создать»), либо занят («играть»), и
// освободить его можно только удалив прежнюю жизнь.

import (
	"fmt"
	"log"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/vladislav/game/internal/assets"
	"github.com/vladislav/game/internal/character"
	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/save"
	"github.com/vladislav/game/internal/ui"
)

// Геометрия карточек слотов (логическое разрешение 640×360).
const (
	slotCardW, slotCardH = 176, 168
	slotCardGap          = 20
	slotCardTop          = 96
)

// Profiles — выбор персонажа: три карточки, под ними подсказки.
type Profiles struct {
	l     *assets.Loader
	back  Scene
	store *save.Store
	book  *save.Book
	cat   *character.Catalog // названия тел; nil — покажем идентификатор

	sel     int
	confirm bool   // выбранный слот ждёт подтверждения удаления
	err     string // не удалось начать забег
}

// NewProfiles читает книгу сохранений и собирает экран. Ошибка чтения здесь
// невозможна по устройству: битый файл читается как «сохранений нет».
func NewProfiles(l *assets.Loader, back Scene) *Profiles {
	return newProfiles(l, back, save.Default())
}

// newProfiles — то же с явным хранилищем (тесты, портативный запуск).
func newProfiles(l *assets.Loader, back Scene, st *save.Store) *Profiles {
	p := &Profiles{l: l, back: back, store: st}
	p.book = p.store.Load()
	if cat, err := character.Load(l.FS(), "character/character.json"); err == nil {
		p.cat = cat
	}
	// Курсор встаёт на первого живого персонажа: чаще всего заходят продолжать.
	for i := range save.Slots {
		if p.book.At(i) != nil {
			p.sel = i
			break
		}
	}
	return p
}

func (p *Profiles) Update() (Scene, error) {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		uiCancel()
		if p.confirm {
			p.confirm = false // сначала отменяется удаление, и только потом выход
			return p, nil
		}
		return p.backScene(), nil
	}

	mx, my := ebiten.CursorPosition()
	hovered := -1
	for i := range save.Slots {
		x, y, w, h := slotRect(i)
		if inRect(mx, my, x, y, w, h) {
			hovered = i
			if i != p.sel {
				p.sel, p.confirm = i, false // ушли с карточки — удаление отменено
			}
		}
	}
	if keyPressed(ebiten.KeyRight, ebiten.KeyD) {
		p.sel, p.confirm = (p.sel+1)%save.Slots, false
	}
	if keyPressed(ebiten.KeyLeft, ebiten.KeyA) {
		p.sel, p.confirm = (p.sel-1+save.Slots)%save.Slots, false
	}

	c := p.book.At(p.sel)
	if c != nil && keyPressed(ebiten.KeyDelete, ebiten.KeyBackspace) {
		p.confirm = true
		return p, nil
	}
	if c != nil && keyPressed(ebiten.KeyJ) {
		return newJournal(p.l, c, p), nil
	}

	act := keyPressed(ebiten.KeyEnter, ebiten.KeyNumpadEnter, ebiten.KeySpace)
	if hovered >= 0 && inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		act = true
	}
	if !act {
		return p, nil
	}
	switch {
	case p.confirm:
		p.remove(p.sel)
		return p, nil
	case c == nil:
		return p.create(p.sel), nil
	default:
		return p.play(p.sel, c)
	}
}

// play продолжает забег персонажа из слота i.
func (p *Profiles) play(i int, c *save.Char) (Scene, error) {
	// Забег, оставленный в меню на паузе, забывается: двух живых забегов у
	// одного персонажа быть не может, иначе они наперегонки писали бы в один
	// слот, и позже сохранившийся затирал бы более новое.
	if m, ok := p.backScene().(*Menu); ok {
		m.Resume = nil
	}
	g, err := NewSavedGame(p.l, p.backScene(), p.store, i, c)
	if err != nil {
		log.Println("забег:", err)
		p.err = "НЕ УДАЛОСЬ НАЧАТЬ ИГРУ"
		return p, nil
	}
	return g, nil
}

// create ведёт создание нового персонажа: сначала тело, потом имя, потом сразу
// в игру. Слот занимается только на последнем шаге — передумавший на середине
// возвращается к трём прежним карточкам.
func (p *Profiles) create(i int) Scene {
	return NewHeroes(p.l, p, func(bodyID string) (Scene, error) {
		return newName(p, bodyID, p.bodyTitle(bodyID), func(name string) (Scene, error) {
			c := NewChar(name, bodyID)
			c.Touch(time.Now())
			p.book.Put(i, c)
			// Мир прежнего жильца слота стирается до первого шага нового: иначе
			// он получил бы в наследство чужую карту (файл именуется слотом).
			p.store.DeleteMap(i)
			if err := p.store.Save(p.book); err != nil {
				log.Println("сохранение:", err)
			}
			return p.play(i, c)
		}), nil
	})
}

// remove стирает персонажа из слота вместе с его миром. Насовсем: другой жизни
// у него не было.
func (p *Profiles) remove(i int) {
	p.book.Delete(i)
	p.store.DeleteMap(i)
	p.confirm = false
	if err := p.store.Save(p.book); err != nil {
		log.Println("сохранение:", err)
		p.err = "НЕ УДАЛОСЬ УДАЛИТЬ"
	}
}

func (p *Profiles) backScene() Scene {
	if p.back != nil {
		return p.back
	}
	return NewMenu()
}

// bodyTitle — название тела для показа (незнакомое тело покажем как есть).
func (p *Profiles) bodyTitle(id string) string {
	if p.cat != nil {
		if b := p.cat.Body(id); b != nil && b.Title.RU != "" {
			return b.Title.RU
		}
	}
	return id
}

func (p *Profiles) Draw(screen *ebiten.Image) {
	drawMenuBack(screen)
	ui.PixelTextCentered(screen, "ПЕРСОНАЖИ", config.ScreenW/2, 40, 3, menuTitle)

	for i := range save.Slots {
		p.drawCard(screen, i)
	}

	hint := "ENTER - СОЗДАТЬ,  ESC - НАЗАД"
	switch {
	case p.confirm:
		hint = "ENTER - УДАЛИТЬ НАВСЕГДА,  ESC - ОТМЕНА"
	case p.book.At(p.sel) != nil:
		hint = "ENTER - ИГРАТЬ,  J - ЖУРНАЛ,  DEL - УДАЛИТЬ"
	}
	ui.PixelTextCentered(screen, hint, config.ScreenW/2, config.ScreenH-24, 1, menuEdge)
	if p.err != "" {
		ui.PixelTextCentered(screen, p.err, config.ScreenW/2, config.ScreenH-40, 1, ovDeadText)
	}
	ui.Cursor = ui.CursorArrow
}

func (p *Profiles) drawCard(dst *ebiten.Image, i int) {
	x, y, w, h := slotRect(i)
	sel := i == p.sel
	plate, edge, text := menuPlate, menuEdge, menuText
	if sel {
		plate, edge, text = menuPlateSel, menuEdgeSel, menuTextSel
	}
	vector.FillRect(dst, x, y, w, h, plate, false)
	vector.StrokeRect(dst, x+0.5, y+0.5, w-1, h-1, 1, edge, false)
	for _, c := range [][2]float32{{x, y}, {x + w - 1, y}, {x, y + h - 1}, {x + w - 1, y + h - 1}} {
		vector.FillRect(dst, c[0], c[1], 1, 1, menuBG, false)
	}

	cx := float64(x + w/2)
	ui.PixelTextCentered(dst, fmt.Sprintf("СЛОТ %d", i+1), cx, float64(y)+10, 1, menuEdge)

	c := p.book.At(i)
	if c == nil {
		ui.PixelTextCentered(dst, "ПУСТО", cx, float64(y+h/2)-14, 2, menuEdge)
		ui.PixelTextCentered(dst, "СОЗДАТЬ ГЕРОЯ", cx, float64(y+h/2)+6, 1, text)
		return
	}
	if sel && p.confirm {
		ui.PixelTextCentered(dst, "УДАЛИТЬ?", cx, float64(y+h/2)-14, 2, ovDeadText)
		ui.PixelTextCentered(dst, "ЭТО НАВСЕГДА", cx, float64(y+h/2)+6, 1, menuTextSel)
		return
	}

	ui.PixelTextCentered(dst, ui.PixelTextFit(c.Name, float64(w)-16, 2), cx, float64(y)+26, 2, text)
	ui.PixelTextCentered(dst, p.bodyTitle(c.Body), cx, float64(y)+48, 1, menuEdge)

	rows := [][2]string{
		{"УРОВЕНЬ", fmt.Sprintf("%d", c.Level)},
		{"УБИТО", fmt.Sprintf("%d", c.KillTotal())},
		{"СМЕРТЕЙ", fmt.Sprintf("%d", c.Deaths)},
		{"В ИГРЕ", playtime(c.Playtime)},
		{"СИД", fmt.Sprintf("%d", c.Seed)},
	}
	ty := float64(y) + 70
	for _, r := range rows {
		ui.PixelText(dst, r[0], float64(x)+10, ty, 1, menuEdge)
		ui.PixelText(dst, r[1], float64(x+w)-10-ui.PixelTextWidth(r[1], 1), ty, 1, text)
		ty += ui.PixelTextHeight(1) + 5
	}
}

// slotRect — рамка i-й карточки: три в ряд по центру экрана.
func slotRect(i int) (x, y, w, h float32) {
	total := float32(save.Slots*slotCardW + (save.Slots-1)*slotCardGap)
	x = (config.ScreenW-total)/2 + float32(i)*(slotCardW+slotCardGap)
	return x, slotCardTop, slotCardW, slotCardH
}

// playtime — время в игре человеческой строкой. Секунды показываются только
// пока их мало: «2Ч 14М 07С» читать некому.
func playtime(sec int) string {
	switch {
	case sec >= 3600:
		return fmt.Sprintf("%dЧ %02dМ", sec/3600, sec/60%60)
	case sec >= 60:
		return fmt.Sprintf("%dМ %02dС", sec/60, sec%60)
	default:
		return fmt.Sprintf("%dС", sec)
	}
}

// drawMenuBack — общий фон экранов вне забега: заливка и скан-линии.
func drawMenuBack(dst *ebiten.Image) {
	dst.Fill(menuBG)
	for y := 0; y < config.ScreenH; y += 3 {
		vector.FillRect(dst, 0, float32(y), config.ScreenW, 1, menuScan, false)
	}
}
