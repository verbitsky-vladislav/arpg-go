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

// Враг и его поведение. Данные о том, кто он, лежат в enemies.json (тип и тир),
// о том, как он думает — в behavior.json (профиль). Здесь только исполнение.
//
// Устройство разговора с миром такое же, как у зверя: враг не знает ни про
// карту целиком, ни про других врагов. Всё, что ему нужно снаружи, приходит в
// EnemyCtx: цель, проходимость и отряд, через который враги договариваются.

// EnemyState — что враг делает прямо сейчас.
type EnemyState uint8

const (
	EIdle    EnemyState = iota // стоит у себя
	EPatrol                    // бредёт по своему участку
	ESuspect                   // услышал шум и идёт смотреть
	EChase                     // идёт на цель (прямо или в обход)
	ECircle                    // кружит рядом, ожидая очереди бить
	EKeep                      // держит дистанцию (стрелок)
	EAttack                    // бьёт
	ESearch                    // ищет у последней известной точки
	EDodge                     // отскакивает из-под замаха
	EReturn                    // возвращается домой
	EHurt                      // получил удар
	EDead                      // мёртв
)

var enemyStateNames = [...]string{
	"idle", "patrol", "suspect", "chase", "circle", "keep", "attack", "search", "dodge", "return", "hurt", "dead",
}

func (s EnemyState) String() string {
	if int(s) >= len(enemyStateNames) {
		return "?"
	}
	return enemyStateNames[s]
}

// Target — цель врага (игрок) глазами ИИ.
type Target struct {
	Pos    engine.Vec2
	Radius float64
	Alive  bool
	// Noise — радиус, на котором в этот тик слышно цель: бег и удары шумят,
	// шаг почти нет. Игровой слой считает его сам, потому что про то, что
	// делает игрок, знает он.
	Noise float64

	// Замах цели: пока он идёт, у врага есть шанс отскочить. Телеграф даёт
	// игровой слой (character.Player знает про свой удар всё), а не ИИ —
	// подглядывать в чужие внутренности врагу нельзя.
	Windup      bool
	WindupFace  engine.Vec2
	WindupReach float64
	WindupArc   float64
}

// EnemyCtx — окружение одного тика.
type EnemyCtx struct {
	// Field — поле физики карты; nil означает «стен нет» (тесты, просмотрщик).
	Field     *physics.Field
	Player    Target
	HasPlayer bool
	// Squad — общий штаб живых врагов: очередь удара, места окружения,
	// перекличка. Может быть nil (одиночная особь в тесте).
	Squad *Squad
	// Nav — общая карта расстояний до цели: по ней враги обходят стены и по
	// ней же меряется слышимость. nil — идти напрямик и слышать по прямой.
	Nav *NavField
}

// Hit — удар врага: сектор перед ним в момент кадра попадания. Тот же смысл,
// что у character.Hit, но пакеты независимы намеренно — герой и мобы не должны
// тянуть друг друга.
type Hit struct {
	Center    engine.Vec2
	Face      engine.Vec2
	Reach     float64
	Arc       float64
	Damage    int
	Knockback float64
}

// Covers — попадает ли цель радиуса radius в сектор удара.
func (h Hit) Covers(p engine.Vec2, radius float64) bool {
	d := p.Sub(h.Center)
	dist := d.Len()
	if dist > h.Reach+radius {
		return false
	}
	if dist <= radius {
		return true
	}
	return d.Normalized().Dot(h.Face) >= math.Cos(h.Arc/2*math.Pi/180)
}

const (
	enemyHurtTicks = 20 // оцепенение от удара без клипа hurt
	enemyFadeTicks = 60 // растворение трупа
	enemyArc       = 90 // раствор сектора удара врага, градусы
)

