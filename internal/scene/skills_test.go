package scene

import (
	"os"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/vladislav/game/internal/assets"
	"github.com/vladislav/game/internal/character"
	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/engine"
	"github.com/vladislav/game/internal/mob"
)

// power — камень с силой на n ударов.
func power(n int) *mob.Stolen {
	return &mob.Stolen{
		Type: "goblin", Tier: "t1", Title: "СИЛА: ГОБЛИН", Element: "slash",
		Attack:  mob.PowerAttack{Damage: 10, Reach: 21, Arc: 73, SwingTicks: 22, CooldownTicks: 24},
		Charges: n,
	}
}

// TestSkillBarFillsAndEmpties — камни ложатся по ячейкам подряд, четвёртому
// места нет, а истраченный камень освобождает свою ячейку.
func TestSkillBarFillsAndEmpties(t *testing.T) {
	var b skillBar
	for i := range skillSlots {
		if !b.put(power(2)) {
			t.Fatalf("камень %d не влез в пустые ячейки", i+1)
		}
	}
	if b.put(power(2)) {
		t.Error("четвёртый камень влез в три ячейки")
	}

	if s := b.spend(0); s == nil || s.Charges != 1 {
		t.Fatalf("после удара в ячейке %+v", s)
	}
	if b.slots[0].Empty() {
		t.Error("камень с оставшимся зарядом исчез")
	}
	if s := b.spend(0); s == nil || s.Charges != 0 {
		t.Fatalf("второй удар дал %+v", s)
	}
	if !b.slots[0].Empty() {
		t.Error("истраченный камень остался в ячейке")
	}
	if b.spend(0) != nil {
		t.Error("пустая ячейка всё ещё бьёт")
	}
	if !b.put(power(3)) {
		t.Error("освободившаяся ячейка не принимает новый камень")
	}
}

// TestStoneGoesToSkillSlot — камень с земли уходит в ячейку умений, а не в
// сумку: сумка его не примет, а потерять силу нельзя.
func TestStoneGoesToSkillSlot(t *testing.T) {
	l := assets.NewLoader(os.DirFS(testAssets))
	g, err := NewGame(l, nil, "male")
	if err != nil {
		t.Fatalf("забег не собрался: %v", err)
	}
	before := g.bag.Count("stone_slash")
	g.dropStone(g.pl.Pos, g.pl.Floor(), power(4))
	if len(g.drops) != 1 {
		t.Fatalf("камень не лёг на землю: %d вещей", len(g.drops))
	}

	// Герой стоит прямо над камнем — хватает нескольких тиков притяжения.
	for range 4 * 60 {
		g.updateLoot()
		if len(g.drops) == 0 {
			break
		}
	}
	if len(g.drops) != 0 {
		t.Fatalf("камень так и не подобрался")
	}
	if g.skills.slots[0].Empty() {
		t.Error("камень подобран, но ячейка умений пуста")
	}
	if g.bag.Count("stone_slash") != before {
		t.Error("камень умения оказался в сумке")
	}
}

// TestCastSpendsChargeAndHits — удар украденной силой тратит заряд и приходит
// поверх собственного урона героя, а не вместо него.
func TestCastSpendsChargeAndHits(t *testing.T) {
	l := assets.NewLoader(os.DirFS(testAssets))
	g, err := NewGame(l, nil, "male")
	if err != nil {
		t.Fatalf("забег не собрался: %v", err)
	}
	s := power(3)
	g.skills.put(s)

	spell := &character.Spell{Attack: spellAttack(s), Damage: s.Rolls()}
	if got := g.skills.spend(0); got == nil || got.Charges != 2 {
		t.Fatalf("заряд не потратился: %+v", got)
	}

	// Замах чужой силой: урон должен быть не меньше собственного, а сектор —
	// чужой геометрии.
	var hits []character.Hit
	for range 120 {
		g.pl.Update(character.Input{Cast: spell}, g.m.Field())
		if h, ok := g.pl.Strike(); ok {
			hits = append(hits, h)
		}
		spell = nil // просьба — фронт нажатия, а не удержание
	}
	if len(hits) == 0 {
		t.Fatal("удар чужой силой не состоялся")
	}
	h := hits[0]
	own := g.pl.Power().Damage
	if h.Damage.Total() <= own.Physical.Max {
		t.Errorf("украденный удар %d не сильнее собственного (%s)", h.Damage.Total(), own.Physical)
	}
	if want := s.Attack.Reach + g.pl.Radius(); h.Reach != want {
		t.Errorf("сектор удара %.0f вместо чужого %.0f", h.Reach, want)
	}
}

// TestSkillsSurviveSave — украденные умения переживают выход из забега вместе с
// остатком зарядов и порядком ячеек.
func TestSkillsSurviveSave(t *testing.T) {
	l := assets.NewLoader(os.DirFS(testAssets))
	g, err := NewGame(l, nil, "male")
	if err != nil {
		t.Fatalf("забег не собрался: %v", err)
	}
	g.char = NewChar("тест", "male")
	live := g.es.Power("goblin", "t1")
	if live == nil {
		t.Skip("в таблице нет гоблинов с силой")
	}
	live.Charges = 2
	g.skills.slots[1] = live
	g.dropStone(engine.Vec2{X: g.pl.Pos.X + 40, Y: g.pl.Pos.Y}, g.pl.Floor(), power(5))
	g.snapshotWorld()

	if n := len(g.char.Skills); n != skillSlots {
		t.Fatalf("сохранено %d ячеек вместо %d", n, skillSlots)
	}
	if sk := g.char.Skills[1]; sk.Type != "goblin" || sk.Left != 2 {
		t.Errorf("сохранена не та сила: %+v", sk)
	}
	if g.char.Skills[0].Type != "" {
		t.Error("пустая ячейка сохранилась занятой")
	}
	var stones int
	for _, d := range g.char.Ground {
		if d.Skill != nil {
			stones++
		}
	}
	if stones != 1 {
		t.Errorf("камней на земле сохранено %d, ожидался один", stones)
	}

	g.skills = skillBar{}
	g.drops = nil
	g.restoreWorld()
	if got := g.skills.slots[1]; got.Empty() || got.Charges != 2 || got.Type != "goblin" {
		t.Errorf("сила вернулась не той: %+v", got)
	}
	if !g.skills.slots[0].Empty() {
		t.Error("пустая ячейка вернулась занятой")
	}
}

// TestDrawRunsHeadless — забег рисуется целиком, не падая: у ячеек умений,
// камней и полос нет ни одного пути, который бы этого не пережил.
func TestDrawRunsHeadless(t *testing.T) {
	l := assets.NewLoader(os.DirFS(testAssets))
	g, err := NewGame(l, nil, "male")
	if err != nil {
		t.Fatalf("забег не собрался: %v", err)
	}
	g.skills.put(power(3))
	g.dropStone(g.pl.Pos, g.pl.Floor(), power(2))
	dst := ebiten.NewImage(config.ScreenW, config.ScreenH)
	g.Draw(dst)
}
