package mob

import (
	"math"
	"math/rand/v2"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/vladislav/game/internal/anim"
	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/engine"
	"github.com/vladislav/game/internal/physics"
	"github.com/vladislav/game/internal/sprite"
)

// State — что животное делает прямо сейчас.
type State uint8

const (
	Idle   State = iota // стоит
	Wander              // бредёт к случайной точке рядом
	Flee                // убегает от угрозы
	Chase               // идёт на угрозу
	Attack              // бьёт
	Hurt                // получило удар
	Dead                // умерло
)

// Ctx — окружение одного тика. Угроза и добыча приходят снаружи, потому что
// знать про всех соседей — работа игрового слоя, а не отдельного зверя.
type Ctx struct {
	// Field — поле физики карты; nil означает «стен нет» (меню, тесты).
	Field  *physics.Field
	Threat engine.Vec2 // обычно игрок
	// HasThreat=false — угрозы на карте нет (меню, пустая сцена).
	HasThreat bool
	// Prey — ближайшая добыча для хищника (nil, если некого есть).
	Prey *Animal
}

const (
	decideMin  = 40  // тиков между решениями «куда пойти»
	decideMax  = 200 //
	hurtTicks  = 24  // столько длится реакция на удар без клипа hurt
	fadeTicks  = 45  // столько тает труп без клипа death
	wanderDist = 96  // радиус блуждания вокруг дома
)

// Animal — одна особь. Вид даёт намерения (кто это и как себя ведёт), пак —
// исполнение (какие клипы вообще есть). Расхождение между ними закрывается
// деградацией в clipFor: намерение всегда выполнимо, даже если анимации нет.
type Animal struct {
	Species *Species
	Pack    *sprite.Pack
	Pos     engine.Vec2
	HP      int
	// XP и Level — то же, что у врага: бросок по полосе вида, сделанный при
	// рождении, и уровень, выведенный из чисел вида.
	XP    int
	Level int
	// Persistent — особь не снимается спавнером по удалению от игрока
	// (прирученные, сюжетные, поставленные вручную).
	Persistent bool

	home    engine.Vec2
	vel     engine.Vec2
	floor   uint8 // этаж: 0 — низ, 1 — макушка плато (physics.Floor*)
	dir     sprite.Dir
	state   State
	clip    string // имя проигрываемого клипа ("" — клипа нет)
	player  *anim.Player
	timer   int // тиков до следующего решения
	flash   int // остаток вспышки при ударе без клипа hurt
	fade    int // остаток растворения трупа без клипа death
	gone    bool
	swim    bool
	rng     *rand.Rand
	lastDmg int // тик-счётчик перезарядки удара
}

// NewAnimal создаёт особь вида sp с паком pack в точке pos.
func NewAnimal(sp *Species, pack *sprite.Pack, pos engine.Vec2, rng *rand.Rand) *Animal {
	a := &Animal{
		Species: sp,
		Pack:    pack,
		Pos:     pos,
		home:    pos,
		HP:      sp.Stats.HP,
		Level:   sp.Level(),
		player:  anim.NewPlayer(nil),
		rng:     rng,
	}
	a.XP = sp.XP.Pick(XPSeed(sp.ID, pos))
	a.enter(Idle)
	return a
}

// Alive — жив ли зверь.
func (a *Animal) Alive() bool { return a.state != Dead }

// Gone — труп доигран и особь пора убрать из мира.
func (a *Animal) Gone() bool { return a.gone }

// State — текущее состояние (для отладки и для игрового слоя).
func (a *Animal) State() State { return a.state }

// Frame — текущий кадр (nil, если рисовать нечего).
func (a *Animal) Frame() *ebiten.Image { return a.player.Frame() }

// Dir — направление взгляда.
func (a *Animal) Dir() sprite.Dir { return a.dir }

// Radius — радиус тела в мировых пикселях, из рамки непрозрачных пикселей.
// Оттуда же, откуда точка опоры: размеры кадра для этого не годятся.
func (a *Animal) Radius() float64 { return packRadius(a.Pack) }