// Enemy — одна особь.
type Enemy struct {
	Type   *EnemyType
	Tier   *Tier
	Pack   *sprite.Pack
	Bhv    Behavior
	Pos    engine.Vec2
	HP     int
	MaxHP  int
	Damage int
	XP     int
	Elite  bool

	home  engine.Vec2
	vel   engine.Vec2
	dir   sprite.Dir
	state EnemyState

	clip    string
	clipDir sprite.Dir
	player  *anim.Player

	// Восприятие.
	seen      bool        // видит цель прямо сейчас
	lastSeen  engine.Vec2 // где видел в последний раз
	knows     bool        // есть ли что вспоминать
	react     int         // остаток задержки реакции
	lose      int         // тиков осталось «по инерции» считать цель видимой
	memory    int         // остаток памяти о последней точке
	search    int         // остаток поиска
	suspect   int         // остаток похода на шум
	suspectAt engine.Vec2

	// Бой.
	commit   int         // тиков до права сменить решение
	cooldown int         // перезарядка удара
	struck   bool        // урон этого замаха уже выдан
	pending  *Hit        // удар ждёт игровой слой
	atkFace  engine.Vec2 // направление зафиксировано в начале замаха
	atkTicks int
	atkFrame int
	token    bool // разрешение бить из отряда
	slot     int  // место в круге окружения (-1 — не назначено)
	role     Role // что он делает в группе
	dodge    int  // остаток отскока
	dodgeCD  int  // перезарядка отскока
	dodgeVec engine.Vec2
	flanker  bool // заходит сбоку, а не в лоб
	circleCW bool // в какую сторону кружит

	wait    int // стоять столько тиков (патруль)
	goal    engine.Vec2
	hasGoal bool

	flash int
	fade  int
	gone  bool
	floor uint8 // этаж: 0 — низ, 1 — макушка плато
	rng   *rand.Rand
}

// Body — тело врага для поля. Летающим вода не преграда, ходячим — преграда:
// брода в данных врагов нет, а плавающих врагов пока не нарисовано.
func (e *Enemy) Body() physics.Body {
	fly := e.flies()
	return physics.Body{
		Radius: packRadius(e.Pack),
		Floor:  e.floor,
		Caps:   physics.Caps{Wade: fly, Swim: fly},
	}
}

// Land ставит врага на этаж той клетки, где он оказался.
func (e *Enemy) Land(f *physics.Field) { e.floor = f.CellAt(e.Pos).Floor() }

// Floor — этаж, на котором стоит враг.
func (e *Enemy) Floor() uint8 { return e.floor }

// Push сдвигает врага на delta с проверкой стен: этим игровой слой
// расталкивает тела, налезшие друг на друга.
func (e *Enemy) Push(f *physics.Field, delta engine.Vec2) {
	e.Pos, e.floor = f.Move(e.Pos, delta, e.Body())
}

// NewEnemy создаёт особь тира t с паком pack и профилем поведения bhv.
func NewEnemy(t *Tier, pack *sprite.Pack, bhv Behavior, pos engine.Vec2, rng *rand.Rand) *Enemy {
	e := &Enemy{
		Type: t.Type, Tier: t, Pack: pack, Bhv: bhv,
		Pos: pos, home: pos,
		HP: t.HP, MaxHP: t.HP, Damage: t.Damage, XP: t.XP,
		player: anim.NewPlayer(nil),
		slot:   -1,
		rng:    rng,
	}
	e.flanker = rng.Float64() < bhv.Group.Flank
	e.circleCW = rng.IntN(2) == 0
	e.enter(EIdle)
	return e
}

// MakeElite усиливает особь по правилам спавна.
func (e *Enemy) MakeElite(hpScale, dmgScale, xpScale float64) {
	e.Elite = true
	e.MaxHP = max(1, int(float64(e.MaxHP)*hpScale))
	e.HP = e.MaxHP
	e.Damage = max(1, int(float64(e.Damage)*dmgScale))
	e.XP = int(float64(e.XP) * xpScale)
}

func (e *Enemy) Alive() bool           { return e.state != EDead }
func (e *Enemy) Gone() bool            { return e.gone }
func (e *Enemy) State() EnemyState     { return e.state }
func (e *Enemy) Dir() sprite.Dir       { return e.dir }
func (e *Enemy) Frame() *ebiten.Image  { return e.player.Frame() }
func (e *Enemy) Clip() string          { return e.clip }
func (e *Enemy) Engaged() bool         { return e.knows && e.Alive() }
func (e *Enemy) Aware() bool           { return e.seen }
func (e *Enemy) LastSeen() engine.Vec2 { return e.lastSeen }

// Radius — радиус тела, как у зверей: четверть ширины рамки непрозрачных
// пикселей.
func (e *Enemy) Radius() float64 { return packRadius(e.Pack) }

