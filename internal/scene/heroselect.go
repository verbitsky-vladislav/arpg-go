package scene

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/vladislav/game/internal/anim"
	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/hero"
	"github.com/vladislav/game/internal/ui"
)

// Раскладка сетки героев.
const (
	cardW   = 96
	cardH   = 132
	cardGap = 8
	gridTop = 74
)

// HeroSelect — экран выбора героя. Два режима:
//   - сетка: все герои проигрывают idle; наведение подсвечивает карточку;
//   - showcase: по клику выбранный герой крупно проигрывает все анимации подряд.
type HeroSelect struct {
	reg   *hero.Registry
	back  Scene // куда вернуться (меню)
	cards []*heroCard

	focus int       // -1 — сетка; иначе индекс героя в showcase-режиме
	show  *showcase // активный showcase (в режиме focus>=0)
	backB ui.Button // «назад» — в меню (сетка) / к сетке (showcase)
	prevB ui.Button // предыдущий герой (showcase)
	nextB ui.Button // следующий герой (showcase)
}

// heroCard — карточка героя в сетке со своим проигрывателем idle.
type heroCard struct {
	h    *hero.Hero
	idle *anim.Player
	rect image.Rectangle
}

// NewHeroSelect строит экран выбора поверх реестра героев; back — сцена возврата.
func NewHeroSelect(reg *hero.Registry, back Scene) *HeroSelect {
	s := &HeroSelect{reg: reg, back: back, focus: -1}

	heroes := reg.Heroes()
	rowW := len(heroes)*cardW + (len(heroes)-1)*cardGap
	startX := (config.ScreenW - rowW) / 2
	for i, h := range heroes {
		x := startX + i*(cardW+cardGap)
		s.cards = append(s.cards, &heroCard{
			h:    h,
			idle: anim.NewPlayer(h.Clip(hero.Idle, hero.Down)),
			rect: image.Rect(x, gridTop, x+cardW, gridTop+cardH),
		})
	}

	s.backB = ui.Button{X: 8, Y: 8, W: 96, H: 22, Label: "< BACK"}
	s.prevB = ui.Button{X: 16, Y: config.ScreenH/2 - 14, W: 28, H: 28, Label: "<"}
	s.nextB = ui.Button{X: config.ScreenW - 44, Y: config.ScreenH/2 - 14, W: 28, H: 28, Label: ">"}
	return s
}

func (s *HeroSelect) Update() (Scene, error) {
	mx, my := ebiten.CursorPosition()
	click := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)

	if s.focus < 0 {
		return s.updateGrid(mx, my, click)
	}
	return s.updateShowcase(mx, my, click)
}

func (s *HeroSelect) updateGrid(mx, my int, click bool) (Scene, error) {
	for _, c := range s.cards {
		c.idle.Update()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		return s.back, nil
	}
	if click && s.backB.Contains(mx, my) {
		return s.back, nil
	}
	// Выбор героя: клик по карточке, цифра 1..N или Enter (герой под курсором).
	if i, ok := s.pickedKey(); ok {
		s.enter(i)
	} else if click {
		for i, c := range s.cards {
			if in(c.rect, mx, my) {
				s.enter(i)
				break
			}
		}
	}
	return s, nil
}

// pickedKey возвращает индекс героя, выбранного с клавиатуры (цифры 1..N,
// либо Enter — герой под курсором, иначе первый).
func (s *HeroSelect) pickedKey() (int, bool) {
	for i := range s.cards {
		if i < 9 && inpututil.IsKeyJustPressed(ebiten.Key(int(ebiten.Key1)+i)) {
			return i, true
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		mx, my := ebiten.CursorPosition()
		for i, c := range s.cards {
			if in(c.rect, mx, my) {
				return i, true
			}
		}
		return 0, true
	}
	return 0, false
}

func (s *HeroSelect) updateShowcase(mx, my int, click bool) (Scene, error) {
	s.show.Update()

	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) ||
		(click && s.backB.Contains(mx, my)) {
		s.focus = -1
		s.show = nil
		return s, nil
	}
	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeyRight) || inpututil.IsKeyJustPressed(ebiten.KeyD) ||
		(click && s.nextB.Contains(mx, my)):
		s.enter((s.focus + 1) % len(s.cards))
	case inpututil.IsKeyJustPressed(ebiten.KeyLeft) || inpututil.IsKeyJustPressed(ebiten.KeyA) ||
		(click && s.prevB.Contains(mx, my)):
		s.enter((s.focus - 1 + len(s.cards)) % len(s.cards))
	// Переключение направления вида: вверх/вниз (front / back / side).
	case inpututil.IsKeyJustPressed(ebiten.KeyUp) || inpututil.IsKeyJustPressed(ebiten.KeyW):
		s.show.cycleFacing(1)
	case inpututil.IsKeyJustPressed(ebiten.KeyDown) || inpututil.IsKeyJustPressed(ebiten.KeyS):
		s.show.cycleFacing(-1)
	}
	return s, nil
}