func packRadius(p *sprite.Pack) float64 {
	b := p.Bounds()
	return math.Max(2, float64(b.W)/4)
}

// Hit наносит урон. Возвращает true, если удар добил.
func (a *Animal) Hit(dmg int) bool {
	if a.state == Dead {
		return false
	}
	a.HP -= dmg
	if a.HP <= 0 {
		a.HP = 0
		a.enter(Dead)
		return true
	}
	a.enter(Hurt)
	return false
}

// Update продвигает животное на один тик логики.
func (a *Animal) Update(c Ctx) {
	if a.gone {
		return
	}
	a.player.Update()

	switch a.state {
	case Dead:
		a.updateDead()
		return
	case Hurt:
		if a.doneReacting() {
			a.decide(c)
		}
		return
	case Attack:
		if a.doneReacting() {
			a.enter(Chase)
		}
		return
	}

	if a.timer--; a.timer <= 0 {
		a.decide(c)
	}
	a.step(c)
	a.retarget(c)
}

// decide выбирает состояние по характеру вида и обстановке. Профилей характера
// пять на все тридцать видов — различия между зверями живут в данных, а не
// здесь.
func (a *Animal) decide(c Ctx) {
	a.timer = decideMin + a.rng.IntN(decideMax-decideMin)
	sp := a.Species
	d := math.Inf(1)
	if c.HasThreat && a.sameFloor(c, c.Threat) {
		d = engine.Dist(a.Pos, c.Threat)
	}

	switch sp.Temper {
	case "skittish":
		if d < sp.Threat.Sight {
			a.enter(Flee)
			return
		}
	case "territorial":
		if d < sp.Threat.Sight && sp.Threat.Attacks && a.wantsFight() {
			a.enter(Chase)
			return
		}
		if d < sp.Threat.Sight/2 && !a.wantsFight() {
			a.enter(Flee)
			return
		}
	case "predator":
		if d < sp.Threat.Sight {
			a.enter(Flee) // от игрока хищник уходит, драться лезет только за добычей
			return
		}
		if c.Prey != nil && c.Prey.Alive() {
			a.enter(Chase)
			return
		}
	case "tame":
		if engine.Dist(a.Pos, a.home) > wanderDist {
			a.enter(Wander)
			return
		}
	case "passive":
		if d < sp.Threat.Sight/3 {
			a.enter(Flee)
			return
		}
	}

	// Спокойная обстановка: постоять или побродить. Вид без клипа idle стоять не
	// умеет — подменять idle нечем, поэтому он просто всегда в движении.
	if a.rng.IntN(3) == 0 && a.Pack.Has("idle") {
		a.enter(Idle)
		return
	}
	a.enter(Wander)
}

// sameFloor — на одном ли этаже зверь и точка p. Через этаж цель не считается:
// бежать к ней всё равно некуда — единственный путь наверх это лестница, а
// искать её звери не умеют. Без этой проверки волк с макушки плато вечно тёрся
// бы носом в обрыв, под которым стоит игрок.
func (a *Animal) sameFloor(c Ctx, p engine.Vec2) bool {
	if c.Field == nil {
		return true
	}
	return c.Field.CellAt(p).Floor() == a.floor
}

// wantsFight — не пора ли территориальному зверю передумать драться.
func (a *Animal) wantsFight() bool {
	sp := a.Species
	if sp.Stats.HP <= 0 {
		return false
	}
	return float64(a.HP)/float64(sp.Stats.HP) > sp.Threat.FleeHP
}

// retarget обновляет цель движения внутри уже выбранного состояния: убегать и
// догонять надо каждый тик, а не раз в decide.
func (a *Animal) retarget(c Ctx) {
	switch a.state {
	case Flee:
		if c.HasThreat && a.sameFloor(c, c.Threat) {
			a.vel = a.Pos.Sub(c.Threat).Normalized().Scale(a.speed())
		}
	case Chase:
		t, ok := a.chaseTarget(c)
		if !ok {
			a.enter(Wander)
			return
		}
		if engine.Dist(a.Pos, t) <= a.reach() {
			a.enter(Attack)
			return
		}
		a.vel = t.Sub(a.Pos).Normalized().Scale(a.speed())
	}
}

