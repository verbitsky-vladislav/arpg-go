package character_test

import (
	"testing"

	"github.com/vladislav/game/internal/character"
	"github.com/vladislav/game/internal/combat"
	"github.com/vladislav/game/internal/engine"
	"github.com/vladislav/game/internal/physics"
	"github.com/vladislav/game/internal/sprite"
)

// plain — поля нет: стен и воды тоже, персонаж ходит свободно. Так же устроен
// просмотрщик и любая сцена без карты.
var plain *physics.Field

// field собирает поле 512×384 px из правила «что в этой точке».
func field(kind func(p engine.Vec2) physics.Cell) *physics.Field {
	const sub = 8.0
	const w, h = 64, 48
	cells := make([]physics.Cell, w*h)
	for sy := range h {
		for sx := range w {
			cells[sy*w+sx] = kind(engine.Vec2{
				X: (float64(sx) + 0.5) * sub,
				Y: (float64(sy) + 0.5) * sub,
			})
		}
	}
	return physics.NewField(w, h, sub, cells)
}

// pond — правее x=200 глубокая вода: персонаж плавать не умеет и обязан
// упереться в берег.
func pond() *physics.Field {
	return field(func(p engine.Vec2) physics.Cell {
		if p.X > 200 {
			return physics.Deep
		}
		return physics.Ground
	})
}

// marsh — та же вода, но с полосой мелководья 200..280: по ней герой бредёт.
func marsh() *physics.Field {
	return field(func(p engine.Vec2) physics.Cell {
		switch {
		case p.X > 280:
			return physics.Deep
		case p.X > 200:
			return physics.Shallow
		}
		return physics.Ground
	})
}

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

// armed — то же по всем парам, но только с лоадаутами, которые вообще бьют:
// безоружному нечем, и проверять на нём темп удара или сектор нечего.
func armed(t *testing.T, fn func(t *testing.T, name string, p *character.Player)) {
	t.Helper()
	each(t, func(t *testing.T, name string, p *character.Player) {
		if !p.Loadout.CanStrike() {
			t.Skip("лоадаут без удара")
		}
		fn(t, name, p)
	})
}