// enter переводит экран в showcase-режим на герое с индексом i.
func (s *HeroSelect) enter(i int) {
	s.focus = i
	s.show = newShowcase(s.cards[i].h)
}

func (s *HeroSelect) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{0x10, 0x12, 0x1a, 0xff})
	if s.focus < 0 {
		s.drawGrid(screen)
	} else {
		s.drawShowcase(screen)
	}
}

func (s *HeroSelect) drawGrid(screen *ebiten.Image) {
	ui.TextCentered(screen, "CHOOSE  YOUR  HERO", config.ScreenW/2, 30)
	ui.TextCentered(screen, "click a hero to preview all animations", config.ScreenW/2, 46)

	mx, my := ebiten.CursorPosition()
	for _, c := range s.cards {
		hovered := in(c.rect, mx, my)
		drawCard(screen, c, hovered)
	}
	s.backB.Draw(screen, s.backB.Contains(mx, my))
}

func drawCard(screen *ebiten.Image, c *heroCard, hovered bool) {
	r := c.rect
	bg := color.RGBA{0x1b, 0x1f, 0x2c, 0xff}
	border := color.RGBA{0x39, 0x40, 0x55, 0xff}
	if hovered {
		bg = color.RGBA{0x27, 0x2e, 0x44, 0xff}
		border = color.RGBA{0xc8, 0xd4, 0xff, 0xff}
	}
	vector.DrawFilledRect(screen, float32(r.Min.X), float32(r.Min.Y),
		float32(r.Dx()), float32(r.Dy()), bg, false)
	vector.StrokeRect(screen, float32(r.Min.X), float32(r.Min.Y),
		float32(r.Dx()), float32(r.Dy()), 1, border, false)

	cx := float64(r.Min.X+r.Max.X) / 2
	ui.DrawSpriteFit(screen, c.idle.Frame(), cx, float64(r.Min.Y)+58, 84)
	ui.TextCentered(screen, c.h.Name, (r.Min.X+r.Max.X)/2, r.Max.Y-18)
}

func (s *HeroSelect) drawShowcase(screen *ebiten.Image) {
	mx, my := ebiten.CursorPosition()
	h := s.cards[s.focus].h

	ui.TextCentered(screen, h.Name, config.ScreenW/2, 30)

	// Крупный превью текущей анимации (с флипом для вида «side»).
	ui.DrawSpriteFitFlip(screen, s.show.Frame(), config.ScreenW/2, config.ScreenH/2, 220, s.show.flip())

	// Подпись действия, направления и прогресс.
	ui.TextCentered(screen, s.show.action().Title()+"  ·  "+s.show.facingLabel(),
		config.ScreenW/2, config.ScreenH-70)
	ui.TextCentered(screen, s.show.progress(), config.ScreenW/2, config.ScreenH-52)
	ui.TextCentered(screen, "<- / -> hero    up/down facing    Esc back",
		config.ScreenW/2, config.ScreenH-28)

	s.backB.Draw(screen, s.backB.Contains(mx, my))
	s.prevB.Draw(screen, s.prevB.Contains(mx, my))
	s.nextB.Draw(screen, s.nextB.Contains(mx, my))
}

// in — попадание точки в прямоугольник.
func in(r image.Rectangle, x, y int) bool {
	return image.Pt(x, y).In(r)
}
