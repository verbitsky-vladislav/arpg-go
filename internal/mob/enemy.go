package mob

import (
	"math"
	"math/rand/v2"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/vladislav/game/internal/anim"
	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/engine"
	"github.com/vladislav/game/internal/physics"
	"github.com/vladislav/game/internal/progress"
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

	// HPFrac — доля здоровья цели (1 — целая). Ниже порога врагам незачем
	// осторожничать: они перестают кружить и лезут добивать. Ноль означает
	// «неизвестно» и читается как «цела».
	HPFrac float64
}

// hpFrac — доля здоровья цели с поправкой на «неизвестно».
func (t Target) hpFrac() float64 {
	if t.HPFrac <= 0 {
		return 1
	}
	return t.HPFrac
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

// Пороги сторожа застревания, в тиках. Полсекунды на «может, дорога устарела»,
// секунда на «обойди», три с половиной на «уходи отсюда».
const (
	stuckStep     = 0.4 // меньше этого за тик — считай, стоишь
	stuckRepath   = 30
	stuckSidestep = 60
	stuckGiveUp   = 210
	unstickTicks  = 24
)

// cutoffLook — как далеко отрезающий ищет узкое место вокруг цели. Дальше
// половины экрана искать нечего: перекрытый выход должен быть виден игроку,
// иначе он не поймёт, почему его встретили.
const cutoffLook = 220

// searchSpread — на каком радиусе от последней точки расходится веер поиска.
const searchSpread = 90

// pathHold — сколько тиков жить найденному пути, прежде чем пересчитать.
// Полторы секунды: карта не меняется, а цель, к которой идут этим путём,
// стоит на месте по определению.
const pathHold = 90

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
	// XP — что отдаст эта особь: бросок по полосе тира, сделанный при рождении.
	// Один раз и навсегда, потому что число всплывает над трупом и оно же
	// уходит в полосу опыта героя — расходиться им нельзя.
	XP int
	// Level — уровень особи по её собственным числам, а не по табличным: у
	// элиты и здоровье, и урон выше, значит выше и уровень.
	Level int
	Elite bool

	// danger — место, которое особь занимает в бюджете спавнера. Не XP:
	// бросок у каждого свой, а население должно держаться вида, а не удачи.
	danger float64

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

	// Сторож застревания: враг, который хочет идти и не идёт, обязан это
	// заметить сам. Мир полон углов, и упереться в них не стыдно — стыдно
	// толкаться в них вечно.
	lastPos  engine.Vec2
	stuck    int
	unstick  int
	sidestep engine.Vec2

	// Рывок за отрывающейся целью.
	dash   int
	dashCD int

	// Оценка скорости цели: по ней бьют с упреждением. Считаем сами, чтобы не
	// требовать её от игрового слоя.
	tgtSeen engine.Vec2
	tgtVel  engine.Vec2
	hasSeen bool

	wait    int // стоять столько тиков (патруль)
	goal    engine.Vec2
	hasGoal bool

	// Личный путь: к последней известной точке, домой, к своему месту в круге.
	// Общей волны для этого нет — цели у всех разные.
	path     []engine.Vec2
	pathAt   int
	pathTo   engine.Vec2
	pathLeft int

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
		HP: t.HP, MaxHP: t.HP, Damage: t.Damage,
		Level: t.Level(), danger: t.XP.Mid(),
		player: anim.NewPlayer(nil),
		slot:   -1,
		rng:    rng,
	}
	e.flanker = rng.Float64() < bhv.Group.Flank
	e.circleCW = rng.IntN(2) == 0
	e.XP = t.XP.Pick(XPSeed(t.Type.ID+"_"+t.ID, pos))
	e.enter(EIdle)
	return e
}

// MakeElite усиливает особь по правилам спавна. Уровень пересчитывается по
// новым числам: элита действительно опаснее ровесника, и герой, переросший
// обычного гоблина, за элитного всё ещё получит опыт.
func (e *Enemy) MakeElite(hpScale, dmgScale, xpScale float64) {
	e.Elite = true
	e.MaxHP = max(1, int(float64(e.MaxHP)*hpScale))
	e.HP = e.MaxHP
	e.Damage = max(1, int(float64(e.Damage)*dmgScale))
	e.XP = int(float64(e.XP) * xpScale)
	e.Level = progress.MobLevel(e.MaxHP, e.Damage)
	e.danger *= xpScale
}

