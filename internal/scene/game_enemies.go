package scene

import (
	"math/rand/v2"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/vladislav/game/internal/assets"
	"github.com/vladislav/game/internal/character"
	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/engine"
	"github.com/vladislav/game/internal/mob"
	"github.com/vladislav/game/internal/physics"
	"github.com/vladislav/game/internal/sprite"
	"github.com/vladislav/game/internal/ui"
)

// Враги в забеге: подключение популяции к сцене.
//
// Отдельный файл, потому что связка с врагами шире звериной: у них своя таблица
// поведения, свой спавнер с бюджетом опасности и обмен ударами в обе стороны.
// Сама сцена по-прежнему ничего не решает — только сводит готовые части.

// Шум героя: насколько далеко его слышно в этот тик. Крадущегося шага не
// существует — стоящий не шумит вовсе, идущий слышен вблизи, бегущий и бьющий
// далеко. Числа отсюда сравниваются с hearing профиля врага.
const (
	noiseWalk   = 120
	noiseRun    = 260
	noiseAttack = 420
)

// newEnemySpawner собирает популяцию врагов для карты забега. Сид тот же, что у
// зверей: одна и та же карта заселяется одинаково.
func newEnemySpawner(l *assets.Loader, m mob.SpawnWorld, packs mob.PackFunc, seed uint64) (*mob.EnemySpawner, error) {
	cat, err := mob.LoadEnemies(l.FS(), enemiesRoot+"/enemies.json")
	if err != nil {
		return nil, err
	}
	bhv, err := mob.LoadBehavior(l.FS(), enemiesRoot+"/behavior.json")
	if err != nil {
		return nil, err
	}
	cfg, err := mob.LoadEnemySpawn(l.FS(), enemiesRoot+"/spawn.json")
	if err != nil {
		return nil, err
	}
	rng := rand.New(rand.NewPCG(seed, 0xB5026F5AA96619E9))
	return mob.NewEnemySpawner(cfg, cat, bhv, m, packs, rng), nil
}

// enemyPack — загрузчик паков врагов с кэшем. Имя приходит от спавнера в виде
// "demon/t1": тип и тир, ровно как лежит на диске.
func (g *Game) enemyPack(art string) (*sprite.Pack, error) {
	if p, ok := g.epacks[art]; ok {
		return p, nil
	}
	p, err := sprite.Load(g.l, enemiesRoot+"/"+art)
	if err != nil {
		return nil, err
	}
	g.epacks[art] = p
	return p, nil
}

// target — герой глазами врагов: где он, слышно ли его и не занёс ли оружие.
//
// Телеграф замаха отдаётся честно, потому что игрок видит его на экране: враг
// не читает мысли, он видит то же самое. Без этого уклонение было бы
// подглядыванием в чужие внутренности.
func (g *Game) target() mob.Target {
	t := mob.Target{
		Pos:    g.pl.Pos,
		Radius: g.pl.Radius(),
		Alive:  g.pl.Alive(),
	}
	switch g.pl.State() {
	case character.Walk:
		t.Noise = noiseWalk
	case character.Run:
		t.Noise = noiseRun
	case character.Attacking:
		t.Noise = noiseAttack
		a := g.pl.Loadout.Attack
		t.Windup = true
		t.WindupFace = g.pl.FaceVec()
		t.WindupReach = a.Reach + g.pl.Radius()
		t.WindupArc = a.Arc
	}
	return t
}

// strikeEnemies раздаёт удар героя врагам. Вызывается из strike() тем же
// сектором, что и по зверям: цель одна — попасть по всему, что в нём стоит.
func (g *Game) strikeEnemies(h character.Hit) {
	for _, e := range g.es.Enemies() {
		if !e.Alive() || !h.Covers(e.Pos, e.Radius()) {
			continue
		}
		if e.Hurt(h.Damage, h.Center) {
			g.fx.pop(g.enemyHead(e), e.XP, fxDamage) // добил — всплывает опыт
		} else {
			g.fx.pop(g.enemyHead(e), h.Damage, fxDamage)
		}
		g.ebars[e] = barShow
	}
}