// Speed — скорость в текущем состоянии.
func (e *Enemy) Speed() float64 {
	s := e.Tier.Speed
	v := s.Walk
	switch e.state {
	case EChase, ECircle, EKeep, ESearch, EReturn:
		if s.Run > 0 {
			v = s.Run
		}
	}
	return v
}

// Strike забирает состоявшийся удар — один раз за замах.
func (e *Enemy) Strike() (Hit, bool) {
	if e.pending == nil {
		return Hit{}, false
	}
	h := *e.pending
	e.pending = nil
	return h, true
}

// Hit наносит урон врагу. Возвращает true, если удар добил.
//
// Урон всегда «будит»: даже если враг не видел, откуда прилетело, он теперь
// знает, что цель рядом, и пойдёт её искать. Иначе можно бить в спину вечно.
func (e *Enemy) Hurt(dmg int, from engine.Vec2) bool {
	if e.state == EDead {
		return false
	}
	e.HP -= dmg
	e.knows, e.lastSeen = true, from
	e.memory = e.Bhv.Perception.MemoryTicks
	e.react = 0 // по боли реагируют мгновенно
	if e.HP <= 0 {
		e.HP = 0
		e.enter(EDead)
		return true
	}
	if e.Bhv.Combat.Flinch {
		e.enter(EHurt)
	}
	return false
}

// Vanish убирает особь с карты без смерти: так спавнер снимает тех, кто остался
// далеко позади. Не «умер» — именно исчез, поэтому ни дропа, ни опыта.
func (e *Enemy) Vanish() {
	e.state = EDead
	e.gone = true
	e.pending = nil
}

// Update продвигает врага на один тик.
func (e *Enemy) Update(c EnemyCtx) {
	if e.gone {
		return
	}
	e.player.Update()
	e.tickTimers()

	switch e.state {
	case EDead:
		e.updateDead()
		return
	case EHurt:
		if e.doneReacting() {
			e.decide(c)
		}
		return
	case EAttack:
		e.updateAttack(c)
		return
	}

	e.perceive(c)
	if e.tryDodge(c) {
		e.act(c)
		return
	}
	if e.commit <= 0 {
		e.decide(c)
	}
	e.act(c)
}

func (e *Enemy) tickTimers() {
	dec := func(v *int) {
		if *v > 0 {
			*v--
		}
	}
	dec(&e.commit)
	dec(&e.cooldown)
	dec(&e.react)
	dec(&e.lose)
	dec(&e.memory)
	dec(&e.search)
	dec(&e.suspect)
	dec(&e.wait)
	dec(&e.dodge)
	dec(&e.dodgeCD)
	if e.memory == 0 {
		e.knows = false
	}
}

// perceive — единственное место, где враг узнаёт про игрока. Видит он в
// секторе перед собой и только по прямой видимости; слышит — вокруг себя,
// сквозь стены и спину.
func (e *Enemy) perceive(c EnemyCtx) {
	p := e.Bhv.Perception
	e.seen = false
	if !c.HasPlayer || !c.Player.Alive {
		return
	}
	d := c.Player.Pos.Sub(e.Pos)
	dist := d.Len()

	if dist <= e.Type.Threat.Sight && e.inFOV(d) && losClear(c.Field, e.Pos, c.Player.Pos) {
		e.seen = true
		e.lose = p.LoseTicks
		if !e.knows {
			e.react = p.ReactionTicks // впервые заметил — не бросается в тот же тик
			if c.Squad != nil {
				c.Squad.Shout(e, c.Player.Pos)
			}
		}
		e.knows = true
		e.lastSeen = c.Player.Pos
		e.memory = p.MemoryTicks
		e.search = p.SearchTicks
		return
	}

	// Слух: шум слышно со спины и из-за угла, но не сквозь скалу. Меряется он
	// по коридорам (расстояние из общей карты навигации), а не по прямой:
	// за стеной звук глохнет, даже если по воздуху рукой подать.
	heard := dist
	if c.Nav != nil {
		// Недостижимое не слышно вовсе: если обхода нет, значит и звуку
		// пройти негде. Без этого за глухой стеной слышимость чудом
		// возвращалась к прямой линии.
		if nd, ok := c.Nav.Dist(e.Pos, e.floor); ok {
			heard = nd
		} else {
			heard = math.Inf(1)
		}
	}
	if c.Player.Noise > 0 && heard <= math.Min(p.Hearing, c.Player.Noise) {
		if !e.knows && e.suspect == 0 {
			e.suspect = p.SuspicionTicks
			e.suspectAt = c.Player.Pos
		} else if e.knows {
			e.lastSeen = c.Player.Pos
			e.memory = p.MemoryTicks
		}
	}
}

