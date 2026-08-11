package scene

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/vladislav/game/internal/anim"
	"github.com/vladislav/game/internal/hero"
)

// loopReplays — сколько раз проигрывать зацикленное действие (idle/walk),
// прежде чем перейти к следующему.
const loopReplays = 2

// holdTicks — пауза на последнем кадре незацикленного действия перед переходом.
const holdTicks = 30

// facingView — вид в галерее: направление в ассетах + нужен ли флип + подпись.
// Бок (side) переиспользует фронт (Down) с горизонтальным зеркалом.
type facingView struct {
	f     hero.Facing
	flip  bool
	label string
}

var facingViews = []facingView{
	{hero.Down, false, "front"},
	{hero.Up, false, "back"},
	{hero.Side, false, "side >"},
	{hero.Side, true, "side <"},
}

// showcase проигрывает все действия героя по очереди, зацикливая всю галерею.
// Незацикленные действия (attack/take_damage/loot) играются один раз с паузой,
// зацикленные (idle/walk) — несколько повторов. Направление вида переключается
// отдельно (front/back/side). Логику держит здесь, чтобы сцена занималась
// только вводом и отрисовкой.
type showcase struct {
	h      *hero.Hero
	p      *anim.Player
	order  []hero.Action
	idx    int
	facing int // индекс в facingViews
	timer  int // тиков в текущем действии
}

func newShowcase(h *hero.Hero) *showcase {
	s := &showcase{h: h, order: hero.Actions}
	s.p = anim.NewPlayer(s.clip())
	return s
}

func (s *showcase) clip() *anim.Clip {
	return s.h.Clip(s.order[s.idx], facingViews[s.facing].f)
}

func (s *showcase) Update() {
	c := s.p.Clip()
	if !c.Valid() { // нет кадров у действия — пропускаем
		s.advance()
		return
	}
	s.p.Update()
	s.timer++

	done := false
	switch {
	case c.Loop:
		done = s.timer >= c.Ticks()*loopReplays
	case s.p.Finished():
		done = s.timer >= c.Ticks()+holdTicks
	}
	if done {
		s.advance()
	}
}

// advance переходит к следующему действию, зацикливая галерею.
func (s *showcase) advance() {
	s.idx = (s.idx + 1) % len(s.order)
	s.timer = 0
	s.p.Play(s.clip())
}

// cycleFacing переключает направление вида (front/back/side), сохраняя действие.
func (s *showcase) cycleFacing(delta int) {
	n := len(facingViews)
	s.facing = (s.facing + delta%n + n) % n
	s.p.Play(s.clip())
}

func (s *showcase) action() hero.Action { return s.order[s.idx] }

func (s *showcase) flip() bool { return facingViews[s.facing].flip }

func (s *showcase) facingLabel() string { return facingViews[s.facing].label }

func (s *showcase) Frame() *ebiten.Image { return s.p.Frame() }

func (s *showcase) progress() string {
	return fmt.Sprintf("%d / %d", s.idx+1, len(s.order))
}
