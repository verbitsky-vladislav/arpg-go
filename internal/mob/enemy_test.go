package mob_test

import (
	"math"
	"math/rand/v2"
	"os"
	"testing"

	"github.com/vladislav/game/internal/assets"
	"github.com/vladislav/game/internal/engine"
	"github.com/vladislav/game/internal/mob"
	"github.com/vladislav/game/internal/physics"
	"github.com/vladislav/game/internal/sprite"
)

const (
	enemiesFile  = "mobs/enemies/enemies.json"
	behaviorFile = "mobs/enemies/behavior.json"
	enemiesDir   = "mobs/enemies"
)

func behavior(t *testing.T) (*assets.Loader, *mob.EnemyCatalog, *mob.BehaviorSet) {
	t.Helper()
	l := assets.NewLoader(os.DirFS(assetsRoot))
	cat, err := mob.LoadEnemies(l.FS(), enemiesFile)
	if err != nil {
		t.Fatal(err)
	}
	b, err := mob.LoadBehavior(l.FS(), behaviorFile)
	if err != nil {
		t.Fatal(err)
	}
	return l, cat, b
}

// enemyAt собирает особь варианта id в точке pos.
func enemyAt(t *testing.T, id string, pos engine.Vec2) *mob.Enemy {
	t.Helper()
	l, cat, bs := behavior(t)
	tier := cat.Get(id)
	if tier == nil {
		t.Fatalf("нет варианта %s", id)
	}
	pack, err := sprite.Load(l, tier.PackDir(enemiesDir))
	if err != nil {
		t.Fatal(err)
	}
	bhv := bs.For(tier.Type.Temper, tier.Type.Family)
	return mob.NewEnemy(tier, pack, bhv, pos, rand.New(rand.NewPCG(7, 11)))
}

// openField — поле без стен нужного размера (в пикселях).
func openField(px int) *physics.Field {
	const sub = 8
	n := px / sub
	cells := make([]physics.Cell, n*n)
	for i := range cells {
		cells[i] = physics.Ground
	}
	return physics.NewField(n, n, sub, cells)
}

// wallField — поле с вертикальной стеной по x=wallX.
func wallField(px, wallX int) *physics.Field {
	const sub = 8
	n := px / sub
	cells := make([]physics.Cell, n*n)
	for i := range cells {
		cells[i] = physics.Ground
	}
	for y := range n {
		cells[y*n+wallX/sub] = physics.Solid
	}
	return physics.NewField(n, n, sub, cells)
}

func ctx(f *physics.Field, at engine.Vec2, noise float64, sq *mob.Squad) mob.EnemyCtx {
	return mob.EnemyCtx{
		Field:     f,
		Player:    mob.Target{Pos: at, Radius: 7, Alive: true, Noise: noise},
		HasPlayer: true,
		Squad:     sq,
	}
}

func run(e *mob.Enemy, c mob.EnemyCtx, n int) {
	for range n {
		e.Update(c)
	}
}

// TestBehaviorValid — профили поведения внутренне связны и сливаются.
func TestBehaviorValid(t *testing.T) {
	_, _, bs := behavior(t)
	for _, p := range bs.Validate() {
		t.Error(p)
	}

	base := bs.For("aggressive", "")
	undead := bs.For("aggressive", "undead")
	human := bs.For("aggressive", "humanoid")
	ooze := bs.For("aggressive", "ooze")

	if undead.Perception.ReactionTicks <= base.Perception.ReactionTicks {
		t.Error("нежить должна реагировать медленнее обычного агрессора")
	}
	if undead.Perception.MemoryTicks <= base.Perception.MemoryTicks {
		t.Error("нежить должна помнить дольше")
	}
	if undead.Combat.Flinch {
		t.Error("нежить не должна сбиваться от боли")
	}
	if human.Combat.AttackSlots <= base.Combat.AttackSlots {
		t.Error("гуманоиды дерутся строем — очередь удара шире")
	}
	if ooze.Perception.FOVDeg != 360 {
		t.Errorf("у слизня обзор кругом, а не %.0f", ooze.Perception.FOVDeg)
	}
	// Поля, которых семья не касается, должны остаться от профиля.
	if human.Patrol.Leash != base.Patrol.Leash {
		t.Error("поправка семьи затёрла поводок профиля")
	}
}