func (a *Animal) chaseTarget(c Ctx) (engine.Vec2, bool) {
	if a.Species.Temper == "predator" {
		if c.Prey != nil && c.Prey.Alive() && a.sameFloor(c, c.Prey.Pos) {
			return c.Prey.Pos, true
		}
		return engine.Vec2{}, false
	}
	if !c.HasThreat || !a.wantsFight() || !a.sameFloor(c, c.Threat) {
		return engine.Vec2{}, false
	}
	return c.Threat, true
}

// step двигает зверя по полю и переключает сушу/воду. Стены, скольжение вдоль
// них и этажи держит physics: здесь — только скорость и то, что зверь упёрся.
func (a *Animal) step(c Ctx) {
	if a.vel.Len() == 0 {
		return
	}
	d := a.vel.Scale(c.Field.SpeedScale(a.Pos) / config.TPS)
	next, floor := c.Field.Move(a.Pos, d, a.Body())
	if engine.Dist(next, a.Pos) < 0.01 {
		a.vel = engine.Vec2{}
		a.timer = 0 // упёрлись — решаем заново на следующем тике
		return
	}
	a.Pos, a.floor = next, floor
	a.swim = c.Field.CellAt(next).Liquid()
	a.dir = sprite.DirFrom(a.vel)
	a.play(a.clipFor(a.state))
}

// Body — тело зверя для поля.
func (a *Animal) Body() physics.Body { return bodyOf(a.Species, a.Pack, a.floor) }

// bodyOf — тело вида sp с паком pack на этаже floor. Возможности берутся у
// вида, но с оглядкой на пак: намерение без клипа не выпускает зверя на глубину,
// иначе он поплывёт стоя. Мелководье доступно и без клипа swim — там бредут.
func bodyOf(sp *Species, pack *sprite.Pack, floor uint8) physics.Body {
	water := sp.Locomotion.Water
	return physics.Body{
		Radius: packRadius(pack),
		Floor:  floor,
		Caps:   physics.Caps{Wade: water, Swim: water && pack.Has("swim")},
	}
}

// Land ставит зверя на этаж той клетки, в которой он оказался: спавнер выбирает
// точку, а этаж у неё свой (макушка плато — верхний).
func (a *Animal) Land(f *physics.Field) { a.floor = f.CellAt(a.Pos).Floor() }

// Floor — этаж, на котором стоит зверь (0 — низ, 1 — макушка плато).
func (a *Animal) Floor() uint8 { return a.floor }

// Push сдвигает зверя на delta с проверкой стен: этим игровой слой
// расталкивает тела, которые налезли друг на друга.
func (a *Animal) Push(f *physics.Field, delta engine.Vec2) {
	a.Pos, a.floor = f.Move(a.Pos, delta, a.Body())
}

func (a *Animal) speed() float64 {
	s := a.Species.Stats.Speed
	if a.swim {
		if s.Swim > 0 {
			return s.Swim
		}
		return s.Walk
	}
	switch a.state {
	case Flee, Chase:
		if s.Run > 0 {
			return s.Run
		}
	}
	return s.Walk
}

// reach — с какой дистанции зверь достаёт цель.
func (a *Animal) reach() float64 { return a.Radius() + 8 }

// enter переводит в состояние s и заряжает подходящий клип.
func (a *Animal) enter(s State) {
	a.state = s
	switch s {
	case Idle:
		a.vel = engine.Vec2{}
	case Wander:
		ang := a.rng.Float64() * 2 * math.Pi
		dist := 24 + a.rng.Float64()*wanderDist
		t := a.home.Add(engine.Vec2{X: math.Cos(ang) * dist, Y: math.Sin(ang) * dist})
		a.vel = t.Sub(a.Pos).Normalized().Scale(a.speed())
		a.dir = sprite.DirFrom(a.vel)
	case Hurt:
		a.vel = engine.Vec2{}
		if !a.Pack.Has("hurt") {
			a.flash = hurtTicks
		}
	case Dead:
		a.vel = engine.Vec2{}
		if !a.Pack.Has("death") {
			a.fade = fadeTicks
		}
	case Attack:
		a.vel = engine.Vec2{}
		a.lastDmg = hurtTicks
	}
	a.play(a.clipFor(s))
}

