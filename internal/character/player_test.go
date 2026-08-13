package character_test

import (
	"testing"

	"github.com/vladislav/game/internal/character"
	"github.com/vladislav/game/internal/engine"
	"github.com/vladislav/game/internal/sprite"
)

// plainWorld — вся суша, воды нет.
type plainWorld struct{}

func (plainWorld) Walkable(engine.Vec2) bool { return true }
func (plainWorld) Water(engine.Vec2) bool    { return false }

// pondWorld — правее x=200 вода: персонаж плавать не умеет и обязан упереться.
type pondWorld struct{}

func (pondWorld) Walkable(engine.Vec2) bool { return true }
func (pondWorld) Water(p engine.Vec2) bool  { return p.X > 200 }

var (
	right = engine.Vec2{X: 1}
	start = engine.Vec2{X: 100, Y: 100}
)

// player собирает персонажа заданной пары.
func player(t *testing.T, body, loadout string) *character.Player {
	t.Helper()
	l, cat := catalog(t)
	b, ld := cat.Body(body), cat.Loadout(loadout)
	if b == nil || ld == nil {
		t.Fatalf("нет пары %s/%s", body, loadout)
	}
	pack, err := character.LoadPack(l, characterDir, b, ld)
	if err != nil {
		t.Fatal(err)
	}
	return character.NewPlayer(cat, b, ld, pack, start)
}

// each обходит все пары «тело × лоадаут».
func each(t *testing.T, fn func(t *testing.T, name string, p *character.Player)) {
	t.Helper()
	_, cat := catalog(t)
	for _, bid := range cat.BodyIDs() {
		for _, lid := range cat.LoadoutIDs() {
			name := bid + "/" + lid
			t.Run(name, func(t *testing.T) { fn(t, name, player(t, bid, lid)) })
		}
	}
}

// run прокручивает n тиков с одним и тем же вводом, забирая удары.
func run(p *character.Player, in character.Input, w character.World, n int) []character.Hit {
	var hits []character.Hit
	for range n {
		p.Update(in, w)
		if h, ok := p.Strike(); ok {
			hits = append(hits, h)
		}
		in.Attack = false // удар — фронт нажатия, а не удержание
	}
	return hits
}

// TestWalkAndRun — персонаж идёт туда, куда просят, разворачивается и бежит
// быстрее, чем шагает.
func TestWalkAndRun(t *testing.T) {
	each(t, func(t *testing.T, name string, p *character.Player) {
		run(p, character.Input{Move: right}, plainWorld{}, 60)
		walked := p.Pos.X - start.X
		if walked <= 0 {
			t.Fatalf("%s: за секунду ходьбы вправо не сдвинулся (x=%.1f)", name, p.Pos.X)
		}
		if p.Dir() != sprite.Right {
			t.Errorf("%s: идёт вправо, а смотрит %v", name, p.Dir())
		}
		if p.State() != character.Walk {
			t.Errorf("%s: состояние %v вместо walk", name, p.State())
		}

		p.Revive(start)
		run(p, character.Input{Move: right, Run: true}, plainWorld{}, 60)
		ran := p.Pos.X - start.X
		if ran <= walked {
			t.Errorf("%s: бег %.1f px не быстрее шага %.1f px", name, ran, walked)
		}
		if p.State() != character.Run {
			t.Errorf("%s: состояние %v вместо run", name, p.State())
		}
	})
}

// TestIdleOnNoInput — без ввода персонаж стоит.
func TestIdleOnNoInput(t *testing.T) {
	each(t, func(t *testing.T, name string, p *character.Player) {
		run(p, character.Input{}, plainWorld{}, 30)
		if p.State() != character.Idle {
			t.Errorf("%s: без ввода состояние %v", name, p.State())
		}
		if p.Pos != start {
			t.Errorf("%s: без ввода уехал в %v", name, p.Pos)
		}
	})
}

// TestAlwaysHasFrame — любое состояние у любой пары отрисовывается. Это
// проверка деградации: у unarmed нет ни attack, ни walk_attack, но пустого
// кадра быть не должно ни на одном тике.
func TestAlwaysHasFrame(t *testing.T) {
	each(t, func(t *testing.T, name string, p *character.Player) {
		w := plainWorld{}
		check := func(stage string) {
			if p.Frame() == nil && p.Alive() {
				t.Fatalf("%s: %s (состояние %v, клип %q) — нечего рисовать",
					name, stage, p.State(), p.Clip())
			}
		}
		for _, in := range []character.Input{
			{},
			{Move: right},
			{Move: right, Run: true},
			{Attack: true},
			{Move: right, Attack: true},
			{Move: right, Run: true, Attack: true},
		} {
			for range 90 {
				p.Update(in, w)
				p.Strike()
				in.Attack = false
				check("ввод")
			}
		}
		p.Damage(1, engine.Vec2{})
		for range 30 {
			p.Update(character.Input{}, w)
			check("после удара")
		}
		p.Kill()
		for range 30 {
			p.Update(character.Input{}, w)
			check("смерть")
		}
	})
}