// TestFOV — со спины враг не видит. Это плата за право игрока подкрасться.
func TestFOV(t *testing.T) {
	f := openField(1024)
	front := enemyAt(t, "skeleton_t1", engine.Vec2{X: 200, Y: 200})
	// Смотрит вниз (стартовое направление): цель снизу видно, сверху нет.
	run(front, ctx(f, engine.Vec2{X: 200, Y: 300}, 0, nil), 30)
	if !front.Engaged() {
		t.Error("цель прямо перед носом не замечена")
	}

	back := enemyAt(t, "skeleton_t1", engine.Vec2{X: 200, Y: 200})
	run(back, ctx(f, engine.Vec2{X: 200, Y: 100}, 0, nil), 30)
	if back.Engaged() {
		t.Error("цель за спиной замечена — угол зрения не работает")
	}
}

// TestWallBlocksSight — сквозь скалу не видно, хотя дистанция позволяет.
func TestWallBlocksSight(t *testing.T) {
	f := wallField(1024, 256)
	e := enemyAt(t, "skeleton_t1", engine.Vec2{X: 200, Y: 300})
	run(e, ctx(f, engine.Vec2{X: 320, Y: 300}, 0, nil), 30)
	if e.Engaged() {
		t.Error("враг видит цель сквозь стену")
	}
}

// TestHearing — шум со спины не делает врага зрячим, но поднимает его idти
// проверить.
func TestHearing(t *testing.T) {
	// Цель за спиной и дальше, чем скелет видит (обзор 200), но ближе, чем
	// слышит (260): иначе он развернётся на шум и честно её увидит, и тест
	// проверял бы не то.
	f := openField(1024)
	e := enemyAt(t, "skeleton_t1", engine.Vec2{X: 500, Y: 500})
	start := e.Pos
	behind := engine.Vec2{X: 500, Y: 270}

	run(e, ctx(f, behind, 300, nil), 40)
	if e.Aware() {
		t.Error("шум сделал врага зрячим — он не должен видеть за спиной")
	}
	if e.State() != mob.ESuspect && engine.Dist(e.Pos, start) == 0 {
		t.Errorf("на шум враг не пошёл: состояние %v", e.State())
	}
}

// TestReactionDelay — заметив цель, враг не срывается в тот же тик.
func TestReactionDelay(t *testing.T) {
	f := openField(1024)
	e := enemyAt(t, "golem_t1", engine.Vec2{X: 200, Y: 200}) // construct: реакция 30
	c := ctx(f, engine.Vec2{X: 200, Y: 260}, 0, nil)
	start := e.Pos

	e.Update(c)
	if engine.Dist(e.Pos, start) > 1 {
		t.Error("враг рванул в тот же тик, когда увидел")
	}
	run(e, c, 120)
	if engine.Dist(e.Pos, start) < 4 {
		t.Errorf("враг так и не пошёл на цель (сдвиг %.1f)", engine.Dist(e.Pos, start))
	}
}

// TestSearchThenReturn — цель исчезла: враг идёт к последней точке, ищет там и
// в конце концов возвращается домой.
func TestSearchThenReturn(t *testing.T) {
	f := openField(2048)
	home := engine.Vec2{X: 400, Y: 400}
	e := enemyAt(t, "gnoll_t1", home)

	seen := engine.Vec2{X: 400, Y: 520}
	run(e, ctx(f, seen, 0, nil), 60)
	if !e.Engaged() {
		t.Fatal("враг не заметил цель")
	}

	// Цель пропала.
	gone := mob.EnemyCtx{Field: f}
	run(e, gone, 60)
	if d := engine.Dist(e.LastSeen(), seen); d > 40 {
		t.Errorf("последняя известная точка уехала на %.0f px", d)
	}
	run(e, gone, 1200)
	if e.Engaged() {
		t.Error("враг помнит цель дольше памяти")
	}
	if d := engine.Dist(e.Pos, home); d > 80 {
		t.Errorf("враг не вернулся домой: %.0f px от дома", d)
	}
}

// TestLeash — за поводком враг разворачивается, даже если цель видна.
func TestLeash(t *testing.T) {
	f := openField(4096)
	home := engine.Vec2{X: 400, Y: 400}
	e := enemyAt(t, "ent_t1", home) // territorial: поводок 320 (plant ужимает до 200)
	leash := e.Bhv.Patrol.Leash

	far := home.Add(engine.Vec2{X: leash + 300})
	run(e, ctx(f, far, 0, nil), 900)
	if d := engine.Dist(e.Pos, home); d > leash+40 {
		t.Errorf("враг ушёл за поводок: %.0f px при поводке %.0f", d, leash)
	}
}