// enemyHits — ответный урон: сектор врага отдаёт его автомат, а попал ли он по
// герою, решает сцена. Неуязвимость после удара считает сам персонаж.
func (g *Game) enemyHits() {
	for _, e := range g.es.Enemies() {
		h, ok := e.Strike()
		if !ok {
			continue
		}
		g.fx.swing(h.Center, h.Face, h.Reach, h.Arc)
		if !g.pl.Alive() || g.pl.Floor() != e.Floor() {
			continue
		}
		if h.Covers(g.pl.Pos, g.pl.Radius()) {
			g.pl.Damage(h.Damage, h.Center)
		}
	}
}

// separateEnemies разводит тела врагов: между собой, с героем и со зверями.
// Правила те же, что у зверей (см. separate): доли перекрытия, сравнение этажей
// и смещение через поле, чтобы никого не вытолкнуть в скалу.
func (g *Game) separateEnemies() {
	f := g.m.Field()
	enemies := g.es.Enemies()
	for i, a := range enemies {
		if !a.Alive() {
			continue
		}
		for _, b := range enemies[i+1:] {
			if !b.Alive() || a.Floor() != b.Floor() {
				continue
			}
			if d, ok := physics.Separate(a.Pos, a.Radius(), b.Pos, b.Radius()); ok {
				a.Push(f, d)
				b.Push(f, d.Scale(-1))
			}
		}
		for _, an := range g.sp.Animals() {
			if !an.Alive() || an.Floor() != a.Floor() {
				continue
			}
			if d, ok := physics.Separate(a.Pos, a.Radius(), an.Pos, an.Radius()); ok {
				a.Push(f, d.Scale(0.5))
				an.Push(f, d.Scale(-1.5))
			}
		}
	}
	if !g.pl.Alive() {
		return
	}
	for _, e := range enemies {
		if !e.Alive() || e.Floor() != g.pl.Floor() {
			continue
		}
		if d, ok := physics.Separate(g.pl.Pos, g.pl.Radius(), e.Pos, e.Radius()); ok {
			g.pl.Push(f, d.Scale(2*playerShare))
			e.Push(f, d.Scale(-2*(1-playerShare)))
		}
	}
}

// tickEnemyBars гасит полоски здоровья врагов, которых давно не трогали.
func (g *Game) tickEnemyBars() {
	for e, ttl := range g.ebars {
		if ttl <= 1 || !e.Alive() || e.Gone() {
			delete(g.ebars, e)
			continue
		}
		g.ebars[e] = ttl - 1
	}
}

// enemyHead — точка над врагом: туда всплывают числа и там висит полоска.
func (g *Game) enemyHead(e *mob.Enemy) engine.Vec2 {
	return engine.Vec2{X: e.Pos.X, Y: e.Pos.Y - float64(e.Pack.Bounds().H)*0.7 - 4}
}

// drawEnemyBars рисует полоски здоровья задетых врагов. Имя показывается всегда:
// в отличие от зверей, знать, кто именно тебя бьёт, важно — от этого зависит,
// драться или бежать.
func (g *Game) drawEnemyBars(dst *ebiten.Image) {
	const w, h = 28, 3
	for e := range g.ebars {
		head := g.cam.WorldToScreen(g.enemyHead(e))
		x, y := float32(head.X)-w/2, float32(head.Y)
		if x < -w || x > config.ScreenW || y < -h || y > config.ScreenH {
			continue
		}
		frac := float32(e.HP) / float32(max(e.MaxHP, 1))
		vector.FillRect(dst, x-1, y-1, w+2, h+2, hudBack, false)
		vector.FillRect(dst, x, y, w*frac, h, hudLow, false)
		name := e.Tier.Title.RU
		if e.Elite {
			name = "★ " + name
		}
		ui.PixelTextCentered(dst, name, head.X, float64(y)-9, 1, hudText)
	}
}