// tryDodge — отскок из-под замаха. Враг не читает чужие мысли: он видит
// занесённое оружие (телеграф в Target) и, если успевает и умеет, уходит вбок.
// Назад отскакивать бессмысленно — из сектора выходят в сторону.
func (e *Enemy) tryDodge(c EnemyCtx) bool {
	if e.dodge > 0 {
		return true // отскок уже идёт
	}
	skill := e.Bhv.Combat.Dodge
	if skill <= 0 || e.dodgeCD > 0 || !c.HasPlayer || !c.Player.Windup || !e.seen {
		return false
	}
	if !e.inWindup(c) || e.rng.Float64() >= skill {
		return false
	}
	away := c.Player.Pos.Sub(e.Pos)
	side := engine.Vec2{X: -away.Y, Y: away.X}.Normalized()
	if e.circleCW {
		side = side.Scale(-1)
	}
	if c.Field != nil && !c.Field.Fits(e.Pos.Add(side.Scale(e.Radius()+10)), e.Body()) {
		side = side.Scale(-1) // в ту сторону стена — уходим в другую
	}
	e.dodgeVec = side
	e.dodge = max(1, e.Bhv.Combat.DodgeTicks)
	e.dodgeCD = e.dodge + e.Bhv.Combat.CommitTicks
	e.release(c)
	e.state = EDodge
	e.play(e.clipFor(EDodge))
	return true
}

// inWindup — попадает ли враг в сектор чужого замаха.
func (e *Enemy) inWindup(c EnemyCtx) bool {
	d := e.Pos.Sub(c.Player.Pos)
	if d.Len() > c.Player.WindupReach+e.Radius() {
		return false
	}
	if c.Player.WindupArc <= 0 || d.Len() == 0 {
		return true
	}
	return d.Normalized().Dot(c.Player.WindupFace) >= math.Cos(c.Player.WindupArc/2*math.Pi/180)
}

func (e *Enemy) inFOV(d engine.Vec2) bool {
	if e.Bhv.Perception.FOVDeg >= 360 {
		return true
	}
	if d.Len() == 0 {
		return true
	}
	return d.Normalized().Dot(e.faceVec()) >= math.Cos(e.Bhv.Perception.FOVDeg/2*math.Pi/180)
}

func (e *Enemy) flies() bool { return e.Type.Locomotion.Air }

// decide выбирает состояние. Вся «умность» тут: порядок проверок и есть
// характер, а числа к ним приходят из профиля.
func (e *Enemy) decide(c EnemyCtx) {
	e.commit = e.Bhv.Combat.CommitTicks

	// Ранен и труслив — выходит из боя, но не убегает с карты: отступает и
	// возвращается, когда отдышится.
	if e.wantsRetreat() {
		e.enter(EReturn)
		return
	}
	// Поводок: за своим участком враг не гонится, даже если видит.
	if engine.Dist(e.Pos, e.home) > e.Bhv.Patrol.Leash {
		e.enter(EReturn)
		return
	}

	if e.knows && e.react == 0 && (e.seen || e.lose > 0 || e.memory > 0) {
		e.engage(c)
		return
	}
	if e.suspect > 0 {
		e.enter(ESuspect)
		return
	}
	if e.knows && e.search > 0 {
		e.enter(ESearch)
		return
	}
	if engine.Dist(e.Pos, e.home) > e.Bhv.Patrol.Radius {
		e.enter(EReturn)
		return
	}
	if e.wait > 0 {
		e.enter(EIdle)
		return
	}
	e.enter(EPatrol)
}

