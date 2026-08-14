package scene

import (
	"os"
	"testing"

	"github.com/vladislav/game/internal/assets"
	"github.com/vladislav/game/internal/character"
	"github.com/vladislav/game/internal/combat"
	"github.com/vladislav/game/internal/engine"
	"github.com/vladislav/game/internal/progress"
)

// sweep — удар, накрывающий всю карту: тесту важно не попадание, а то, что
// случается после смерти цели.
func sweep(from engine.Vec2) character.Hit {
	return character.Hit{
		Center: from, Face: engine.Vec2{X: 0, Y: 1},
		Reach: 1e6, Arc: 360, Damage: combat.Damage{Physical: 1e6},
	}
}

// TestKillGivesXP — добитый враг отдаёт опыт герою, и опыт складывается в
// уровни. Проверяется вся связка сцены: удар → смерть → награда → полоса.
func TestKillGivesXP(t *testing.T) {
	l := assets.NewLoader(os.DirFS("../../assets"))
	g, err := NewGame(l, nil, "male")
	if err != nil {
		t.Fatal(err)
	}
	if g.prog.Level != 1 || g.prog.XP != 0 {
		t.Fatalf("забег начинается не с первого уровня: %+v", g.prog)
	}
	live := 0
	for _, e := range g.es.Enemies() {
		if e.Alive() {
			live++
		}
	}
	if live == 0 {
		t.Fatal("карта заселена без живых врагов")
	}

	g.strikeEnemies(sweep(g.pl.Pos))

	for _, e := range g.es.Enemies() {
		if e.Alive() {
			t.Fatalf("враг пережил удар на миллион: %s", e.Tier.Title.RU)
		}
	}
	if g.prog.Level == 1 && g.prog.XP == 0 {
		t.Fatalf("за %d убитых врагов не дали ни очка опыта", live)
	}
	if g.prog.Points != g.prog.Level-1 {
		t.Errorf("уровень %d, а очков прокачки %d", g.prog.Level, g.prog.Points)
	}
}

// TestOutlevelledGiveNothing — герой, переросший всю карту, не получает ничего.
// Это и есть потолок локации: он держится на разнице уровней, а не на счётчике.
func TestOutlevelledGiveNothing(t *testing.T) {
	l := assets.NewLoader(os.DirFS("../../assets"))
	g, err := NewGame(l, nil, "male")
	if err != nil {
		t.Fatal(err)
	}
	// Выше самого сильного лесного врага более чем на OutlevelGap.
	top := 0
	for _, e := range g.es.Enemies() {
		top = max(top, e.Level)
	}
	g.prog = progress.Character{Level: top + progress.OutlevelGap}

	g.strikeEnemies(sweep(g.pl.Pos))

	if g.prog.XP != 0 || g.prog.Level != top+progress.OutlevelGap {
		t.Errorf("переросшему карту всё-таки дали опыт: %+v (сильнейший враг %d ур)", g.prog, top)
	}
}

// TestAnimalKillGivesXP — звери платят по тем же правилам, что и враги: это
// один и тот же путь награды, и разойтись они не должны.
func TestAnimalKillGivesXP(t *testing.T) {
	l := assets.NewLoader(os.DirFS("../../assets"))
	g, err := NewGame(l, nil, "male")
	if err != nil {
		t.Fatal(err)
	}
	live := 0
	for _, a := range g.sp.Animals() {
		if a.Alive() {
			live++
			if a.XP <= 0 {
				t.Fatalf("зверь %s без опыта", a.Species.ID)
			}
		}
	}
	if live == 0 {
		t.Skip("на этой карте зверей не оказалось")
	}

	// Удар раздаётся вручную: strike() забирает его у автомата персонажа, а тот
	// по заказу не бьёт.
	h := sweep(g.pl.Pos)
	for _, a := range g.sp.Animals() {
		if !a.Alive() || !h.Covers(a.Pos, a.Radius()) {
			continue
		}
		if a.Hit(h.Damage.Total()) {
			g.reward(g.headOf(a), a.Level, a.XP)
		}
	}
	if g.prog.Level == 1 && g.prog.XP == 0 {
		t.Errorf("за %d убитых зверей не дали ни очка опыта", live)
	}
}
