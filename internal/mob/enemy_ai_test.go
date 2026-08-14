package mob_test

import (
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
	nav := mob.NewNav(f, 32, 8)
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
	nav := mob.NewNav(f, 32, 8)
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
	nav2 := mob.NewNav(open, 32, 8)
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