// engage выбирает боевое состояние: бить, держать дистанцию, кружить или идти.
func (e *Enemy) engage(c EnemyCtx) {
	dist := engine.Dist(e.Pos, e.target(c))
	reach := e.reach(c)
	cb := e.Bhv.Combat

	// Стрелок: подошли ближе полосы — пятится, дальше полосы — подходит.
	if cb.PreferRange > 0 {
		switch {
		case dist < cb.PreferRange-cb.KeepBand, dist > cb.PreferRange+cb.KeepBand:
			e.enter(EKeep)
			return
		case e.seen && e.cooldown == 0 && e.claim(c):
			e.startAttack(c)
			return
		default:
			e.enter(ECircle)
			return
		}
	}

	if dist <= reach {
		// Вплотную, но бить можно не всем сразу: очередь держит отряд.
		if e.cooldown == 0 && e.claim(c) {
			e.startAttack(c)
			return
		}
		e.enter(ECircle)
		return
	}
	if dist <= reach*2.2 && e.rng.Float64() < cb.Strafe {
		e.enter(ECircle)
		return
	}
	e.enter(EChase)
}

// claim просит у отряда право ударить. Без отряда бьёт кто хочет.
func (e *Enemy) claim(c EnemyCtx) bool {
	if e.token {
		return true
	}
	if c.Squad == nil {
		e.token = true
		return true
	}
	e.token = c.Squad.ClaimAttack(e)
	return e.token
}

func (e *Enemy) release(c EnemyCtx) {
	if e.token && c.Squad != nil {
		c.Squad.ReleaseAttack(e)
	}
	e.token = false
}

func (e *Enemy) wantsRetreat() bool {
	f := e.Type.Threat.FleeHP
	return f > 0 && float64(e.HP)/float64(e.MaxHP) < f
}

// target — куда идти: видимая цель или последняя известная точка.
func (e *Enemy) target(c EnemyCtx) engine.Vec2 {
	if e.seen && c.HasPlayer {
		return c.Player.Pos
	}
	return e.lastSeen
}

func (e *Enemy) reach(c EnemyCtx) float64 {
	return e.Type.Threat.Reach + c.Player.Radius
}

// act исполняет выбранное состояние: считает желаемую скорость и двигает.
func (e *Enemy) act(c EnemyCtx) {
	switch e.state {
	case EIdle:
		e.vel = engine.Vec2{}
	case EPatrol:
		e.wander(c)
	case ESuspect:
		e.moveTo(c, e.suspectAt, 1)
		if engine.Dist(e.Pos, e.suspectAt) < 24 {
			e.suspect = 0
		}
	case EChase:
		e.moveTo(c, e.approachPoint(c), 1)
	case ECircle:
		e.circle(c)
	case EKeep:
		e.keepRange(c)
	case ESearch:
		e.searchAround(c)
	case EDodge:
		e.vel = e.dodgeVec.Scale(e.Speed() * 1.4)
		e.faceTo(c.Player.Pos)
		if e.dodge == 0 {
			e.commit = 0
		}
	case EReturn:
		e.moveTo(c, e.home, 1)
		if engine.Dist(e.Pos, e.home) < 16 {
			e.knows = false
			e.enter(EIdle)
		}
	}
	e.step(c)
}

// approachPoint — куда именно идти к цели. Роль решает, откуда заходить: в лоб,
// сбоку или на отход. Так группа окружает, а не выстраивается в очередь.
func (e *Enemy) approachPoint(c EnemyCtx) engine.Vec2 {
	t := e.target(c)
	if c.Squad != nil {
		if e.role == RoleNone {
			e.role = c.Squad.AssignRole(e)
		}
		if e.role == RoleCutoff {
			// Отрезающий встаёт с той стороны, куда цель побежит: напротив
			// того места, откуда пришла группа.
			away := t.Sub(c.Squad.Center()).Normalized()
			if away.Len() == 0 {
				away = engine.Vec2{X: 1}
			}
			return t.Add(away.Scale(e.reach(c) * 1.4))
		}
	}
	if !e.flanker || c.Squad == nil {
		return t
	}
	slots := e.Bhv.Group.SurroundSlots
	if e.slot < 0 {
		e.slot = c.Squad.Slot(e, slots)
	}
	if e.slot < 0 {
		return t
	}
	// Место в круге считается от направления «цель → враг», чтобы обход шёл
	// с той стороны, где он уже стоит, а не через всю карту.
	base := e.Pos.Sub(t).Angle()
	step := 2 * math.Pi / float64(slots)
	ang := base + step*float64(e.slot%slots)/2
	r := e.reach(c) * 0.9
	return t.Add(engine.Vec2{X: math.Cos(ang) * r, Y: math.Sin(ang) * r})
}

