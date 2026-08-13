// Package anim — единый механизм покадровой (спрайтовой) анимации.
// Clip хранит кадры и тайминг, Player проигрывает их во времени. Здесь нет
// никакой предметной логики (герои, действия, направления) — только кадры и
// время, поэтому один и тот же механизм годится для героев, мобов, снарядов,
// UI-эффектов и всего, что анимируется по кадрам.
package anim

import "github.com/hajimehoshi/ebiten/v2"

// Clip — последовательность кадров с одинаковой длительностью каждого кадра.
// Тайминг задаётся в тиках логики (при config.TPS == 60 один тик = 1/60 c).
type Clip struct {
	Frames     []*ebiten.Image
	FrameTicks int  // тиков на кадр (минимум 1)
	Loop       bool // зациклен ли клип
}

// Valid сообщает, можно ли проигрывать клип. Безопасен на nil-приёмнике.
func (c *Clip) Valid() bool { return c != nil && len(c.Frames) > 0 }

// Ticks — длительность одного полного прохода клипа в тиках.
func (c *Clip) Ticks() int {
	if !c.Valid() {
		return 0
	}
	return len(c.Frames) * c.frameTicks()
}

// Retimed — тот же клип с другой длительностью кадра: ускоренная походка вместо
// бега, замедленный бег вместо походки. Кадры общие с исходным клипом,
// копируется только тайминг, поэтому подмена ничего не стоит по памяти.
func (c *Clip) Retimed(frameTicks int) *Clip {
	if !c.Valid() {
		return nil
	}
	return &Clip{Frames: c.Frames, FrameTicks: max(1, frameTicks), Loop: c.Loop}
}

func (c *Clip) frameTicks() int {
	if c.FrameTicks < 1 {
		return 1
	}
	return c.FrameTicks
}

// Player проигрывает Clip: считает тики и отдаёт текущий кадр. Один Player —
// одно независимое состояние проигрывания, поэтому у каждой анимированной
// сущности (или карточки в меню) он свой.
type Player struct {
	clip     *Clip
	tick     int
	frame    int
	finished bool
}

// NewPlayer создаёт проигрыватель, сразу заряженный клипом c (может быть nil).
func NewPlayer(c *Clip) *Player {
	p := &Player{}
	p.Play(c)
	return p
}

// Play заряжает новый клип и сбрасывает проигрывание в начало.
func (p *Player) Play(c *Clip) {
	p.clip = c
	p.tick = 0
	p.frame = 0
	p.finished = false
}

// Update продвигает анимацию на один тик логики.
func (p *Player) Update() {
	if !p.clip.Valid() || p.finished {
		return
	}
	p.tick++
	if p.tick < p.clip.frameTicks() {
		return
	}
	p.tick = 0
	p.frame++
	if p.frame < len(p.clip.Frames) {
		return
	}
	if p.clip.Loop {
		p.frame = 0
	} else {
		p.frame = len(p.clip.Frames) - 1
		p.finished = true
	}
}

// Frame — текущий кадр (nil, если клип пуст/не задан).
func (p *Player) Frame() *ebiten.Image {
	if !p.clip.Valid() {
		return nil
	}
	return p.clip.Frames[p.frame]
}

// Index — индекс текущего кадра.
func (p *Player) Index() int { return p.frame }

// Finished — true, если незацикленный клип доигран до последнего кадра.
func (p *Player) Finished() bool { return p.finished }

// Clip — текущий клип (может быть nil).
func (p *Player) Clip() *Clip { return p.clip }
