package mob

// Внутренний тест: правило «не лезь под своего» — это одна функция управления,
// и проверять её надо на ней самой. Снаружи такую расстановку не собрать —
// сосед, стоящий вплотную к цели, сам занимает очередь и бьёт, а правило
// касается тех, кто ждёт.

import (
	"math/rand/v2"
	"testing"

	"github.com/vladislav/game/internal/engine"
	"github.com/vladislav/game/internal/sprite"
)

// coneEnemy собирает особь с голым паком: для проверки управления нужны только
// позиция, радиус тела (он считается по рамке пака) и состояние.
func coneEnemy(pos engine.Vec2, w int) *Enemy {
	ty := &EnemyType{}
	ty.Threat.Reach = 30
	pack := &sprite.Pack{Manifest: sprite.Manifest{BBox: &sprite.Rect{W: w, H: w}}}
	return &Enemy{
		Type: ty, Tier: &Tier{Type: ty}, Pack: pack, Pos: pos,
		rng: rand.New(rand.NewPCG(1, 2)),
	}
}

func TestClearOfAllies(t *testing.T) {
	sq := NewSquad()
	// Бьющий смотрит вправо и уже машет.
	attacker := coneEnemy(engine.Vec2{X: 100, Y: 100}, 48)
	attacker.state, attacker.atkFace = EAttack, engine.Vec2{X: 1}
	sq.Add(attacker)

	inLine := coneEnemy(engine.Vec2{X: 130, Y: 100}, 48) // прямо под удар
	behind := coneEnemy(engine.Vec2{X: 70, Y: 100}, 48)  // за спиной бьющего
	far := coneEnemy(engine.Vec2{X: 400, Y: 100}, 48)    // далеко впереди
	for _, e := range []*Enemy{inLine, behind, far} {
		sq.Add(e)
	}
	c := EnemyCtx{Squad: sq}

	push := inLine.clearOfAllies(c)
	if push.Len() == 0 {
		t.Fatal("стоящего под ударом не уводит с линии")
	}
	if absf(push.X) > absf(push.Y) {
		t.Errorf("уводит вдоль удара, а не вбок: %v", push)
	}
	if p := behind.clearOfAllies(c); p.Len() != 0 {
		t.Errorf("стоящего за спиной уводит зря: %v", p)
	}
	if p := far.clearOfAllies(c); p.Len() != 0 {
		t.Errorf("стоящего вне досягаемости уводит зря: %v", p)
	}

	// Бьющий опустил оружие — повода уходить больше нет.
	attacker.state = ECircle
	if p := inLine.clearOfAllies(c); p.Len() != 0 {
		t.Errorf("уводит из-под несуществующего замаха: %v", p)
	}
}

func absf(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