// circle — ходьба боком вокруг цели: враг не стоит столбом в ожидании очереди.
func (e *Enemy) circle(c EnemyCtx) {
	t := e.target(c)
	d := e.Pos.Sub(t)
	if d.Len() == 0 {
		d = engine.Vec2{X: 1}
	}
	tangent := engine.Vec2{X: -d.Y, Y: d.X}.Normalized()
	if !e.circleCW {
		tangent = tangent.Scale(-1)
	}
	// Немного «дышит» по радиусу, иначе орбита выглядит механической.
	want := e.reach(c) * 1.1
	radial := d.Normalized().Scale((want - d.Len()) * 0.02)
	e.vel = tangent.Add(radial).Normalized().Scale(e.Speed() * 0.75)
	e.faceTo(t)
}

// keepRange — поведение стрелка: держать полосу дистанции.
func (e *Enemy) keepRange(c EnemyCtx) {
	t := e.target(c)
	d := e.Pos.Sub(t)
	dist := d.Len()
	cb := e.Bhv.Combat
	switch {
	case dist < cb.PreferRange-cb.KeepBand:
		e.vel = d.Normalized().Scale(e.Speed()) // пятится, не отворачиваясь
	case dist > cb.PreferRange+cb.KeepBand:
		e.vel = d.Normalized().Scale(-e.Speed())
	default:
		e.vel = engine.Vec2{}
	}
	e.faceTo(t)
}

// searchAround — обыск у последней известной точки: дошёл и оглядывается по
// сторонам, а не стоит лицом в стену.
func (e *Enemy) searchAround(c EnemyCtx) {
	if engine.Dist(e.Pos, e.lastSeen) > 20 {
		e.moveTo(c, e.lastSeen, 1)
		return
	}
	e.vel = engine.Vec2{}
	if e.wait > 0 {
		return
	}
	e.wait = 30 + e.rng.IntN(40)
	ang := e.rng.Float64() * 2 * math.Pi
	e.faceTo(e.Pos.Add(engine.Vec2{X: math.Cos(ang) * 32, Y: math.Sin(ang) * 32}))
}

// wander — блуждание по участку вокруг дома.
func (e *Enemy) wander(c EnemyCtx) {
	if !e.hasGoal || engine.Dist(e.Pos, e.goal) < 12 {
		if e.wait > 0 {
			e.vel = engine.Vec2{}
			return
		}
		w := e.Bhv.Patrol.WaitTicks
		e.wait = w[0] + e.rng.IntN(max(1, w[1]-w[0]))
		ang := e.rng.Float64() * 2 * math.Pi
		r := e.Bhv.Patrol.Radius * (0.3 + 0.7*e.rng.Float64())
		e.goal = e.home.Add(engine.Vec2{X: math.Cos(ang) * r, Y: math.Sin(ang) * r})
		e.hasGoal = true
	}
	e.moveTo(c, e.goal, 0.6)
}

// moveTo задаёт скорость к точке с учётом соседей и препятствий.
//
// Направление берётся из общей карты навигации, если она ведёт туда же, куда мы
// идём: тогда враг огибает стену целиком, а не тыкается в неё углом. Без карты
// (или если идём не к цели) остаётся прямая с локальным объездом.
func (e *Enemy) moveTo(c EnemyCtx, to engine.Vec2, scale float64) {
	d := to.Sub(e.Pos)
	if d.Len() < 1 {
		e.vel = engine.Vec2{}
		return
	}
	dir := d.Normalized()
	// Порог щедрый: точка подхода у фланкёра и отрезающего смещена от цели на
	// корпус-другой, но дорога к ней та же самая.
	if !e.flies() && c.Nav != nil && engine.Dist(c.Nav.Goal(), to) < 120 {
		if nd, ok := c.Nav.Step(e.Pos, e.floor); ok {
			dir = nd
		}
	}
	dir = dir.Add(e.separation(c)).Add(e.avoid(c, dir))
	if dir.Len() == 0 {
		dir = d.Normalized()
	}
	e.vel = dir.Normalized().Scale(e.Speed() * scale)
	e.faceTo(to)
}