// TestAttackStrikesOnce — за один замах ровно один удар, и он накрывает цель
// перед персонажем, но не за спиной. Верно и для unarmed, у которого клипа
// удара нет вовсе: намерение не зависит от наличия анимации.
func TestAttackStrikesOnce(t *testing.T) {
	each(t, func(t *testing.T, name string, p *character.Player) {
		hits := run(p, character.Input{Attack: true, Aim: engine.Vec2{X: 300, Y: 100}, HasAim: true},
			plainWorld{}, 120)
		if len(hits) != 1 {
			t.Fatalf("%s: за один замах %d ударов, ожидался один", name, len(hits))
		}
		h := hits[0]
		if h.Damage != p.Loadout.Attack.Damage {
			t.Errorf("%s: урон %d вместо %d", name, h.Damage, p.Loadout.Attack.Damage)
		}
		front := start.Add(engine.Vec2{X: h.Reach - 1})
		back := start.Add(engine.Vec2{X: -(h.Reach - 1)})
		if !h.Covers(front, 0) {
			t.Errorf("%s: цель перед носом (%v) не задета сектором reach=%.0f arc=%.0f",
				name, front, h.Reach, h.Arc)
		}
		if h.Covers(back, 0) {
			t.Errorf("%s: цель за спиной (%v) задета", name, back)
		}
		if far := start.Add(engine.Vec2{X: h.Reach + 20}); h.Covers(far, 0) {
			t.Errorf("%s: цель за пределами reach задета", name)
		}
	})
}

// TestAttackRate — удержание удара даёт серию замахов, но не чаще темпа,
// который задают данные. Темп — это максимум из длительности замаха и
// перезарядки: следующий замах не начнётся ни посреди предыдущего, ни раньше
// cooldown_ticks.
func TestAttackRate(t *testing.T) {
	each(t, func(t *testing.T, name string, p *character.Player) {
		const ticks = 300
		a := p.Loadout.Attack
		period := max(a.SwingTicks, a.CooldownTicks)

		hits := 0
		for range ticks {
			p.Update(character.Input{Attack: true}, plainWorld{})
			if _, ok := p.Strike(); ok {
				hits++
			}
		}
		if hits < 2 {
			t.Fatalf("%s: за %d тиков удержания прошло %d ударов — серии нет", name, ticks, hits)
		}
		if maxHits := ticks/period + 1; hits > maxHits {
			t.Errorf("%s: %d ударов за %d тиков — чаще темпа (замах %d, перезарядка %d)",
				name, hits, ticks, a.SwingTicks, a.CooldownTicks)
		}
	})
}

// TestAttackBlockedWhileBusy — в замахе и в оцепенении новый удар не начать.
func TestAttackBlockedWhileBusy(t *testing.T) {
	each(t, func(t *testing.T, name string, p *character.Player) {
		p.Update(character.Input{Attack: true}, plainWorld{})
		if p.State() != character.Attacking {
			t.Fatalf("%s: удар не начался, состояние %v", name, p.State())
		}
		if p.CanAttack() {
			t.Errorf("%s: посреди замаха готов бить снова", name)
		}
		p.Damage(3, start)
		if p.State() != character.Hurt {
			t.Fatalf("%s: удар по герою не сбил замах (состояние %v)", name, p.State())
		}
		if p.CanAttack() {
			t.Errorf("%s: в оцепенении готов бить", name)
		}
		if _, ok := p.Strike(); ok {
			t.Errorf("%s: сбитый замах всё равно нанёс урон", name)
		}
	})
}

// TestAttackOnMove — лоадаут с ударом на ходу продолжает двигаться в замахе,
// лоадаут без него останавливается.
func TestAttackOnMove(t *testing.T) {
	each(t, func(t *testing.T, name string, p *character.Player) {
		in := character.Input{Move: right, Attack: true}
		p.Update(in, plainWorld{}) // старт замаха
		in.Attack = false
		at := p.Pos.X
		for range 10 {
			p.Update(in, plainWorld{})
			p.Strike()
		}
		moved := p.Pos.X - at
		if p.Loadout.Attack.OnMove {
			if moved <= 0 {
				t.Errorf("%s: on_move=true, но в замахе стоит на месте", name)
			}
		} else if moved != 0 {
			t.Errorf("%s: on_move=false, но в замахе проехал %.2f px", name, moved)
		}
	})
}

// TestHurtLocksAndInvuln — удар оцепеняет и даёт кадры неуязвимости: второй
// удар подряд не проходит, ввод в оцепенении не слушается.
func TestHurtLocksAndInvuln(t *testing.T) {
	each(t, func(t *testing.T, name string, p *character.Player) {
		hp := p.HP
		if p.Damage(7, start.Add(engine.Vec2{X: -20})) {
			t.Fatalf("%s: 7 урона убили героя со %d hp", name, hp)
		}
		if p.HP != hp-7 {
			t.Errorf("%s: hp %d вместо %d", name, p.HP, hp-7)
		}
		if p.State() != character.Hurt {
			t.Errorf("%s: состояние %v вместо hurt", name, p.State())
		}
		if p.Damage(7, start) {
			t.Errorf("%s: добит вторым ударом сквозь неуязвимость", name)
		}
		if p.HP != hp-7 {
			t.Errorf("%s: урон прошёл сквозь неуязвимость (hp %d)", name, p.HP)
		}
		// Отброс идёт от источника (слева), то есть вправо, а не по вводу влево.
		run(p, character.Input{Move: engine.Vec2{X: -1}}, plainWorld{}, 5)
		if p.Pos.X < start.X {
			t.Errorf("%s: в оцепенении послушался ввода (x=%.1f < %.1f)", name, p.Pos.X, start.X)
		}
		// Оцепенение конечно.
		run(p, character.Input{}, plainWorld{}, p.Cat.Base.Hurt.LockTicks+2)
		if p.State() == character.Hurt {
			t.Errorf("%s: оцепенение не кончилось за lock_ticks", name)
		}
	})
}