// clipFor — чем показать состояние s. Здесь и живёт деградация: набор анимаций
// у паков рваный (attack есть только у кабана, run — у пяти видов), поэтому
// состояние никогда не зависит от наличия клипа.
//
//	состояние        клип             если клипа нет
//	Idle             idle             состояние не выбирается (см. decide)
//	Wander           walk/swim        —
//	Flee, Chase      run              walk ускоренный вдвое
//	Attack           attack           рывок на walk/run
//	Hurt             hurt             вспышка (flash)
//	Dead             death            растворение (fade)
func (a *Animal) clipFor(s State) (string, *anim.Clip) {
	pick := func(names ...string) (string, *anim.Clip) {
		for _, n := range names {
			if c := a.Pack.Clip(n, a.dir); c != nil {
				return n, c
			}
		}
		return "", nil
	}
	if a.swim {
		switch s {
		case Idle:
			return pick("swim_idle", "swim")
		default:
			return pick("swim", "swim_idle")
		}
	}
	switch s {
	case Idle:
		return pick("idle")
	case Wander:
		return pick("walk", "run")
	case Flee, Chase:
		if n, c := pick("run"); c != nil {
			return n, c
		}
		// Имя подменыша отличается от исходного намеренно: play() сравнивает
		// имена, и без этого переход «бреду» → «бегу» на том же walk остался бы
		// с прежним, медленным таймингом.
		if _, c := pick("walk"); c != nil {
			return "run*", faster(c)
		}
		return "", nil
	case Attack:
		if n, c := pick("attack"); c != nil {
			return n, c
		}
		if _, c := pick("run", "walk"); c != nil {
			return "attack*", faster(c) // рывок вместо удара
		}
		return "", nil
	case Hurt:
		return pick("hurt", "idle", "walk")
	case Dead:
		return pick("death", "hurt", "idle", "walk")
	}
	return "", nil
}

// faster — тот же клип вдвое быстрее. Кадры общие, копируется только тайминг,
// поэтому подмена бега походкой ничего не стоит по памяти.
func faster(c *anim.Clip) *anim.Clip {
	return &anim.Clip{Frames: c.Frames, FrameTicks: max(1, c.FrameTicks/2), Loop: c.Loop}
}

// play заряжает клип, если это не тот же самый, что уже идёт.
func (a *Animal) play(name string, c *anim.Clip) {
	if name == a.clip && c != nil {
		return
	}
	a.clip = name
	a.player.Play(c)
}

// doneReacting — отыграла ли короткая реакция (удар, получение урона).
func (a *Animal) doneReacting() bool {
	if a.flash > 0 {
		a.flash--
		return a.flash == 0
	}
	if a.lastDmg > 0 {
		a.lastDmg--
		return a.lastDmg == 0
	}
	return a.player.Finished() || !a.player.Clip().Valid()
}

func (a *Animal) updateDead() {
	if a.fade > 0 {
		if a.fade--; a.fade == 0 {
			a.gone = true
		}
		return
	}
	if a.player.Finished() || !a.player.Clip().Valid() {
		a.fade = fadeTicks // клип смерти доигран — труп тает и убирается
	}
}

// Draw рисует животное так, чтобы его точка опоры легла в screen. Вспышка при
// ударе и растворение трупа — тоже здесь: это подмена недостающих клипов, и
// живёт она рядом с остальной деградацией.
func (a *Animal) Draw(dst *ebiten.Image, screen engine.Vec2) {
	img := a.player.Frame()
	if img == nil || a.gone {
		return
	}
	foot := a.Pack.Foot()
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(screen.X-float64(foot.X), screen.Y-float64(foot.Y))
	switch {
	case a.flash > 0:
		op.ColorScale.Scale(2, 2, 2, 1)
	case a.fade > 0:
		op.ColorScale.ScaleAlpha(float32(a.fade) / fadeTicks)
	}
	dst.DrawImage(img, op)
}