// Danger — место особи в бюджете спавнера.
func (e *Enemy) Danger() float64 { return e.danger }

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
	if e.dash > 0 && e.Bhv.Combat.DashScale > 1 {
		v *= e.Bhv.Combat.DashScale
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
	e.search = e.Bhv.Perception.SearchTicks
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
	dec(&e.dash)
	dec(&e.dashCD)
	dec(&e.unstick)
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
		// Скорость цели считаем сами: игровому слою незачем её объявлять, а
		// упреждение без неё невозможно.
		if e.hasSeen {
			e.tgtVel = c.Player.Pos.Sub(e.tgtSeen).Scale(config.TPS)
		}
		e.tgtSeen, e.hasSeen = c.Player.Pos, true
		if c.Squad != nil {
			c.Squad.DropSearch(e) // нашёл — сектор поиска больше не нужен
		}
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
		if nd, ok := c.Nav.Dist(e.Pos, e.floor, 0); ok {
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

	// Застрял намертво — бросить цель и уйти домой. Дорога отсюда есть всегда:
	// пришёл же он как-то сюда.
	if e.stuck > stuckGiveUp {
		e.stuck = 0
		e.knows = false
		e.dropPath()
		e.enter(EReturn)
		return
	}
	// Цель недостижима для тела такого размера (узкий мост, проход в скале,
	// макушка плато без лестницы). Идти напрямик в этом случае — это и есть
	// «тупит у обрыва»: надо либо бить с дистанции, либо разворачиваться.
	if e.knows && e.unreachable(c) {
		if e.Bhv.Combat.PreferRange > 0 && e.seen &&
			engine.Dist(e.Pos, e.target(c)) <= e.Bhv.Combat.PreferRange+e.Bhv.Combat.KeepBand {
			e.enter(EKeep)
			return
		}
		e.enter(EReturn)
		return
	}

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

	// Видим цель (или только что видели) — деремся.
	if e.knows && e.react == 0 && (e.seen || e.lose > 0) {
		e.engage(c)
		return
	}
	// Не видим, но помним где — ищем. Раньше здесь стояло «помню → дерусь с
	// точкой», и поиск не включался почти никогда: память живёт дольше, чем
	// он, и группа просто стояла в последней точке, глядя в пустоту.
	if e.knows && (e.memory > 0 || e.search > 0) {
		e.enter(ESearch)
		return
	}
	if e.suspect > 0 {
		e.enter(ESuspect)
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

// unreachable — есть ли вообще дорога до цели для этого тела. Летающих не
// касается: им карта не указ.
func (e *Enemy) unreachable(c EnemyCtx) bool {
	if e.flies() || c.Nav == nil || !c.HasPlayer {
		return false
	}
	if engine.Dist(c.Nav.Goal(), e.target(c)) > 120 {
		return false // волна не про эту точку — судить не по чему
	}
	_, ok := c.Nav.Dist(e.Pos, e.floor, e.Radius())
	return !ok
}

// watchStuck следит, двигается ли враг, когда хочет двигаться.
//
// Три ступени: сбросить путь (может, дорога устарела), шагнуть вбок (обойти
// то, во что упёрлись), сдаться и уйти домой (решает decide). Без этого враг
// толкается в дерево бесконечно: обстановка не меняется, значит и решение
// каждый раз то же самое.
func (e *Enemy) watchStuck(c EnemyCtx) {
	moved := engine.Dist(e.Pos, e.lastPos)
	e.lastPos = e.Pos
	if e.vel.Len() == 0 || moved > stuckStep {
		e.stuck = 0
		return
	}
	e.stuck++
	switch e.stuck {
	case stuckRepath:
		e.dropPath()
		e.commit = 0
	case stuckSidestep:
		// Вбок и чуть назад: строго вбок можно упереться в ту же стену.
		side := engine.Vec2{X: -e.vel.Y, Y: e.vel.X}.Normalized()
		if e.rng.IntN(2) == 0 {
			side = side.Scale(-1)
		}
		e.sidestep = side.Sub(e.vel.Normalized().Scale(0.3)).Normalized()
		e.unstick = unstickTicks
	}
}

// engage выбирает боевое состояние: бить, держать дистанцию, кружить или идти.
func (e *Enemy) engage(c EnemyCtx) {
	dist := engine.Dist(e.Pos, e.target(c))
	reach := e.reach(c)
	cb := e.Bhv.Combat
	// Раненая цель снимает осторожность: не кружим, лезем и бьём.
	finish := c.HasPlayer && c.Player.hpFrac() < cb.FinishHP

	// Стрелок: подошли ближе полосы — пятится, дальше полосы — подходит.
	if cb.PreferRange > 0 {
		// Бить выгоднее, когда цель связана ближним боем: пока свои держат её,
		// стрелок работает спокойно. Свободную цель он предпочтёт сначала
		// отодвинуть — иначе она просто дойдёт до него.
		held := c.Squad != nil && c.Squad.MeleePressure(e.target(c)) > 0
		switch {
		case dist < cb.PreferRange-cb.KeepBand, dist > cb.PreferRange+cb.KeepBand:
			if held && e.seen && e.cooldown == 0 && dist <= cb.PreferRange*1.4 && e.claim(c, finish) {
				e.startAttack(c) // держат — можно и с неудобной дистанции
				return
			}
			e.enter(EKeep)
			return
		case e.seen && e.cooldown == 0 && e.claim(c, finish):
			e.startAttack(c)
			return
		default:
			e.enter(ECircle)
			return
		}
	}

	if dist <= reach {
		// Вплотную, но бить можно не всем сразу: очередь держит отряд.
		if e.cooldown == 0 && e.claim(c, finish) {
			e.startAttack(c)
			return
		}
		e.enter(ECircle)
		return
	}
	// Цель отрывается — рывок. Иначе от быстрого врага можно просто уходить
	// спиной, изредка отмахиваясь.
	if e.tryDash(c, dist) {
		return
	}
	if !finish && dist <= reach*2.2 && e.rng.Float64() < cb.Strafe {
		e.enter(ECircle)
		return
	}
	e.enter(EChase)
}

// tryDash — короткое ускорение за целью, которая разрывает дистанцию.
func (e *Enemy) tryDash(c EnemyCtx, dist float64) bool {
	cb := e.Bhv.Combat
	if cb.Dash <= 0 || e.dash > 0 || e.dashCD > 0 || !e.seen {
		return false
	}
	if dist < e.reach(c)*1.6 || dist > e.Type.Threat.Sight*0.8 {
		return false
	}
	// Рывок нужен вдогонку, а не навстречу: если цель идёт на нас, она и так
	// придёт.
	if away := e.target(c).Sub(e.Pos).Normalized().Dot(e.tgtVel.Normalized()); away < 0.2 {
		return false
	}
	if e.rng.Float64() >= cb.Dash {
		return false
	}
	e.dash = max(1, cb.DashTicks)
	e.dashCD = e.dash + cb.CommitTicks*2
	e.enter(EChase)
	return true
}

// claim просит у отряда право ударить. Без отряда бьёт кто хочет.
func (e *Enemy) claim(c EnemyCtx, urgent bool) bool {
	if e.token {
		return true
	}
	if c.Squad == nil {
		e.token = true
		return true
	}
	e.token = c.Squad.ClaimAttack(e, urgent)
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
		// Засадник не бродит: он ждёт. Пока цель не замечена, он часть
		// пейзажа — и в этом весь смысл засады.
		if e.Bhv.Combat.Ambush {
			e.vel = engine.Vec2{}
			break
		}
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
	// Обход того, во что упёрлись, важнее выбранного направления: пока он идёт,
	// враг движется вбок, а не давит в стену.
	if e.unstick > 0 && e.vel.Len() > 0 {
		e.vel = e.sidestep.Scale(e.Speed())
	}
	e.step(c)
	e.watchStuck(c)
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
			// Отрезающий встаёт на пути отхода — с той стороны, куда цель
			// побежит, то есть напротив того места, откуда пришла группа.
			away := t.Sub(c.Squad.Center()).Normalized()
			if away.Len() == 0 {
				away = e.faceVec().Scale(-1)
			}
			// В коридоре перекрывать надо дверь, а не пятачок рядом с целью:
			// точку у выхода обойти нельзя, а точку в чистом поле — можно.
			if p, ok := c.Nav.Choke(t, away, e.floor, e.Radius(), cutoffLook); ok {
				return p
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
	e.vel = tangent.Add(radial).Add(e.crowd(c)).Normalized().Scale(e.Speed() * 0.75)
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

// searchAround — обыск местности у последней известной точки.
//
// Идут не в саму точку, а каждый в свой сектор вокруг неё: втроём топтаться в
// одном месте бессмысленно, а веером они накрывают площадь, и спрятаться за
// углом становится трудно.
func (e *Enemy) searchAround(c EnemyCtx) {
	spot := e.lastSeen
	if c.Squad != nil {
		const sectors = 6
		i := c.Squad.SearchSlot(e, sectors)
		ang := 2 * math.Pi * float64(i) / sectors
		r := searchSpread * (0.6 + 0.4*float64(i%2))
		spot = e.lastSeen.Add(engine.Vec2{X: math.Cos(ang) * r, Y: math.Sin(ang) * r})
	}
	if engine.Dist(e.Pos, spot) > 20 {
		e.moveTo(c, spot, 1)
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
	if !e.flies() && c.Nav != nil {
		switch {
		// Идём к игроку — берём общую волну: она уже посчитана на всех, и
		// полоса выбирается под ширину этого тела. Порог щедрый: точка подхода
		// у фланкёра и отрезающего смещена на корпус-другой, дорога та же.
		case engine.Dist(c.Nav.Goal(), to) < 120:
			if nd, ok := c.Nav.Step(e.Pos, e.floor, e.Radius()); ok {
				dir = nd
			}
		// Идём куда-то ещё (последняя точка, дом) — считаем свой путь. Волна
		// от игрока тут не помощник: она ведёт не туда.
		case d.Len() > 64:
			if nd, ok := e.follow(c, to); ok {
				dir = nd
			}
		}
	}
	if e.waitGap(c, dir) {
		e.vel = engine.Vec2{} // впереди узкое место, и оно занято своим
		return
	}
	dir = dir.Add(e.crowd(c)).Add(e.avoid(c, dir))
	if dir.Len() == 0 {
		dir = d.Normalized()
	}
	e.vel = dir.Normalized().Scale(e.Speed() * scale)
	e.faceTo(to)
}

// follow ведёт по личному пути к точке to. Путь считается редко: карта не
// меняется, а цель стоит на месте — пересчитывать его каждый тик значило бы
// гонять A* на всю толпу без нужды.
func (e *Enemy) follow(c EnemyCtx, to engine.Vec2) (engine.Vec2, bool) {
	if e.pathLeft--; e.pathLeft <= 0 || len(e.path) == 0 || engine.Dist(e.pathTo, to) > 48 {
		e.path = c.Nav.Path(e.Pos, to, e.floor, e.Radius())
		e.pathAt, e.pathTo, e.pathLeft = 0, to, pathHold
	}
	// Пройденные точки снимаются: иначе враг возвращается к первой из них.
	for e.pathAt < len(e.path) && engine.Dist(e.Pos, e.path[e.pathAt]) < 20 {
		e.pathAt++
	}
	if e.pathAt >= len(e.path) {
		return engine.Vec2{}, false
	}
	return e.path[e.pathAt].Sub(e.Pos).Normalized(), true
}

// dropPath забывает найденную дорогу: цель сменилась, идти по старой незачем.
func (e *Enemy) dropPath() { e.path, e.pathAt, e.pathLeft = nil, 0, 0 }

// waitGap пропускает своего через узкое место вперёд себя.
//
// Мост в один тайл, дверь, тропа между скалами: расталкивание там работает
// против всех — двое выпихивают друг друга в воду. Проходит тот, кто занял
// место первым, остальные ждут в шаге позади.
func (e *Enemy) waitGap(c EnemyCtx, dir engine.Vec2) bool {
	if c.Nav == nil || c.Squad == nil || e.unstick > 0 {
		return false
	}
	ahead := e.Pos.Add(dir.Scale(e.Radius() + c.Nav.CellSize()))
	if c.Nav.Width(ahead, dir, e.floor, e.Radius()) > 2*e.Radius()*2 {
		c.Squad.LeaveGap(e) // впереди простор — держать место незачем
		return false
	}
	return !c.Squad.TakeGap(e, gapKey(c.Nav, ahead))
}

// gapKey — клетка навигации как ключ: соседние точки внутри одной клетки
// должны считаться одним и тем же проходом.
func gapKey(n *NavField, p engine.Vec2) int64 {
	s := n.CellSize()
	return int64(math.Floor(p.X/s))<<32 | int64(int32(math.Floor(p.Y/s)))
}

// crowd — всё, что толкает врага в сторону от своих: расталкивание тел и уход
// с линии чужого удара.
func (e *Enemy) crowd(c EnemyCtx) engine.Vec2 {
	return e.separation(c).Add(e.clearOfAllies(c))
}

// clearOfAllies уводит из сектора замаха своего же соседа.
//
// Очередь удара разводит атаки по времени, но не по месту: пока один бьёт,
// второй спокойно стоит там, куда придётся удар. На экране это читается как
// «они бьют друг друга», даже если урона по своим нет. Уходить надо вбок —
// назад из сектора не выйти.
func (e *Enemy) clearOfAllies(c EnemyCtx) engine.Vec2 {
	if c.Squad == nil {
		return engine.Vec2{}
	}
	var push engine.Vec2
	for _, o := range c.Squad.Members() {
		if o == e || !o.Alive() || o.state != EAttack {
			continue
		}
		d := e.Pos.Sub(o.Pos)
		dist := d.Len()
		reach := o.Type.Threat.Reach + o.Radius() + e.Radius()
		if dist == 0 || dist > reach {
			continue
		}
		if d.Normalized().Dot(o.atkFace) < math.Cos(enemyArc/2*math.Pi/180) {
			continue // не на линии удара
		}
		side := engine.Vec2{X: -o.atkFace.Y, Y: o.atkFace.X}
		if side.Dot(d) < 0 {
			side = side.Scale(-1) // выходим в ту сторону, к которой уже ближе
		}
		push = push.Add(side.Scale(1.2))
	}
	return push
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
	// Бьём туда, где цель окажется к кадру попадания, а не туда, где она
	// сейчас: по бегущему иначе не попасть никогда.
	e.faceTo(e.aimPoint(c))
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

// aimPoint — точка прицеливания с упреждением: цель плюс её скорость за время
// до попадания, взвешенное профилем (lead).
//
// Без этого враг всегда бьёт туда, где цель была в начале замаха, и по бегущему
// не попадает никогда — а игрок быстро понимает, что достаточно не стоять.
func (e *Enemy) aimPoint(c EnemyCtx) engine.Vec2 {
	t := e.target(c)
	lead := e.Bhv.Combat.Lead
	if lead <= 0 || !e.seen || e.tgtVel.Len() == 0 {
		return t
	}
	// Время до попадания: половина выдержки решения — достаточная оценка,
	// кадр удара обычно приходится примерно туда.
	dt := float64(max(1, e.Bhv.Combat.CommitTicks)) / 2 / config.TPS
	return t.Add(e.tgtVel.Scale(dt * lead))
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
	// Смена занятия — смена цели: старая дорога больше не туда.
	switch s {
	case EIdle, EPatrol, EAttack, EHurt, EDead:
		e.dropPath()
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

func (e *Enemy) faceVec() engine.Vec2 { return e.dir.Vec() }

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
	// Не помещаемся там, где стоим, — этаж разошёлся с местом (толкнули у
	// обрыва, сошли с лестницы боком). Перечитать его дешевле, чем всю дорогу
	// выдавливаться из стены по пикселю.
	if !c.Field.Fits(e.Pos, e.Body()) {
		e.Land(c.Field)
	}
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
