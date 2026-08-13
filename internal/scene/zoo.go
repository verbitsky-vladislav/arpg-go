package scene

import (
	"fmt"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/vladislav/game/internal/anim"
	"github.com/vladislav/game/internal/assets"
	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/mob"
	"github.com/vladislav/game/internal/sprite"
	"github.com/vladislav/game/internal/ui"
)

// Zoo — служебная сцена: все виды животных сразу, сеткой. Нужна, чтобы глазами
// сверить графику с данными — какие анимации на самом деле в паках, как они
// называются, совпадает ли характер вида с тем, что видно на спрайте.
//
// Управление: стрелки — выбор клетки, [ ] — анимация выбранного, TAB —
// направление у всех, ПРОБЕЛ — пауза, ESC — в меню.
type Zoo struct {
	cells []zooCell
	cur   int
	dir   sprite.Dir
	pause bool
}

type zooCell struct {
	id      string
	sp      *mob.Species
	pack    *sprite.Pack
	anims   []string
	anim    int
	player  *anim.Player
	missing bool // пак не загрузился
	err     string
}

const (
	zooCols  = 6
	zooCellW = config.ScreenW / zooCols
	zooCellH = 66
	zooTop   = 18
)

// NewZoo собирает сцену по таблице видов: на каждый вид — свой спрайт-пак.
// Вид без пака не роняет сцену, а показывается красной клеткой с ошибкой —
// смысл сцены как раз в том, чтобы такие расхождения было видно.
func NewZoo(l *assets.Loader, cat *mob.Catalog, root string) *Zoo {
	z := &Zoo{}
	for _, id := range cat.IDs() {
		sp := cat.Get(id)
		c := zooCell{id: id, sp: sp}
		p, err := sprite.Load(l, root+"/"+sp.Art)
		if err != nil {
			c.missing, c.err = true, err.Error()
		} else {
			c.pack, c.anims = p, p.Anims()
			c.player = anim.NewPlayer(nil)
		}
		z.cells = append(z.cells, c)
	}
	z.reload()
	return z
}

// reload перезаряжает проигрыватели после смены направления или анимации.
func (z *Zoo) reload() {
	for i := range z.cells {
		c := &z.cells[i]
		if c.missing || len(c.anims) == 0 {
			continue
		}
		c.player.Play(c.pack.Clip(c.anims[c.anim], z.dir))
	}
}

func (z *Zoo) Update() (Scene, error) {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		return NewMenu(), nil
	}
	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeyRight):
		z.cur = (z.cur + 1) % len(z.cells)
	case inpututil.IsKeyJustPressed(ebiten.KeyLeft):
		z.cur = (z.cur - 1 + len(z.cells)) % len(z.cells)
	case inpututil.IsKeyJustPressed(ebiten.KeyDown):
		z.cur = (z.cur + zooCols) % len(z.cells)
	case inpututil.IsKeyJustPressed(ebiten.KeyUp):
		z.cur = (z.cur - zooCols + len(z.cells)) % len(z.cells)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyTab) {
		z.dir = (z.dir + 1) % sprite.DirCount
		z.reload()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		z.pause = !z.pause
	}
	if c := &z.cells[z.cur]; !c.missing && len(c.anims) > 0 {
		switch {
		case inpututil.IsKeyJustPressed(ebiten.KeyBracketRight):
			c.anim = (c.anim + 1) % len(c.anims)
			c.player.Play(c.pack.Clip(c.anims[c.anim], z.dir))
		case inpututil.IsKeyJustPressed(ebiten.KeyBracketLeft):
			c.anim = (c.anim - 1 + len(c.anims)) % len(c.anims)
			c.player.Play(c.pack.Clip(c.anims[c.anim], z.dir))
		}
	}

	if !z.pause {
		for i := range z.cells {
			if c := &z.cells[i]; !c.missing {
				c.player.Update()
				// Незацикленные клипы (death, hurt) в зоопарке крутим по кругу —
				// иначе половина сетки замирает на последнем кадре.
				if c.player.Finished() {
					c.player.Play(c.player.Clip())
				}
			}
		}
	}
	return z, nil
}

