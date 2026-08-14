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

// mazeField — поле со сплошной стеной и единственным проходом внизу: путь в
// обход есть, но по прямой не пройти и не докричаться.
func mazeField(px, wallX, gapY int) *physics.Field {
	const sub = 8
	n := px / sub
	cells := make([]physics.Cell, n*n)
	for i := range cells {
		cells[i] = physics.Ground
	}
	col := wallX / sub
	gap := gapY / sub
	for y := range n {
		// Проход в 48 px: у́же тела крупного врага он бы не пропустил, и тест
		// проверял бы физику, а не дорогу.
		if y >= gap && y < gap+6 {
			continue
		}
		cells[y*n+col] = physics.Solid
	}
	return physics.NewField(n, n, sub, cells)
}

// TestNavGoesAroundWall — враг обходит стену через проход, а не упирается в неё
// лбом. Без общей карты навигации он бы застрял: локальный объезд угла видит
// только на полтора корпуса вперёд.
func TestNavGoesAroundWall(t *testing.T) {
	// Проход недалеко: обход должен укладываться в память врага (420 тиков),
	// иначе он честно забудет цель по дороге — и тест будет проверять память,
	// а не дорогу.
	const wallX, gapY = 512, 420
	f := mazeField(1024, wallX, gapY)
	target := engine.Vec2{X: 620, Y: 300}

	// Цель за стеной: увидеть её нельзя, поэтому враг знает о ней от удара в
	// спину — проверяем дорогу, а не восприятие.
	withNav := enemyAt(t, "gnoll_t1", engine.Vec2{X: 400, Y: 300})
	withNav.Hurt(1, target)
	nav := mob.NewNav(f, 32)
	nav.Rebuild(target, 0)

	c := mob.EnemyCtx{
		Field:     f,
		Player:    mob.Target{Pos: target, Radius: 7, Alive: true, Noise: 400},
		HasPlayer: true,
		Nav:       nav,
	}
	for i := range 1200 {
		withNav.Update(c)
		withNav.Strike()
		if i%20 == 0 {
			nav.Rebuild(target, 0)
		}
		if withNav.Pos.X > wallX+20 {
			break
		}
	}
	if withNav.Pos.X <= wallX {
		t.Errorf("с картой навигации враг не обошёл стену: застрял на x=%.0f", withNav.Pos.X)
	}

	// Тот же случай без карты: враг имеет право застрять — именно этот разрыв
	// и закрывает навигация.
	blind := enemyAt(t, "gnoll_t1", engine.Vec2{X: 400, Y: 300})
	blind.Hurt(1, target)
	cb := c
	cb.Nav = nil
	for range 1200 {
		blind.Update(cb)
		blind.Strike()
	}
	t.Logf("без навигации враг дошёл до x=%.0f (стена на %d)", blind.Pos.X, wallX)
}

// TestHearingMuffledByWall — за стеной шум глохнет: слышно по коридорам, а не
// сквозь скалу. По прямой до цели 120 px при слухе 260 — в чистом поле услышал
// бы наверняка.
func TestHearingMuffledByWall(t *testing.T) {
	const wallX = 512
	f := mazeField(2048, wallX, 1800) // проход далеко внизу — обход длинный
	target := engine.Vec2{X: 580, Y: 300}
	nav := mob.NewNav(f, 32)
	nav.Rebuild(target, 0)

	e := enemyAt(t, "skeleton_t1", engine.Vec2{X: 460, Y: 300}) // 120 px по прямой
	c := mob.EnemyCtx{
		Field:     f,
		Player:    mob.Target{Pos: target, Radius: 7, Alive: true, Noise: 400},
		HasPlayer: true,
		Nav:       nav,
	}
	run(e, c, 60)
	if e.Engaged() {
		t.Error("шум прошёл сквозь стену — слышимость считается по прямой")
	}

	// Та же геометрия без стены: тут услышать обязан.
	open := openField(2048)
	e2 := enemyAt(t, "skeleton_t1", engine.Vec2{X: 460, Y: 300})
	nav2 := mob.NewNav(open, 32)
	nav2.Rebuild(target, 0)
	c2 := mob.EnemyCtx{
		Field:     open,
		Player:    mob.Target{Pos: target, Radius: 7, Alive: true, Noise: 400},
		HasPlayer: true,
		Nav:       nav2,
	}
	run(e2, c2, 60)
	if !e2.Engaged() {
		t.Error("в чистом поле шум в 120 px не услышан")
	}
}