// separation — расталкивание своих: без него группа сходится в одну точку и
// выглядит одним мигающим спрайтом.
func (e *Enemy) separation(c EnemyCtx) engine.Vec2 {
	if c.Squad == nil {
		return engine.Vec2{}
	}
	var push engine.Vec2
	r := e.Bhv.Group.Spacing
	for _, o := range c.Squad.Members() {
		if o == e || !o.Alive() {
			continue
		}
		d := e.Pos.Sub(o.Pos)
		dist := d.Len()
		if dist == 0 {
			push = push.Add(engine.Vec2{X: 1})
			continue
		}
		if dist < r {
			push = push.Add(d.Normalized().Scale((r - dist) / r))
		}
	}
	return push.Scale(0.8)
}

// avoid — обход препятствия: если прямо по курсу стена, пробуем свернуть.
// Это не поиск пути, а объезд угла — на карте с длинной стеной враг всё равно
// упрётся, зато не залипает на каждом камне.
func (e *Enemy) avoid(c EnemyCtx, dir engine.Vec2) engine.Vec2 {
	if c.Field == nil {
		return engine.Vec2{}
	}
	ahead := e.Radius() + 14
	fits := func(v engine.Vec2) bool { return c.Field.Fits(e.Pos.Add(v.Scale(ahead)), e.Body()) }
	if fits(dir) {
		return engine.Vec2{}
	}
	left := engine.Vec2{X: -dir.Y, Y: dir.X}
	right := left.Scale(-1)
	switch {
	case fits(left) && !fits(right):
		return left
	case fits(right) && !fits(left):
		return right
	case fits(left) && fits(right):
		if e.circleCW {
			return left
		}
		return right
	}
	return engine.Vec2{}
}

// startAttack начинает замах.
func (e *Enemy) startAttack(c EnemyCtx) {
	e.state = EAttack
	e.vel = engine.Vec2{}
	e.faceTo(e.target(c))
	e.atkFace = e.faceVec()
	e.struck = false
	e.pending = nil

	name, clip := e.pick("attack", "walk", "idle")
	e.clip = "" // замах всегда с первого кадра
	e.play(name, clip)
	e.atkTicks = max(1, clip.Ticks())
	e.atkFrame = e.atkTicks / 2 / max(1, clip.FrameTicks)
	if clip.Valid() {
		e.atkFrame = len(clip.Frames) / 2
	}
}

func (e *Enemy) updateAttack(c EnemyCtx) {
	if !e.struck && e.player.Index() >= e.atkFrame {
		e.struck = true
		e.pending = &Hit{
			Center: e.Pos, Face: e.atkFace,
			Reach: e.Type.Threat.Reach + e.Radius(), Arc: enemyArc,
			Damage: e.Damage, Knockback: 30,
		}
	}
	if e.atkTicks--; e.atkTicks <= 0 {
		e.cooldown = max(20, e.Bhv.Combat.CommitTicks)
		e.release(c)
		e.commit = 0
		e.enter(ECircle) // после удара — в сторону, дать ударить другому
	}
}

// enter переводит в состояние и заряжает клип.
func (e *Enemy) enter(s EnemyState) {
	switch s {
	case EIdle, EDead:
		e.vel = engine.Vec2{}
	case EHurt:
		e.vel = engine.Vec2{}
		if !e.Pack.Has("hurt") {
			e.flash = enemyHurtTicks
		}
	}
	if s == EDead {
		e.pending, e.struck = nil, true
		e.token = false
		if !e.Pack.Has("death") {
			e.fade = enemyFadeTicks
		}
	}
	if s != EPatrol {
		e.hasGoal = false
	}
	e.state = s
	e.play(e.clipFor(s))
}

// clipFor — чем показать состояние. Деградация та же, что у зверей: набор
// анимаций рваный, а состояние от него не зависит.
func (e *Enemy) clipFor(s EnemyState) (string, *anim.Clip) {
	switch s {
	case EIdle:
		return e.pick("idle", "walk")
	case EPatrol, ESuspect, ESearch, EReturn:
		return e.pick("walk", "run", "idle")
	case EDodge:
		if n, cl := e.pick("run"); cl != nil {
			return n, cl
		}
		if _, cl := e.pick("walk"); cl != nil {
			return "dodge*", cl.Retimed(max(1, cl.FrameTicks/2))
		}
		return e.pick("idle")
	case EChase, ECircle, EKeep:
		if n, c := e.pick("run"); c != nil {
			return n, c
		}
		if _, c := e.pick("walk"); c != nil {
			return "run*", c.Retimed(max(1, c.FrameTicks/2))
		}
		return e.pick("idle")
	case EHurt:
		return e.pick("hurt", "idle", "walk")
	case EDead:
		return e.pick("death", "hurt", "idle")
	}
	return e.pick("idle", "walk")
}