// TestRangedKeepsDistance — стрелок не лезет в ближний бой, а держит полосу.
func TestRangedKeepsDistance(t *testing.T) {
	f := openField(2048)
	e := enemyAt(t, "lich_t1", engine.Vec2{X: 600, Y: 600})
	want := e.Bhv.Combat.PreferRange
	if want <= 0 {
		t.Fatal("у лича не задана предпочтительная дистанция")
	}

	// Цель вплотную и прямо перед носом (враг смотрит вниз) — стрелок обязан
	// отойти, а не махать руками.
	c := ctx(f, engine.Vec2{X: 600, Y: 630}, 0, nil)
	run(e, c, 300)
	got := engine.Dist(e.Pos, c.Player.Pos)
	if got < want*0.6 {
		t.Errorf("стрелок подошёл на %.0f при желаемой дистанции %.0f", got, want)
	}
}

// TestAttackSlots — толпа бьёт по очереди, а не всем скопом.
func TestAttackSlots(t *testing.T) {
	f := openField(2048)
	sq := mob.NewSquad()
	target := engine.Vec2{X: 800, Y: 800}
	var group []*mob.Enemy
	for i := range 6 {
		ang := float64(i) / 6 * 2 * math.Pi
		pos := target.Add(engine.Vec2{X: math.Cos(ang) * 30, Y: math.Sin(ang) * 30})
		e := enemyAt(t, "skeleton_t1", pos)
		sq.Add(e)
		group = append(group, e)
	}
	slots := group[0].Bhv.Combat.AttackSlots

	c := ctx(f, target, 0, sq)
	worst := 0
	for range 600 {
		sq.Prune()
		for _, e := range group {
			e.Update(c)
			e.Strike()
		}
		attacking := 0
		for _, e := range group {
			if e.State() == mob.EAttack {
				attacking++
			}
		}
		worst = max(worst, attacking)
	}
	if worst == 0 {
		t.Fatal("никто ни разу не ударил")
	}
	if worst > slots {
		t.Errorf("одновременно бьют %d при очереди на %d", worst, slots)
	}
}

// TestSeparation — свои не слипаются в одну точку.
func TestSeparation(t *testing.T) {
	f := openField(2048)
	sq := mob.NewSquad()
	target := engine.Vec2{X: 900, Y: 900}
	var group []*mob.Enemy
	for i := range 4 {
		e := enemyAt(t, "gnoll_t1", target.Add(engine.Vec2{X: float64(i)*3 - 200, Y: -180}))
		sq.Add(e)
		group = append(group, e)
	}

	c := ctx(f, target, 0, sq)
	for range 400 {
		sq.Prune()
		for _, e := range group {
			e.Update(c)
			e.Strike()
		}
	}
	for i, a := range group {
		for j, b := range group {
			if j <= i {
				continue
			}
			if d := engine.Dist(a.Pos, b.Pos); d < 4 {
				t.Errorf("особи %d и %d слиплись: %.1f px", i, j, d)
			}
		}
	}
}

// TestShoutWakesGroup — увидел один, узнали все, кто рядом.
func TestShoutWakesGroup(t *testing.T) {
	f := openField(2048)
	sq := mob.NewSquad()
	target := engine.Vec2{X: 700, Y: 800}

	scout := enemyAt(t, "orc_t1", engine.Vec2{X: 700, Y: 720}) // смотрит вниз, видит цель
	// Второй стоит так, что сам цель не увидит никогда: она дальше его обзора
	// (210), зато крик разведчика до него достаёт.
	deaf := enemyAt(t, "orc_t1", engine.Vec2{X: 560, Y: 560})
	sq.Add(scout)
	sq.Add(deaf)

	// Двадцати тиков хватает на «увидел, крикнул, услышали»: дальше второй
	// орк успеет подойти на дистанцию обзора и увидит цель сам, а проверяем мы
	// не это.
	c := ctx(f, target, 0, sq)
	for range 20 {
		sq.Prune()
		scout.Update(c)
		deaf.Update(c)
	}
	if !scout.Engaged() {
		t.Fatal("разведчик не увидел цель")
	}
	if !deaf.Engaged() {
		t.Error("свои не услышали крик — перекличка не работает")
	}
	if deaf.Aware() {
		t.Error("по крику враг узнал точку, но видеть цель он не должен")
	}
	if d := engine.Dist(deaf.LastSeen(), target); d > 1 {
		t.Errorf("крик передал точку с ошибкой %.0f px", d)
	}
}