// TestDodgeUnderWindup — увидев занесённое оружие, враг уходит вбок, а не стоит
// под ударом. Голем не умеет — и не должен.
func TestDodgeUnderWindup(t *testing.T) {
	f := openField(1024)
	target := engine.Vec2{X: 300, Y: 340}

	windup := func(at engine.Vec2) mob.Target {
		return mob.Target{
			Pos: at, Radius: 7, Alive: true,
			Windup: true, WindupFace: engine.Vec2{X: 0, Y: -1}, WindupReach: 40, WindupArc: 120,
		}
	}

	e := enemyAt(t, "gnoll_t1", engine.Vec2{X: 300, Y: 300}) // beast: dodge 0.55
	c := mob.EnemyCtx{Field: f, Player: windup(target), HasPlayer: true}
	dodged := false
	side := 0.0
	for range 200 {
		e.Update(c)
		e.Strike()
		if e.State() == mob.EDodge {
			dodged = true
			side = max(side, absF(e.Pos.X-300))
		}
	}
	if !dodged {
		t.Error("зверь ни разу не ушёл из-под замаха")
	}
	if side < 2 {
		t.Errorf("отскок не сдвинул врага вбок (%.1f px)", side)
	}

	golem := enemyAt(t, "golem_t1", engine.Vec2{X: 300, Y: 300}) // construct: dodge 0
	cg := mob.EnemyCtx{Field: f, Player: windup(target), HasPlayer: true}
	for range 200 {
		golem.Update(cg)
		golem.Strike()
		if golem.State() == mob.EDodge {
			t.Fatal("голем уклоняется, хотя не умеет")
		}
	}
}