func (e *Enemy) pick(names ...string) (string, *anim.Clip) {
	for _, n := range names {
		if c := e.Pack.Clip(n, e.dir); c != nil {
			return n, c
		}
	}
	return "", nil
}

func (e *Enemy) play(name string, c *anim.Clip) {
	if name == e.clip && e.clipDir == e.dir && c != nil {
		return
	}
	e.clip, e.clipDir = name, e.dir
	e.player.Play(c)
}

func (e *Enemy) faceTo(p engine.Vec2) {
	if d := p.Sub(e.Pos); d.Len() > 1 {
		e.dir = sprite.DirFrom(d)
	}
}

func (e *Enemy) faceVec() engine.Vec2 {
	switch e.dir {
	case sprite.Up:
		return engine.Vec2{X: 0, Y: -1}
	case sprite.Left:
		return engine.Vec2{X: -1, Y: 0}
	case sprite.Right:
		return engine.Vec2{X: 1, Y: 0}
	default:
		return engine.Vec2{X: 0, Y: 1}
	}
}

// step двигает врага. Скольжение вдоль стен и смену этажа берёт на себя поле;
// здесь остаётся заметить, что шаг не удался, и передумать.
func (e *Enemy) step(c EnemyCtx) {
	if e.vel.Len() == 0 {
		return
	}
	delta := e.vel.Scale(1.0 / config.TPS)
	if c.Field == nil {
		e.Pos = e.Pos.Add(delta)
		e.play(e.clipFor(e.state))
		return
	}
	delta = delta.Scale(c.Field.SpeedScale(e.Pos)) // мелководье вязкое
	before := e.Pos
	e.Pos, e.floor = c.Field.Move(e.Pos, delta, e.Body())
	if engine.Dist(before, e.Pos) < delta.Len()*0.25 {
		e.commit = 0 // упёрлись — решаем заново, а не давим в стену
	}
	e.play(e.clipFor(e.state))
}

func (e *Enemy) doneReacting() bool {
	if e.flash > 0 {
		e.flash--
		return e.flash == 0
	}
	return e.player.Finished() || !e.player.Clip().Valid()
}

func (e *Enemy) updateDead() {
	if e.fade > 0 {
		if e.fade--; e.fade == 0 {
			e.gone = true
		}
		return
	}
	if e.player.Finished() || !e.player.Clip().Valid() {
		e.fade = enemyFadeTicks
	}
}

// Draw рисует врага так, чтобы точка опоры легла в screen.
func (e *Enemy) Draw(dst *ebiten.Image, screen engine.Vec2) {
	img := e.player.Frame()
	if img == nil || e.gone {
		return
	}
	foot := e.Pack.Foot()
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(screen.X-float64(foot.X), screen.Y-float64(foot.Y))
	switch {
	case e.flash > 0:
		op.ColorScale.Scale(2, 2, 2, 1)
	case e.fade > 0:
		op.ColorScale.ScaleAlpha(float32(e.fade) / enemyFadeTicks)
	case e.Elite:
		op.ColorScale.Scale(1.25, 1.1, 0.9, 1) // элита заметно теплее обычной
	}
	dst.DrawImage(img, op)
}

// losClear — есть ли прямая видимость между точками. Взгляд останавливает
// только скала: вода, мелководье и обрыв не мешают смотреть, они мешают идти.
func losClear(f *physics.Field, from, to engine.Vec2) bool {
	if f == nil {
		return true
	}
	d := to.Sub(from)
	steps := int(d.Len() / f.Sub())
	if steps <= 1 {
		return true
	}
	for i := 1; i < steps; i++ {
		if f.CellAt(from.Add(d.Scale(float64(i)/float64(steps)))) == physics.Solid {
			return false
		}
	}
	return true
}