// TestDeathEnds — смерть доигрывается и труп убирается даже у пар, где клип
// death короткий; воскрешение возвращает персонажа в игру.
func TestDeathEnds(t *testing.T) {
	each(t, func(t *testing.T, name string, p *character.Player) {
		p.Kill()
		if p.Alive() {
			t.Fatalf("%s: пережил Kill", name)
		}
		w := plainWorld{}
		gone := false
		for range 600 {
			p.Update(character.Input{Move: right, Attack: true}, w)
			if p.Gone() {
				gone = true
				break
			}
		}
		if !gone {
			t.Fatalf("%s: труп не растаял за 600 тиков", name)
		}
		if p.Pos != start {
			t.Errorf("%s: труп уехал по вводу в %v", name, p.Pos)
		}
		p.Revive(start)
		if !p.Alive() || p.Gone() || p.HP != p.MaxHP {
			t.Errorf("%s: после Revive: alive=%v gone=%v hp=%d/%d",
				name, p.Alive(), p.Gone(), p.HP, p.MaxHP)
		}
		run(p, character.Input{Move: right}, w, 30)
		if p.Pos.X <= start.X {
			t.Errorf("%s: после Revive не двигается", name)
		}
	})
}

// TestWaterBlocks — плавать персонаж не умеет, значит в воду не заходит.
func TestWaterBlocks(t *testing.T) {
	each(t, func(t *testing.T, name string, p *character.Player) {
		p.Revive(engine.Vec2{X: 190, Y: 100})
		run(p, character.Input{Move: right, Run: true}, pondWorld{}, 300)
		if p.Pos.X > 200 {
			t.Errorf("%s: зашёл в воду (x=%.1f)", name, p.Pos.X)
		}
		if p.Pos.X <= 190 {
			t.Errorf("%s: упёрся в берег, не дойдя до него (x=%.1f)", name, p.Pos.X)
		}
	})
}

// TestEquipKeepsCharacter — смена оружия меняет пак и параметры удара, но не
// трогает позицию и здоровье, и не оставляет персонажа без кадра.
func TestEquipKeepsCharacter(t *testing.T) {
	l, cat := catalog(t)
	p := player(t, "male", "unarmed")
	run(p, character.Input{Move: right}, plainWorld{}, 20)
	p.Damage(10, start)
	run(p, character.Input{}, plainWorld{}, p.Cat.Base.Hurt.LockTicks+2) // переждать оцепенение
	pos, hp := p.Pos, p.HP

	sword := cat.Loadout("sword")
	pack, err := character.LoadPack(l, characterDir, p.Body, sword)
	if err != nil {
		t.Fatal(err)
	}
	p.Equip(sword, pack)

	if p.Pos != pos || p.HP != hp {
		t.Errorf("смена оружия сдвинула персонажа: %v/%d вместо %v/%d", p.Pos, p.HP, pos, hp)
	}
	if p.Loadout.ID != "sword" || p.Pack != pack {
		t.Errorf("лоадаут не сменился: %s", p.Loadout.ID)
	}
	hits := run(p, character.Input{Attack: true}, plainWorld{}, 120)
	if len(hits) != 1 || hits[0].Damage != sword.Attack.Damage {
		t.Errorf("после смены оружия удары %v, ожидался один на %d урона", hits, sword.Attack.Damage)
	}
	if p.Frame() == nil {
		t.Error("после смены оружия нечего рисовать")
	}
}

// TestSetBodyKeepsHealthShare — смена тела сохраняет долю здоровья, а не число.
func TestSetBodyKeepsHealthShare(t *testing.T) {
	l, cat := catalog(t)
	p := player(t, "male", "sword")
	p.Damage(p.MaxHP/2, start)
	share := float64(p.HP) / float64(p.MaxHP)

	female := cat.Body("female")
	pack, err := character.LoadPack(l, characterDir, female, p.Loadout)
	if err != nil {
		t.Fatal(err)
	}
	p.SetBody(female, pack)

	if got := float64(p.HP) / float64(p.MaxHP); got < share-0.02 || got > share+0.02 {
		t.Errorf("доля здоровья после смены тела %.2f вместо %.2f", got, share)
	}
	if p.MaxHP != max(1, int(float64(cat.Base.HP)*female.HPScale)) {
		t.Errorf("макс. здоровье %d не пересчитано под тело", p.MaxHP)
	}
}