func absF(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// TestSquadRoles — в большой группе появляются роли: ударный строй, фланги и
// один отрезающий отход. Роль держится, а не перевыбирается каждый тик.
func TestSquadRoles(t *testing.T) {
	f := openField(2048)
	sq := mob.NewSquad()
	target := engine.Vec2{X: 900, Y: 1000}
	var group []*mob.Enemy
	for i := range 6 {
		e := enemyAt(t, "orc_t1", engine.Vec2{X: 860 + float64(i)*16, Y: 900})
		sq.Add(e)
		group = append(group, e)
	}

	c := ctx(f, target, 0, sq)
	for range 200 {
		sq.Prune()
		for _, e := range group {
			e.Update(c)
			e.Strike()
		}
	}

	roles := map[mob.Role]int{}
	for _, e := range group {
		roles[sq.RoleOf(e)]++
	}
	if roles[mob.RoleFront] == 0 {
		t.Error("никто не идёт в лоб")
	}
	if roles[mob.RoleFront] > group[0].Bhv.Combat.AttackSlots {
		t.Errorf("в лоб идут %d при очереди на %d", roles[mob.RoleFront], group[0].Bhv.Combat.AttackSlots)
	}
	if roles[mob.RoleCutoff] != 1 {
		t.Errorf("отрезающих %d, а должен быть ровно один", roles[mob.RoleCutoff])
	}

	before := sq.RoleOf(group[0])
	for range 100 {
		for _, e := range group {
			e.Update(c)
			e.Strike()
		}
	}
	if sq.RoleOf(group[0]) != before {
		t.Error("роль сменилась на ходу — группа будет выглядеть паникующей")
	}
}

// --- спавнер -------------------------------------------------------------

func enemySpawner(t *testing.T, biome string) (*mob.EnemySpawner, *mob.EnemySpawnConfig, *mob.EnemyCatalog) {
	t.Helper()
	l := assets.NewLoader(os.DirFS(assetsRoot))
	cat, err := mob.LoadEnemies(l.FS(), enemiesFile)
	if err != nil {
		t.Fatal(err)
	}
	bhv, err := mob.LoadBehavior(l.FS(), behaviorFile)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := mob.LoadEnemySpawn(l.FS(), "mobs/enemies/spawn.json")
	if err != nil {
		t.Fatal(err)
	}
	packs := func(art string) (*sprite.Pack, error) { return sprite.Load(l, enemiesDir+"/"+art) }
	s := mob.NewEnemySpawner(cfg, cat, bhv, newForestWorld(biome), packs, rand.New(rand.NewPCG(9, 4)))
	return s, cfg, cat
}

// TestEnemySpawnerPopulate — карта заселяется в пределах бюджета опасности и
// лимитов по типам, и только теми, кто в этом биоме водится.
func TestEnemySpawnerPopulate(t *testing.T) {
	s, cfg, cat := enemySpawner(t, "forest")
	s.Populate(false)

	if s.Danger() <= 0 {
		t.Fatal("карта осталась пустой")
	}
	if s.Danger() > cfg.Budget(false) {
		t.Errorf("бюджет опасности превышен: %.0f при %.0f", s.Danger(), cfg.Budget(false))
	}
	if len(s.Enemies()) > cfg.Limits.Global {
		t.Errorf("особей %d при лимите %d", len(s.Enemies()), cfg.Limits.Global)
	}

	perType := map[string]int{}
	for _, e := range s.Enemies() {
		perType[e.Type.ID]++
		if cat.Types[e.Type.ID].Habitat.Biomes["forest"] <= 0 {
			t.Errorf("в лесу завёлся %s, которому здесь не место", e.Type.ID)
		}
	}
	for id, n := range perType {
		if n > cfg.TypeCap(id) {
			t.Errorf("%s: %d особей при пределе %d", id, n, cfg.TypeCap(id))
		}
	}
	if len(perType) < 2 {
		t.Errorf("на карте только %d тип(а) — лимиты не держат разнообразие", len(perType))
	}
}

// TestEnemySpawnRing — подсев не ставит никого в поле зрения игрока.
func TestEnemySpawnRing(t *testing.T) {
	s, cfg, _ := enemySpawner(t, "forest")
	player := engine.Vec2{X: worldSide / 2, Y: worldSide / 2}
	seen := map[*mob.Enemy]bool{}

	for range 4000 {
		s.Update(mob.Target{Pos: player, Radius: 7, Alive: true}, true, 0, false)
		for _, e := range s.Enemies() {
			if seen[e] {
				continue
			}
			seen[e] = true
			if d := engine.Dist(e.Pos, player); d < cfg.Radius.SpawnMin-cfg.Rate.GroupSpread {
				t.Fatalf("враг появился в %.0f px от игрока при кольце от %.0f", d, cfg.Radius.SpawnMin)
			}
		}
	}
	if len(seen) == 0 {
		t.Error("за 4000 тиков не появилось ни одного врага")
	}
}

// TestEnemyDespawnFar — оставшиеся далеко снимаются, но тот, кто уже дерётся,
// не исчезает: пропасть на глазах у игрока — худшее, что может сделать спавнер.
func TestEnemyDespawnFar(t *testing.T) {
	s, cfg, _ := enemySpawner(t, "forest")
	s.Populate(false)
	if len(s.Enemies()) == 0 {
		t.Fatal("карта пуста")
	}

	// Один из них уже знает про игрока (получил удар) и стоит далеко.
	corner := engine.Vec2{X: 60, Y: 60}
	var engaged *mob.Enemy
	for _, e := range s.Enemies() {
		if engine.Dist(e.Pos, corner) > cfg.Radius.Despawn {
			e.Hurt(1, e.Pos.Add(engine.Vec2{X: 10}))
			engaged = e
			break
		}
	}

	// Пока враг в бою, спавнер не имеет права его снять. Проверяем это на
	// каждом тике: потерять интерес и быть снятым потом — законно, исчезнуть
	// посреди боя — нет.
	for range 3000 {
		wasEngaged := engaged != nil && engaged.Engaged()
		s.Update(mob.Target{Pos: corner, Radius: 7, Alive: true}, true, 0, false)
		if wasEngaged && engaged.Gone() && engaged.Alive() {
			t.Fatal("враг исчез, находясь в бою")
		}
	}
	for _, e := range s.Enemies() {
		if e == engaged {
			continue
		}
		if d := engine.Dist(e.Pos, corner); d > cfg.Radius.Despawn+cfg.Radius.SpawnMax {
			t.Errorf("враг остался в %.0f px от игрока при снятии за %.0f", d, cfg.Radius.Despawn)
		}
	}

}

// TestEnemySpawnerLive — долгий прогон с игроком посреди карты: никто не
// остаётся без кадра, бюджет держится, паки грузятся.
func TestEnemySpawnerLive(t *testing.T) {
	s, cfg, _ := enemySpawner(t, "forest")
	s.Populate(true)
	player := engine.Vec2{X: 1000, Y: 1000}

	for i := range 3000 {
		night := i%1200 < 600
		s.Update(mob.Target{Pos: player, Radius: 7, Alive: true, Noise: 120}, true, 0, night)
		for _, e := range s.Enemies() {
			e.Strike()
			if e.Alive() && e.Frame() == nil {
				t.Fatalf("тик %d: %s в состоянии %v без кадра", i, e.Type.ID, e.State())
			}
		}
		if s.Danger() > cfg.Budget(true)+1 {
			t.Fatalf("тик %d: опасность %.0f вышла за ночной бюджет %.0f", i, s.Danger(), cfg.Budget(true))
		}
	}
	if errs := s.Errors(); len(errs) > 0 {
		t.Errorf("ошибки загрузки паков: %v", errs)
	}
	if len(s.Enemies()) == 0 {
		t.Error("к концу прогона карта опустела")
	}
}

// TestSafeSpawnZone — вокруг точки старта врагов не заводят: ни при заселении
// карты, ни подсевом. Игрок должен успеть осмотреться, а не отбиваться с
// первого кадра.
func TestSafeSpawnZone(t *testing.T) {
	s, cfg, _ := enemySpawner(t, "forest")
	start := engine.Vec2{X: worldSide / 2, Y: worldSide / 2}
	s.Guard(start)
	s.Populate(false)

	if cfg.Safe.Radius <= 0 {
		t.Fatal("радиус тишины не задан")
	}
	for _, e := range s.Enemies() {
		if d := engine.Dist(e.Pos, start); d < cfg.Safe.Radius {
			t.Fatalf("при заселении враг встал в %.0f px от старта при тишине %.0f", d, cfg.Safe.Radius)
		}
	}

	// Игрок стоит на старте: подсев тоже обязан обходить это место стороной.
	seen := map[*mob.Enemy]bool{}
	for range 4000 {
		s.Update(mob.Target{Pos: start, Radius: 7, Alive: true}, true, 0, false)
		for _, e := range s.Enemies() {
			if seen[e] {
				continue
			}
			seen[e] = true
			if d := engine.Dist(e.Pos, start); d < cfg.Safe.Radius {
				t.Fatalf("подсев поставил врага в %.0f px от старта", d)
			}
		}
	}

	// Но прийти сами они обязаны: тишина — это про появление, а не про запрет
	// заходить. Ставим врага у края зоны и смотрим, что он идёт на игрока.
	e := enemyAt(t, "gnoll_t1", start.Add(engine.Vec2{X: cfg.Safe.Radius + 20}))
	e.Hurt(1, start)
	c := ctx(openField(worldSide), start, 0, nil)
	run(e, c, 600)
	if d := engine.Dist(e.Pos, start); d >= cfg.Safe.Radius {
		t.Errorf("враг не зашёл в тихую зону за игроком: %.0f px", d)
	}
}

// TestNavLanesBySize — крупного врага не ведут в проход, куда он не пролезет.
// Сетка знает про размеры тел: мелкому проход открыт, крупному — нет, и путь
// для него идёт в обход.
func TestNavLanesBySize(t *testing.T) {
	// Стена с двумя проходами: узкий (32 px) близко и широкий (96 px) далеко.
	const sub, side = 8, 128 // 1024 px
	cells := make([]physics.Cell, side*side)
	for i := range cells {
		cells[i] = physics.Ground
	}
	col := 512 / sub
	for y := range side {
		py := y * sub
		narrow := py >= 300 && py < 332
		wide := py >= 700 && py < 796
		if narrow || wide {
			continue
		}
		cells[y*side+col] = physics.Solid
	}
	f := physics.NewField(side, side, sub, cells)

	nav := mob.NewNav(f, 16)
	target := engine.Vec2{X: 620, Y: 300}
	nav.Rebuild(target, 0)
	from := engine.Vec2{X: 400, Y: 300} // напротив узкого прохода

	small, okS := nav.Dist(from, 0, 10)
	big, okB := nav.Dist(from, 0, 32)
	if !okS || !okB {
		t.Fatalf("путь не найден вовсе: мелкий %v, крупный %v", okS, okB)
	}
	// Мелкий проскакивает рядом, крупный идёт кругом через дальний проход.
	if small >= big {
		t.Errorf("крупному путь не длиннее (%.0f против %.0f) — сетка не различает размеры", big, small)
	}
	if big < 800 {
		t.Errorf("крупный прошёл за %.0f px — похоже, его пустили в узкую щель", big)
	}

	// Шаг тоже обязан различаться: мелкий идёт к проходу, крупный — вдоль стены.
	sStep, _ := nav.Step(from, 0, 10)
	bStep, _ := nav.Step(from, 0, 32)
	if sStep.Y > 0.5 {
		t.Errorf("мелкий пошёл не в свой проход: %v", sStep)
	}
	if bStep.Y <= 0 {
		t.Errorf("крупный не пошёл к дальнему проходу: %v", bStep)
	}
}

// TestSearchPathAroundWall — к последней известной точке враг идёт дорогой, а
// не упирается в стену. Общей волны тут нет: игрока на карте нет вовсе, и путь
// каждый считает себе сам.
func TestSearchPathAroundWall(t *testing.T) {
	const wallX, gapY = 512, 420
	f := mazeField(1024, wallX, gapY)
	seen := engine.Vec2{X: 620, Y: 300} // где «видели» игрока — за стеной

	e := enemyAt(t, "gnoll_t1", engine.Vec2{X: 400, Y: 300})
	e.Hurt(1, seen) // теперь он знает точку и пойдёт её проверять

	// Игрока на карте нет: ни волны, ни цели — только память и карта.
	c := mob.EnemyCtx{Field: f, Nav: mob.NewNav(f, 16)}
	for range 1200 {
		e.Update(c)
		e.Strike()
		if e.Pos.X > wallX+20 {
			break
		}
	}
	if e.Pos.X <= wallX {
		t.Errorf("враг не нашёл дорогу к запомненной точке: застрял на x=%.0f", e.Pos.X)
	}
}

// TestChokeFindsDoorway — узкое место ищется там, где оно есть, и не выдумывается
// в чистом поле.
func TestChokeFindsDoorway(t *testing.T) {
	const sub, side = 8, 128
	cells := make([]physics.Cell, side*side)
	for i := range cells {
		cells[i] = physics.Ground
	}
	col := 700 / sub
	for y := range side {
		if py := y * sub; py >= 480 && py < 544 { // дверь
			continue
		}
		cells[y*side+col] = physics.Solid
	}
	f := physics.NewField(side, side, sub, cells)
	nav := mob.NewNav(f, 16)

	// Цель у двери, группа слева: отход — направо, через дверь.
	target := engine.Vec2{X: 620, Y: 512}
	p, ok := nav.Choke(target, engine.Vec2{X: 1}, 0, 16, 220)
	if !ok {
		t.Fatal("дверь не найдена — отрезать отход будет негде")
	}
	if p.X < 660 || p.X > 760 {
		t.Errorf("узкое место найдено не у двери: %v", p)
	}
	if p.Y < 470 || p.Y > 555 {
		t.Errorf("узкое место найдено не в проёме по высоте: %v", p)
	}

	// В чистом поле перекрывать нечего.
	open := openField(1024)
	if _, ok := mob.NewNav(open, 16).Choke(engine.Vec2{X: 500, Y: 500}, engine.Vec2{X: 1}, 0, 16, 220); ok {
		t.Error("в чистом поле нашлось «узкое место» — отрезающий встанет в никуда")
	}
}

// pocketField — тупик: карман, открытый только сзади. Идти к цели впереди
// некуда, и упереться в стену можно бесконечно.
func pocketField(px int) *physics.Field {
	const sub = 8
	n := px / sub
	cells := make([]physics.Cell, n*n)
	for i := range cells {
		cells[i] = physics.Ground
	}
	for y := range n {
		for x := range n {
			py, pxx := y*sub, x*sub
			if pxx >= 520 && pxx < 560 && py >= 200 && py < 500 {
				cells[y*n+x] = physics.Solid
			}
		}
	}
	return physics.NewField(n, n, sub, cells)
}

// TestStuckWatchdog — упёршийся враг не толкается в стену вечно: сбрасывает
// дорогу, пробует обойти, а в конце концов бросает цель и уходит домой.
//
// Карты навигации нарочно нет: проверяем именно сторож, а не отказ по
// недостижимости.
func TestStuckWatchdog(t *testing.T) {
	f := pocketField(1024)
	behind := engine.Vec2{X: 700, Y: 350} // за стеной, дороги нет
	e := enemyAt(t, "skeleton_t1", engine.Vec2{X: 480, Y: 350})
	e.Hurt(1, behind)

	c := mob.EnemyCtx{Field: f} // ни игрока, ни навигации
	gave := false
	for range 900 {
		e.Update(c)
		if !e.Engaged() {
			gave = true
			break
		}
	}
	if !gave {
		t.Error("враг так и не бросил недостижимую цель — толкается в стену бесконечно")
	}
	if e.Pos.X > 520 {
		t.Errorf("враг оказался в стене: x=%.0f", e.Pos.X)
	}
}

// TestUnreachableGivesUpFast — когда карта говорит «дороги нет для такого
// тела», враг разворачивается сразу, а не ждёт сторожа.
func TestUnreachableGivesUpFast(t *testing.T) {
	const wallX = 512
	f := mazeField(2048, wallX, 1900) // проход есть, но на другом конце карты
	target := engine.Vec2{X: 600, Y: 300}
	nav := mob.NewNav(f, 16)
	nav.Rebuild(target, 0)

	// Крупному врагу проход у края слишком узок: ставим стену без прохода.
	solid := mazeField(2048, wallX, 4000)
	navSolid := mob.NewNav(solid, 16)
	navSolid.Rebuild(target, 0)

	e := enemyAt(t, "ent_t1", engine.Vec2{X: 400, Y: 300})
	e.Hurt(1, target)
	c := mob.EnemyCtx{
		Field:     solid,
		Player:    mob.Target{Pos: target, Radius: 7, Alive: true},
		HasPlayer: true,
		Nav:       navSolid,
	}
	for range 120 {
		e.Update(c)
	}
	if e.State() != mob.EReturn && e.Engaged() {
		t.Errorf("враг не отказался от недостижимой цели: состояние %v", e.State())
	}
	_ = nav
}

// TestLeadAim — по бегущей цели бьют с упреждением: удар уходит туда, куда она
// бежит, а не туда, где она была в начале замаха.
func TestLeadAim(t *testing.T) {
	f := openField(1024)
	e := enemyAt(t, "skeleton_t1", engine.Vec2{X: 500, Y: 500})

	// Цель идёт вправо прямо перед носом (враг смотрит вниз).
	pos := engine.Vec2{X: 500, Y: 530}
	var hit mob.Hit
	got := false
	for range 300 {
		pos.X += 1.5 // ~90 px/с
		c := ctx(f, pos, 0, nil)
		e.Update(c)
		if h, ok := e.Strike(); ok {
			hit, got = h, true
			break
		}
	}
	if !got {
		t.Fatal("враг не ударил")
	}
	if hit.Face.X <= 0 {
		t.Errorf("удар без упреждения: цель бежит вправо, а бьют в %v", hit.Face)
	}
}

// TestDashAfterRetreat — от зверя нельзя просто уходить спиной: он рвётся
// вдогонку и на короткое время бежит быстрее обычного.
func TestDashAfterRetreat(t *testing.T) {
	f := openField(4096)
	e := enemyAt(t, "gnoll_t1", engine.Vec2{X: 500, Y: 500})
	runSpeed := e.Tier.Speed.Run / 60

	pos := engine.Vec2{X: 500, Y: 560} // перед носом
	prev := e.Pos
	fastest := 0.0
	for range 600 {
		pos.Y += 1.2 // цель отступает
		e.Update(ctx(f, pos, 0, nil))
		e.Strike()
		if d := engine.Dist(e.Pos, prev); d > fastest {
			fastest = d
		}
		prev = e.Pos
	}
	if fastest <= runSpeed*1.2 {
		t.Errorf("зверь не рванул за целью: быстрейший шаг %.2f при беге %.2f", fastest, runSpeed)
	}
}

// TestFinishDropsCaution — раненую цель добивают: очередь удара шире на одного
// и никто не кружит в стороне.
func TestFinishDropsCaution(t *testing.T) {
	f := openField(2048)
	target := engine.Vec2{X: 900, Y: 900}

	count := func(hp float64) int {
		sq := mob.NewSquad()
		var group []*mob.Enemy
		for i := range 5 {
			ang := float64(i) / 5 * 2 * math.Pi
			e := enemyAt(t, "skeleton_t1",
				target.Add(engine.Vec2{X: math.Cos(ang) * 26, Y: math.Sin(ang) * 26}))
			sq.Add(e)
			group = append(group, e)
		}
		c := ctx(f, target, 0, sq)
		c.Player.HPFrac = hp
		worst := 0
		for range 600 {
			sq.Prune()
			at := 0
			for _, e := range group {
				e.Update(c)
				e.Strike()
				if e.State() == mob.EAttack {
					at++
				}
			}
			worst = max(worst, at)
		}
		return worst
	}

	calm, wounded := count(1.0), count(0.15)
	if wounded <= calm {
		t.Errorf("раненую цель бьют не активнее целой: %d против %d", wounded, calm)
	}
}

// TestAmbusherWaits — засадник стоит неподвижно, пока не заметит, и срывается,
// когда цель входит в поле зрения. Бродящая «засада» — не засада.
func TestAmbusherWaits(t *testing.T) {
	f := openField(2048)
	e := enemyAt(t, "vampire_t1", engine.Vec2{X: 900, Y: 900}) // ambusher
	if !e.Bhv.Combat.Ambush {
		t.Fatal("вампир перестал быть засадником — тест не о том")
	}
	home := e.Pos

	// Цели нет: он обязан стоять.
	run(e, mob.EnemyCtx{Field: f}, 600)
	if d := engine.Dist(e.Pos, home); d > 2 {
		t.Errorf("засадник бродит: ушёл на %.1f px", d)
	}

	// Цель перед носом — срывается.
	run(e, ctx(f, home.Add(engine.Vec2{Y: 60}), 0, nil), 120)
	if engine.Dist(e.Pos, home) < 4 {
		t.Error("засадник не сорвался, увидев цель")
	}
}

// TestSearchFansOut — потеряв цель, группа расходится веером, а не топчется в
// одной точке.
func TestSearchFansOut(t *testing.T) {
	f := openField(2048)
	sq := mob.NewSquad()
	seen := engine.Vec2{X: 1000, Y: 1000}
	var group []*mob.Enemy
	for i := range 3 {
		e := enemyAt(t, "gnoll_t1", seen.Add(engine.Vec2{X: float64(i)*10 - 120}))
		e.Hurt(1, seen)
		sq.Add(e)
		group = append(group, e)
	}

	c := mob.EnemyCtx{Field: f, Squad: sq} // цели на карте нет — только память
	for range 400 {
		sq.Prune()
		for _, e := range group {
			e.Update(c)
		}
	}
	for i, a := range group {
		for j, b := range group {
			if j <= i {
				continue
			}
			if d := engine.Dist(a.Pos, b.Pos); d < 30 {
				t.Errorf("ищут в одной точке: %d и %d в %.0f px друг от друга", i, j, d)
			}
		}
	}
}

// TestGapQueue — узкое место занимает один: второй ждёт, а не выпихивает
// первого. Проверяется на самом договоре, поведение поверх него уже дело
// управления.
func TestGapQueue(t *testing.T) {
	sq := mob.NewSquad()
	a := enemyAt(t, "goblin_t1", engine.Vec2{X: 100, Y: 100})
	b := enemyAt(t, "goblin_t1", engine.Vec2{X: 130, Y: 100})
	sq.Add(a)
	sq.Add(b)

	const gap int64 = 42
	if !sq.TakeGap(a, gap) {
		t.Fatal("первый не смог занять проход")
	}
	if sq.TakeGap(b, gap) {
		t.Error("второй влез в занятый проход")
	}
	if !sq.TakeGap(a, gap) {
		t.Error("занявший потерял своё место")
	}
	sq.LeaveGap(a)
	if !sq.TakeGap(b, gap) {
		t.Error("освобождённый проход не достался второму")
	}
}

// TestRefillAfterWipe — вырезанная местность снова становится опасной.
//
// Проверяется и то, что бюджет освобождается за убитыми (иначе карта осталась
// бы навсегда «полной» из мёртвых), и то, что на зачищенном месте подсев идёт
// заметно быстрее обычного: иначе выгодной тактикой становится выкосить пятачок
// и жить на нём.
func TestRefillAfterWipe(t *testing.T) {
	s, cfg, _ := enemySpawner(t, "forest")
	s.Populate(false)
	player := engine.Vec2{X: worldSide / 2, Y: worldSide / 2}
	tgt := mob.Target{Pos: player, Radius: 7, Alive: true}

	// Запоминаем поимённо: «пустой карты» после зачистки не увидеть — новые
	// появляются раньше, чем истлевают трупы, и это правильно.
	born := map[*mob.Enemy]bool{}
	for _, e := range s.Enemies() {
		born[e] = true
		e.Hurt(1_000_000, e.Pos)
	}
	if len(born) == 0 {
		t.Fatal("карта не заселилась")
	}

	cleared := false
	for range 900 {
		s.Update(tgt, true, 0, false)
		left := 0
		for _, e := range s.Enemies() {
			if born[e] {
				left++
			}
		}
		if left == 0 {
			cleared = true
			break
		}
	}
	if !cleared {
		t.Fatal("убитые не убрались с карты: трупы висят вечно")
	}

	// Место, которое они занимали, обязано освободиться — иначе подсев
	// больше никогда не сработает.
	fresh := 0
	for _, e := range s.Enemies() {
		if e.Alive() {
			fresh++
		}
	}
	if fresh == 0 {
		t.Error("на зачищенной местности никто не появился")
	}

	for range 900 {
		s.Update(tgt, true, 0, false)
	}
	alive := 0
	for _, e := range s.Enemies() {
		if e.Alive() {
			alive++
		}
	}
	if alive < 3 {
		t.Errorf("за 15 секунд после зачистки набралось всего %d врагов", alive)
	}
	if d := s.Danger(); d < cfg.Danger.NearBudget*0.5 {
		t.Errorf("опасность восстановилась лишь до %.0f при бюджете у игрока %.0f",
			d, cfg.Danger.NearBudget)
	}
}

// TestClearedGroundRefillsFaster — на зачищенной местности мир торопится, на
// населённой держит спокойный темп.
func TestClearedGroundRefillsFaster(t *testing.T) {
	s, cfg, _ := enemySpawner(t, "forest")
	player := engine.Vec2{X: worldSide / 2, Y: worldSide / 2}
	tgt := mob.Target{Pos: player, Radius: 7, Alive: true}

	first := 0
	for i := 1; i <= 600; i++ {
		s.Update(tgt, true, 0, false)
		if len(s.Enemies()) > 0 {
			first = i
			break
		}
	}
	if first == 0 {
		t.Fatal("на пустой карте так никто и не появился")
	}
	// Быстрый темп — 12 тиков; с шестью попытками на цикл первый враг обязан
	// появиться за считаные циклы, а не за десяток обычных.
	if slow := cfg.Rate.IntervalTicks * 3; first > slow {
		t.Errorf("первый враг появился на %d тике — не похоже на ускоренный подсев (%d тиков на цикл)",
			first, cfg.Rate.EmptyIntervalTicks)
	}
}