var (
	zooBG      = color.RGBA{0x10, 0x12, 0x1a, 0xff}
	zooCellBG  = color.RGBA{0x18, 0x1c, 0x28, 0xff}
	zooSel     = color.RGBA{0xf2, 0xc0, 0x50, 0xff}
	zooWild    = color.RGBA{0x8c, 0xd8, 0x7a, 0xff}
	zooTame    = color.RGBA{0xd8, 0xc0, 0x90, 0xff}
	zooDim     = color.RGBA{0x70, 0x78, 0x90, 0xff}
	zooBad     = color.RGBA{0xe0, 0x60, 0x60, 0xff}
	zooAttacks = color.RGBA{0xe8, 0x80, 0x50, 0xff}
)

func (z *Zoo) Draw(screen *ebiten.Image) {
	screen.Fill(zooBG)

	for i := range z.cells {
		c := &z.cells[i]
		x := float32((i % zooCols) * zooCellW)
		y := float32(zooTop + (i/zooCols)*zooCellH)
		vector.FillRect(screen, x+1, y+1, zooCellW-2, zooCellH-2, zooCellBG, false)

		base := y + zooCellH - 14 // общая «земля»: животных удобно сравнивать по росту
		if c.missing {
			ui.TextCentered(screen, "нет пака", int(x)+zooCellW/2, int(base)-10)
		} else if img := c.player.Frame(); img != nil {
			b := img.Bounds()
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Translate(
				float64(x)+float64(zooCellW-b.Dx())/2,
				float64(base)-float64(b.Dy()),
			)
			screen.DrawImage(img, op)
		}

		name := c.id
		if len(name) > 15 {
			name = name[:15]
		}
		col := zooTame
		switch {
		case c.missing:
			col = zooBad
		case c.sp.Wild():
			col = zooWild
		}
		ui.DrawTextCentered(screen, name, float64(x)+zooCellW/2, float64(y)+zooCellH-13, col)

		if !c.missing && len(c.anims) > 0 {
			ui.DrawTextCentered(screen, c.anims[c.anim], float64(x)+zooCellW/2, float64(y)+zooCellH-4, zooDim)
		}
		if !c.missing && c.sp.Threat.Attacks {
			ui.DrawText(screen, "!", float64(x)+4, float64(y)+2, zooAttacks)
		}
		if i == z.cur {
			vector.StrokeRect(screen, x+1, y+1, zooCellW-2, zooCellH-2, 1, zooSel, false)
		}
	}

	ui.DrawText(screen, z.header(), 4, 2, zooSel)
}

// header — строка состояния: что выбрано и что про это говорят данные вида.
func (z *Zoo) header() string {
	c := &z.cells[z.cur]
	if c.missing {
		return fmt.Sprintf("%s — %s", c.id, c.err)
	}
	s := c.sp
	stage := "взрослый"
	if s.Young() {
		stage = "детёныш -> " + s.GrowsInto
	}
	kind := "домашнее"
	if s.Wild() {
		kind = "дикое"
	}
	def := s.Threat
	atk := "не нападает"
	if def.Attacks {
		atk = "нападает в ответ"
		if def.Unprovoked {
			atk = "нападает первым"
		}
		if !c.pack.Has("attack") {
			atk += " (без клипа attack)"
		}
	}
	return fmt.Sprintf("%s (%s)  %s, %s, %s  hp%d  [%s %d кадр.]  dir:%s  %s",
		s.Title.RU, c.id, kind, stage, atk, s.Stats.HP,
		c.anims[c.anim], len(c.player.Clip().Frames), z.dir,
		map[bool]string{true: "ПАУЗА", false: ""}[z.pause])
}