// run прокручивает n тиков с одним и тем же вводом, забирая удары.
func run(p *character.Player, in character.Input, w *physics.Field, n int) []character.Hit {
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
		run(p, character.Input{Move: right}, plain, 60)
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
		run(p, character.Input{Move: right, Run: true}, plain, 60)
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
		run(p, character.Input{}, plain, 30)
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
		w := plain
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
// перед персонажем, но не за спиной. Проверяется на лоадаутах с ударом; клип
// удара при этом не обязателен — намерение не зависит от наличия анимации.
func TestAttackStrikesOnce(t *testing.T) {
	armed(t, func(t *testing.T, name string, p *character.Player) {
		hits := run(p, character.Input{Attack: true, Aim: engine.Vec2{X: 300, Y: 100}, HasAim: true},
			plain, 120)
		if len(hits) != 1 {
			t.Fatalf("%s: за один замах %d ударов, ожидался один", name, len(hits))
		}
		h := hits[0]
		want := p.Power().Damage.Physical
		if h.Damage.Physical < want.Min || h.Damage.Physical > want.Max {
			t.Errorf("%s: урон %d вне диапазона %s", name, h.Damage.Physical, want)
		}
		if h.Damage.Total() != h.Damage.Physical {
			t.Errorf("%s: удар без оружия нанёс не только физический урон: %+v", name, h.Damage)
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

// TestWeaponReplacesBaseDamage — базовый урон это урон «без всего»: вещь в руке
// замещает его целиком, и палка на 2-4 бьёт ровно на 2-4. Иначе число на экране
// не сходилось бы с числом в таблице.
func TestWeaponReplacesBaseDamage(t *testing.T) {
	armed(t, func(t *testing.T, name string, p *character.Player) {
		base := p.Cat.Base.Damage.Physical
		bare := hitDamage(t, name, p)
		if bare.Physical < base.Min || bare.Physical > base.Max {
			t.Errorf("%s: голыми руками %d, а базовый урон %s", name, bare.Physical, base)
		}

		stick := combat.Roll{Min: 2, Max: 4}
		p.SetWeapon(&combat.Weapon{
			Speed: 1, Shape: combat.ShapeSingle,
			Damage: combat.Rolls{Physical: stick},
		})
		for range 8 {
			p.Revive(start)
			got := hitDamage(t, name, p)
			if got.Physical < stick.Min || got.Physical > stick.Max {
				t.Fatalf("%s: с оружием %d вне %s — база подмешалась к урону вещи",
					name, got.Physical, stick)
			}
		}
	})
}

// TestWeaponWithoutDamageKeepsBase — вещь без своего урона базу не отменяет:
// ею бьют как рукой, разница только в анимации.
func TestWeaponWithoutDamageKeepsBase(t *testing.T) {
	p := player(t, "male", "stick")
	p.SetWeapon(&combat.Weapon{Speed: 1, Shape: combat.ShapeSingle})
	if got := p.Power().Damage; got != p.Cat.Base.Damage {
		t.Errorf("вещь без урона дала %+v вместо базового %+v", got, p.Cat.Base.Damage)
	}
}

// TestAreaWeaponHitsAround — площадное оружие бьёт кругом вокруг героя: и за
// спину достаёт, и сектор лоадаута ему не указ.
func TestAreaWeaponHitsAround(t *testing.T) {
	p := player(t, "male", "stick")
	p.SetWeapon(&combat.Weapon{
		Speed: 1, Shape: combat.ShapeArea, Radius: 60,
		Damage: combat.Rolls{Physical: combat.Roll{Min: 1, Max: 1}},
	})
	hits := run(p, character.Input{Attack: true, Aim: engine.Vec2{X: 300, Y: 100}, HasAim: true},
		plain, 120)
	if len(hits) != 1 {
		t.Fatalf("за один замах %d ударов, ожидался один", len(hits))
	}
	h := hits[0]
	if h.Single {
		t.Error("площадное оружие пришло одиночным ударом")
	}
	if !h.Covers(start.Add(engine.Vec2{X: -50}), 0) {
		t.Errorf("цель за спиной не задета кругом reach=%.0f arc=%.0f", h.Reach, h.Arc)
	}
	if h.Covers(start.Add(engine.Vec2{X: 200}), 0) {
		t.Error("цель за пределами радиуса задета")
	}
}

// TestAttackSpeedShortensSwing — быстрое оружие сжимает и замах, и перезарядку.
func TestAttackSpeedShortensSwing(t *testing.T) {
	slow := player(t, "male", "stick")
	fast := player(t, "male", "stick")
	fast.SetWeapon(&combat.Weapon{
		Speed: 2, Shape: combat.ShapeSingle,
		Damage: combat.Rolls{Physical: combat.Roll{Min: 1, Max: 1}},
	})
	const ticks = 240
	count := func(p *character.Player) int {
		n := 0
		for range ticks {
			p.Update(character.Input{Attack: true}, plain)
			if _, ok := p.Strike(); ok {
				n++
			}
		}
		return n
	}
	s, f := count(slow), count(fast)
	if f <= s {
		t.Errorf("оружие вдвое быстрее дало %d ударов против %d обычных", f, s)
	}
}

// hitDamage — урон одного удара с текущим оружием.
func hitDamage(t *testing.T, name string, p *character.Player) combat.Damage {
	t.Helper()
	hits := run(p, character.Input{Attack: true}, plain, 120)
	if len(hits) != 1 {
		t.Fatalf("%s: за один замах %d ударов, ожидался один", name, len(hits))
	}
	return hits[0].Damage
}

// TestAttackRate — удержание удара даёт серию замахов, но не чаще темпа,
// который задают данные. Темп — это максимум из длительности замаха и
// перезарядки: следующий замах не начнётся ни посреди предыдущего, ни раньше
// cooldown_ticks.
func TestAttackRate(t *testing.T) {
	armed(t, func(t *testing.T, name string, p *character.Player) {
		const ticks = 300
		a := p.Loadout.Attack
		period := max(a.SwingTicks, a.CooldownTicks)

		hits := 0
		for range ticks {
			p.Update(character.Input{Attack: true}, plain)
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
	armed(t, func(t *testing.T, name string, p *character.Player) {
		p.Update(character.Input{Attack: true}, plain)
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
	armed(t, func(t *testing.T, name string, p *character.Player) {
		in := character.Input{Move: right, Attack: true}
		p.Update(in, plain) // старт замаха
		in.Attack = false
		at := p.Pos.X
		for range 10 {
			p.Update(in, plain)
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
		run(p, character.Input{Move: engine.Vec2{X: -1}}, plain, 5)
		if p.Pos.X < start.X {
			t.Errorf("%s: в оцепенении послушался ввода (x=%.1f < %.1f)", name, p.Pos.X, start.X)
		}
		// Оцепенение конечно.
		run(p, character.Input{}, plain, p.Cat.Base.Hurt.LockTicks+2)
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
		w := plain
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

// TestWaterBlocks — плавать персонаж не умеет, значит в глубокую воду не
// заходит. Останавливает его тело, а не точка опоры: между героем и водой
// остаётся его радиус.
func TestWaterBlocks(t *testing.T) {
	each(t, func(t *testing.T, name string, p *character.Player) {
		p.Revive(engine.Vec2{X: 190, Y: 100})
		run(p, character.Input{Move: right, Run: true}, pond(), 300)
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
	run(p, character.Input{Move: right}, plain, 20)
	p.Damage(10, start)
	run(p, character.Input{}, plain, p.Cat.Base.Hurt.LockTicks+2) // переждать оцепенение
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
	hits := run(p, character.Input{Attack: true}, plain, 120)
	if len(hits) != 1 {
		t.Errorf("после смены оружия %d ударов, ожидался один", len(hits))
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

// TestWaterBlocksPlayer — вода непроходима: ни мелководье, ни глубина. Герой
// упирается в берег и остаётся на суше.
//
// Раньше мелководье было вязкой дорогой, и это оказалось плохой идеей на игре:
// вода перестала читаться как граница, и в неё заходили случайно, не понимая,
// что происходит с управлением.
func TestWaterBlocksPlayer(t *testing.T) {
	each(t, func(t *testing.T, name string, p *character.Player) {
		w := marsh()
		p.Revive(engine.Vec2{X: 190, Y: 100})
		run(p, character.Input{Move: right, Run: true}, w, 300)
		if p.Pos.X > 200 {
			t.Errorf("%s: зашёл в воду (x=%.1f)", name, p.Pos.X)
		}
		if p.Pos.X <= 190 {
			t.Errorf("%s: упёрся, не дойдя до берега (x=%.1f)", name, p.Pos.X)
		}
	})
}

// TestAimDrivesStrike — герой бьёт туда, где курсор, а не туда, куда идёт.
// Одно из двух: смотреть можно в одну сторону, а отступать в другую.
func TestAimDrivesStrike(t *testing.T) {
	armed(t, func(t *testing.T, name string, p *character.Player) {
		// Идём влево, целимся вправо: удар обязан уйти вправо.
		in := character.Input{
			Move: engine.Vec2{X: -1}, Attack: true,
			Aim: start.Add(engine.Vec2{X: 200}), HasAim: true,
		}
		hits := run(p, in, plain, 120)
		if len(hits) != 1 {
			t.Fatalf("%s: ударов %d вместо одного", name, len(hits))
		}
		if hits[0].Face.X <= 0.5 {
			t.Errorf("%s: удар ушёл в сторону движения (face=%v), а не на прицел",
				name, hits[0].Face)
		}
		if p.Dir() != sprite.Right {
			t.Errorf("%s: смотрит %v, хотя прицел справа", name, p.Dir())
		}
	})
}

// TestAttackWhileMovingAnyDirection — с ударом на ходу герой продолжает идти, и
// идти он может куда угодно: хоть от цели, которую бьёт.
func TestAttackWhileMovingAnyDirection(t *testing.T) {
	armed(t, func(t *testing.T, name string, p *character.Player) {
		if !p.Loadout.Attack.OnMove {
			t.Skip("лоадаут не умеет бить на ходу")
		}
		in := character.Input{
			Move: engine.Vec2{X: -1}, Attack: true,
			Aim: start.Add(engine.Vec2{X: 200}), HasAim: true,
		}
		p.Update(in, plain) // старт замаха
		in.Attack = false
		at := p.Pos.X
		for range 12 {
			p.Update(in, plain)
			p.Strike()
		}
		if p.State() != character.Attacking {
			t.Fatalf("%s: замах кончился раньше проверки (%v)", name, p.State())
		}
		if p.Pos.X >= at {
			t.Errorf("%s: в замахе не отступил (был %.1f, стал %.1f)", name, at, p.Pos.X)
		}
	})
}
