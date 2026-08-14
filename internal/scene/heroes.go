package scene

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/vladislav/game/internal/anim"
	"github.com/vladislav/game/internal/assets"
	"github.com/vladislav/game/internal/character"
	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/sprite"
	"github.com/vladislav/game/internal/ui"
)

// Геометрия карточек выбора (логическое разрешение 640×360).
const (
	heroCardW, heroCardH = 160, 196
	heroCardGap          = 24
	heroCardTop          = 92
)

// heroCard — один вариант героя: тело плюс его пак без оружия.
type heroCard struct {
	body *character.Body
	pack *sprite.Pack
	play *anim.Player
}

// Heroes — экран выбора героя перед стартом. Лоадаут один — «без оружия»:
// выбирается только тело, поэтому карточек ровно столько, сколько тел.
type Heroes struct {
	l     *assets.Loader
	back  Scene
	cat   *character.Catalog
	cards []heroCard
	sel   int
	err   string // не удалось загрузить данные или начать игру
}

// NewHeroes собирает экран выбора. Ошибка загрузки не роняет сцену: экран
// покажет её текстом — из меню игрок всё равно сможет уйти.
func NewHeroes(l *assets.Loader, back Scene) *Heroes {
	h := &Heroes{l: l, back: back}
	cat, err := character.Load(l.FS(), "character/character.json")
	if err != nil {
		h.err = "НЕТ ДАННЫХ ПЕРСОНАЖА"
		return h
	}
	h.cat = cat
	lo := cat.Loadout(gameLoadout)
	if lo == nil {
		h.err = "НЕТ ЛОАДАУТА " + gameLoadout
		return h
	}
	for _, id := range cat.BodyIDs() {
		b := cat.Body(id)
		p, err := character.LoadPack(l, "character", b, lo)
		if err != nil {
			h.err = "НЕТ СПРАЙТОВ " + id
			continue
		}
		h.cards = append(h.cards, heroCard{body: b, pack: p, play: anim.NewPlayer(walkClip(p))})
	}
	return h
}

func (h *Heroes) Update() (Scene, error) {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		return h.backScene(), nil
	}
	if len(h.cards) == 0 {
		return h, nil
	}

	mx, my := ebiten.CursorPosition()
	hovered := -1
	for i := range h.cards {
		x, y, w, hh := heroCardRect(i, len(h.cards))
		if inRect(mx, my, x, y, w, hh) {
			hovered, h.sel = i, i
		}
	}
	if keyPressed(ebiten.KeyRight, ebiten.KeyD) {
		h.sel = (h.sel + 1) % len(h.cards)
	}
	if keyPressed(ebiten.KeyLeft, ebiten.KeyA) {
		h.sel = (h.sel - 1 + len(h.cards)) % len(h.cards)
	}

	start := keyPressed(ebiten.KeyEnter, ebiten.KeyNumpadEnter, ebiten.KeySpace)
	if hovered >= 0 && inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		start = true
	}
	if start {
		g, err := NewGame(h.l, h.backScene(), h.cards[h.sel].body.ID)
		if err != nil {
			h.err = "НЕ УДАЛОСЬ НАЧАТЬ ИГРУ"
			return h, nil
		}
		return g, nil
	}

	for i := range h.cards {
		c := &h.cards[i]
		c.play.Update()
		if c.play.Finished() {
			c.play.Play(c.play.Clip())
		}
	}
	return h, nil
}

func (h *Heroes) backScene() Scene {
	if h.back != nil {
		return h.back
	}
	return NewMenu()
}

func (h *Heroes) Draw(screen *ebiten.Image) {
	screen.Fill(menuBG)
	for y := 0; y < config.ScreenH; y += 3 {
		vector.FillRect(screen, 0, float32(y), config.ScreenW, 1, menuScan, false)
	}
	ui.PixelTextCentered(screen, "ВЫБЕРИТЕ ГЕРОЯ", config.ScreenW/2, 40, 3, menuTitle)

	if h.err != "" {
		ui.PixelTextCentered(screen, h.err, config.ScreenW/2, config.ScreenH/2, 2, menuTextSel)
	}
	for i, c := range h.cards {
		h.drawCard(screen, i, c)
	}
	ui.PixelTextCentered(screen, "ESC - НАЗАД,  ENTER - В ИГРУ", config.ScreenW/2, config.ScreenH-24, 1, menuEdge)
}

func (h *Heroes) drawCard(dst *ebiten.Image, i int, c heroCard) {
	x, y, w, ch := heroCardRect(i, len(h.cards))
	sel := i == h.sel
	plate, edge, text := menuPlate, menuEdge, menuText
	if sel {
		plate, edge, text = menuPlateSel, menuEdgeSel, menuTextSel
	}
	vector.FillRect(dst, x, y, w, ch, plate, false)
	vector.StrokeRect(dst, x+0.5, y+0.5, w-1, ch-1, 1, edge, false)
	for _, p := range [][2]float32{{x, y}, {x + w - 1, y}, {x, y + ch - 1}, {x + w - 1, y + ch - 1}} {
		vector.FillRect(dst, p[0], p[1], 1, 1, menuBG, false)
	}

	base := float64(y+ch) - 44
	if img := c.play.Frame(); img != nil {
		drawFrameFit(dst, img, c.pack.Bounds(), c.pack.Frame.H,
			float64(x+w/2), base, 150, float64(w)-16, base-float64(y)-10)
	}
	ui.PixelTextCentered(dst, c.body.Title.RU, float64(x+w/2), float64(y+ch)-36, 2, text)
	ui.PixelTextCentered(dst, "БЕЗ ОРУЖИЯ", float64(x+w/2), float64(y+ch)-18, 1, menuEdge)
}

// heroCardRect — рамка i-й карточки из n (карточки стоят в ряд по центру).
func heroCardRect(i, n int) (x, y, w, h float32) {
	total := float32(n*heroCardW + (n-1)*heroCardGap)
	x = (config.ScreenW-total)/2 + float32(i)*(heroCardW+heroCardGap)
	return x, heroCardTop, heroCardW, heroCardH
}