// TestHurtWakes — удар в спину поднимает даже того, кто ничего не видел.
func TestHurtWakes(t *testing.T) {
	f := openField(1024)
	e := enemyAt(t, "zombie_t1", engine.Vec2{X: 300, Y: 300})
	from := engine.Vec2{X: 300, Y: 240}
	if e.Engaged() {
		t.Fatal("враг знает про цель до всякого контакта")
	}
	e.Hurt(3, from)
	if !e.Engaged() {
		t.Error("удар в спину не разбудил врага")
	}
	run(e, mob.EnemyCtx{Field: f}, 120)
	if d := engine.Dist(e.Pos, from); d > 70 {
		t.Errorf("враг не пошёл к источнику удара: %.0f px", d)
	}
}

// TestEnemyStrikeOnce — за замах ровно один удар, и он накрывает цель перед
// врагом, но не за спиной.
func TestEnemyStrikeOnce(t *testing.T) {
	f := openField(1024)
	target := engine.Vec2{X: 300, Y: 340}
	e := enemyAt(t, "orc_t1", engine.Vec2{X: 300, Y: 300})
	c := ctx(f, target, 0, nil)

	hits := 0
	var last mob.Hit
	for range 240 {
		e.Update(c)
		if h, ok := e.Strike(); ok {
			hits++
			last = h
		}
	}
	if hits == 0 {
		t.Fatal("враг ни разу не ударил, стоя вплотную к цели")
	}
	if !last.Covers(target, 7) {
		t.Error("удар не накрывает цель, к которой враг повёрнут")
	}
	behind := e.Pos.Sub(last.Face.Scale(last.Reach - 1))
	if last.Covers(behind, 0) {
		t.Error("удар достаёт за спину")
	}
}

// TestEnemiesAlwaysDrawable — любой вариант в любом состоянии есть чем
// нарисовать: наборы анимаций у паков рваные, а деградация обязана закрывать.
func TestEnemiesAlwaysDrawable(t *testing.T) {
	l, cat, bs := behavior(t)
	f := openField(1024)
	rng := rand.New(rand.NewPCG(3, 5))

	for _, id := range cat.IDs() {
		tier := cat.Get(id)
		pack, err := sprite.Load(l, tier.PackDir(enemiesDir))
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		e := mob.NewEnemy(tier, pack, bs.For(tier.Type.Temper, tier.Type.Family),
			engine.Vec2{X: 400, Y: 400}, rng)
		c := ctx(f, engine.Vec2{X: 430, Y: 430}, 120, nil)
		for i := range 400 {
			e.Update(c)
			e.Strike()
			if e.Alive() && e.Frame() == nil {
				t.Fatalf("%s: тик %d, состояние %v, клип %q — нечего рисовать",
					id, i, e.State(), e.Clip())
			}
		}
		e.Hurt(tier.HP*10, c.Player.Pos)
		for range 400 {
			e.Update(c)
			if e.Gone() {
				break
			}
		}
		if !e.Gone() {
			t.Errorf("%s: труп не убрался с карты", id)
		}
	}
}

// TestEnemySpawnConfig — конфиг заселения читается и внутренне связен, а
// выбор тира слушается биома: в подземелье третьего круга старшие тиры должны
// встречаться заметно чаще, чем на первом.
func TestEnemySpawnConfig(t *testing.T) {
	l := assets.NewLoader(os.DirFS(assetsRoot))
	cfg, err := mob.LoadEnemySpawn(l.FS(), "mobs/enemies/spawn.json")
	if err != nil {
		t.Fatal(err)
	}
	// Кольцо спавна должно быть за пределами обзора: экран 640x360.
	if halfDiag := math.Hypot(640, 360) / 2; cfg.Radius.SpawnMin <= halfDiag {
		t.Errorf("spawn_min=%.0f не больше полудиагонали обзора %.0f — игрок увидит появление",
			cfg.Radius.SpawnMin, halfDiag)
	}
	if cfg.Budget(true) <= cfg.Budget(false) {
		t.Error("ночью мир должен становиться злее")
	}
	if cfg.TypeCap("demon") >= cfg.TypeCap("skeleton") {
		t.Error("демонов должно быть меньше, чем скелетов")
	}

	_, cat, _ := behavior(t)
	demon := cat.Types["demon"]
	rng := rand.New(rand.NewPCG(1, 2))
	count := func(biome string) int {
		high := 0
		for range 2000 {
			if tier := cfg.RollTier(rng, demon, biome); tier != nil && tier.ID == "t3" {
				high++
			}
		}
		return high
	}
	shallow, deep := count("dungeon_1"), count("dungeon_3")
	if deep <= shallow*2 {
		t.Errorf("глубина не сдвигает силу: t3 в dungeon_1 %d, в dungeon_3 %d", shallow, deep)
	}
}
