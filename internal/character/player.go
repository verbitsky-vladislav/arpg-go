package character

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/vladislav/game/internal/anim"
	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/engine"
	"github.com/vladislav/game/internal/physics"
	"github.com/vladislav/game/internal/sprite"
)

// State — что персонаж делает прямо сейчас.
type State uint8

const (
	Idle      State = iota // стоит
	Walk                   // идёт
	Run                    // бежит
	Attacking              // бьёт (стоя или на ходу)
	Hurt                   // получил удар, оцепенел
	Dead                   // умер
)

var stateNames = [...]string{"idle", "walk", "run", "attack", "hurt", "dead"}

func (s State) String() string {
	if int(s) >= len(stateNames) {
		return "?"
	}
	return stateNames[s]
}

// placeSearch — в каком радиусе искать место под героя, если в заданную точку
// он не помещается (появление, воскрешение). Полтора экрана: дальше уносить
// игрока от того места, куда его звали, уже нельзя.
const placeSearch = 256

// playerCaps — что герой умеет в смысле физики: заходит в мелководье (медленно),
// но не плавает — клипа swim в паках нет, и по глубокой воде он бы «шёл стоя».
var playerCaps = physics.Caps{Wade: true}

// Input — намерения игрока за один тик. Персонаж не знает про клавиши и мышь:
// раскладку держит тот, кто его обновляет (сцена, просмотрщик, тест). Ровно та
// же роль, что у mob.Ctx для животного, только источник — человек.
type Input struct {
	// Move — направление движения; длина сверх 1 обрезается (геймпад/клавиши).
	Move engine.Vec2
	// Run — просьба бежать (Shift). Без Move ничего не значит.
	Run bool
	// Attack — удар нажат в этом тике (фронт нажатия, не удержание).
	Attack bool
	// Aim/HasAim — куда смотреть (мировая точка под курсором). Без прицела
	// персонаж смотрит туда, куда идёт.
	Aim    engine.Vec2
	HasAim bool
}

// Hit — состоявшийся удар: сектор перед персонажем в момент кадра попадания.
// Кого он задел, решает игровой слой (у персонажа нет списка целей), поэтому
// Hit — это описание области, а не событие с уже выбранной жертвой.
type Hit struct {
	Center    engine.Vec2 // откуда бьют (позиция персонажа)
	Face      engine.Vec2 // единичный вектор направления удара
	Reach     float64     // радиус сектора
	Arc       float64     // раствор сектора, градусы
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
	// Цель вплотную (в том числе точно под ногами) считается задетой всегда:
	// направление на неё вырождается, и проверять угол не по чему.
	if dist <= radius {
		return true
	}
	cos := d.Normalized().Dot(h.Face)
	return cos >= math.Cos(h.Arc/2*math.Pi/180)
}

// Player — персонаж в мире: пара «тело × лоадаут» плюс автомат состояний.
//
// Как и у животных, намерение отделено от исполнения: состояние выбирается по
// вводу и никогда не зависит от того, есть ли под него анимация. Расхождение
// закрывает clipFor — у лоадаута unarmed нет ни attack, ни walk_attack, но
// ударить им можно (см. docs/character/character.md, §деградация).
type Player struct {
	Cat     *Catalog
	Body    *Body
	Loadout *Loadout
	// Stolen — украденная у врага сила: ею бьют, когда оружия нет. Заглушка
	// под будущий механизм воровства (у врагов в enemies.json уже описаны
	// power/steal_chance/charges), выдавать её пока некому. Своих клипов удара
	// у неё не будет — attackClip в этом случае бьёт из стойки, а замах
	// рисует игровой слой.
	Stolen *Loadout
	Pack   *sprite.Pack
	Pos    engine.Vec2
	HP     int
	MaxHP  int

	vel     engine.Vec2
	floor   uint8 // этаж: 0 — низ, 1 — макушка плато (physics.Floor*)
	dir     sprite.Dir
	state   State
	clip    string     // имя проигрываемого клипа ("" — клипа нет)
	clipDir sprite.Dir // направление, для которого клип был заряжен
	player  *anim.Player

	atk       Attack      // копия удара лоадаута на время замаха
	atkTicks  int         // тиков до конца замаха
	atkFrame  int         // кадр, на котором наносится урон
	atkFace   engine.Vec2 // направление удара, зафиксированное в начале замаха
	atkMoving bool        // замах на ходу (walk_attack/run_attack)
	struck    bool        // урон этого замаха уже выдан
	pending   *Hit        // удар ждёт, пока его заберёт игровой слой

	cooldown int // тиков до следующего удара
	invuln   int // тиков неуязвимости
	lock     int // тиков оцепенения после удара
	fade     int // остаток растворения трупа
	gone     bool
}

// NewPlayer создаёт персонажа тела b с лоадаутом l в точке pos.
func NewPlayer(c *Catalog, b *Body, l *Loadout, pack *sprite.Pack, pos engine.Vec2) *Player {
	p := &Player{
		Cat: c, Body: b, Loadout: l, Pack: pack, Pos: pos,
		player: anim.NewPlayer(nil),
	}
	p.MaxHP = max(1, int(float64(c.Base.HP)*b.HPScale))
	p.HP = p.MaxHP
	p.enter(Idle)
	return p
}

// Equip меняет лоадаут (оружие) на ходу, сохраняя позицию, здоровье и
// направление. Замах прерывается: клипы у нового лоадаута другие, и доигрывать
// удар мечом кулаками было бы враньём.
func (p *Player) Equip(l *Loadout, pack *sprite.Pack) {
	p.Loadout, p.Pack = l, pack
	p.pending, p.struck = nil, true
	if p.state == Attacking {
		p.enter(Idle)
		return
	}
	p.clip = "" // пак сменился — перезарядить клип даже под тем же именем
	p.play(p.clipFor(p.state))
}

// SetBody меняет тело (внешность), пересчитывая здоровье в той же доле.
func (p *Player) SetBody(b *Body, pack *sprite.Pack) {
	frac := float64(p.HP) / float64(p.MaxHP)
	p.Body, p.Pack = b, pack
	p.MaxHP = max(1, int(float64(p.Cat.Base.HP)*b.HPScale))
	p.HP = max(1, int(float64(p.MaxHP)*frac))
	p.clip = ""
	p.play(p.clipFor(p.state))
}

// Alive — жив ли персонаж.
func (p *Player) Alive() bool { return p.state != Dead }

// Gone — смерть доиграна и труп растаял.
func (p *Player) Gone() bool { return p.gone }

// State — текущее состояние.
func (p *Player) State() State { return p.state }

// Clip — имя проигрываемого клипа (для отладки и просмотрщика).
func (p *Player) Clip() string { return p.clip }

// Frame — текущий кадр (nil, если рисовать нечего).
func (p *Player) Frame() *ebiten.Image { return p.player.Frame() }

// Dir — направление взгляда.
func (p *Player) Dir() sprite.Dir { return p.dir }

// Radius — радиус тела в мировых пикселях. Берётся из данных, а не из рамки
// кадра: у пака с мечом рамка включает вылет клинка.
func (p *Player) Radius() float64 { return p.Cat.Base.BodyRadius }

// Invulnerable — идут ли кадры неуязвимости после полученного удара.
func (p *Player) Invulnerable() bool { return p.invuln > 0 }

// Armed — есть ли чем бить вообще: оружие в руках или украденная у врага сила.
// Голыми руками герой не дерётся, поэтому правило одно на всю игру и лежит
// здесь, а не в сцене.
func (p *Player) Armed() bool {
	return p.Loadout.CanStrike() || p.Stolen.CanStrike()
}

// striker — чем бьём: оружием, а если его нет — украденной силой.
func (p *Player) striker() *Loadout {
	if !p.Loadout.CanStrike() && p.Stolen.CanStrike() {
		return p.Stolen
	}
	return p.Loadout
}

// CanAttack — готов ли удар: есть чем бить, перезарядка вышла, персонаж не
// занят. Без оружия и без украденной силы удара нет — ни замаха, ни сектора.
func (p *Player) CanAttack() bool {
	return p.Armed() &&
		p.Alive() && p.cooldown == 0 && p.state != Hurt && p.state != Attacking
}

// Speed — скорость персонажа в текущем состоянии, px/с.
func (p *Player) Speed(run bool) float64 {
	s := p.Cat.Base.Speed.Walk
	if run {
		s = p.Cat.Base.Speed.Run
	}
	return s * p.Body.SpeedScale * p.Loadout.SpeedScale
}

// Strike забирает состоявшийся удар: непустой ровно один раз за замах, на
// кадре попадания. Игровой слой обходит им своих целей (животных, врагов).
func (p *Player) Strike() (Hit, bool) {
	if p.pending == nil {
		return Hit{}, false
	}
	h := *p.pending
	p.pending = nil
	return h, true
}

// Damage наносит персонажу урон от точки from. Возвращает true, если удар
// добил. Во время неуязвимости урон не проходит.
func (p *Player) Damage(dmg int, from engine.Vec2) bool {
	if p.state == Dead || p.invuln > 0 || dmg <= 0 {
		return false
	}
	p.HP -= dmg
	if p.HP <= 0 {
		p.HP = 0
		p.enter(Dead)
		return true
	}
	p.knock(from)
	p.enter(Hurt)
	return false
}

// Kill убивает персонажа мимо неуязвимости (отладка, скрипты, обрывы).
func (p *Player) Kill() {
	if p.state == Dead {
		return
	}
	p.HP = 0
	p.enter(Dead)
}

// Revive поднимает персонажа с полным здоровьем в точке pos.
func (p *Player) Revive(pos engine.Vec2) {
	p.HP, p.Pos = p.MaxHP, pos
	p.floor = physics.FloorLow // воскрешают на точке старта, а она внизу
	p.gone, p.fade, p.lock, p.invuln, p.cooldown = false, 0, 0, 0, 0
	p.pending, p.vel = nil, engine.Vec2{}
	p.clip = ""
	p.enter(Idle)
}

// Update продвигает персонажа на один тик логики.
func (p *Player) Update(in Input, f *physics.Field) {
	if p.gone {
		return
	}
	p.player.Update()
	if p.cooldown > 0 {
		p.cooldown--
	}
	if p.invuln > 0 {
		p.invuln--
	}

	switch p.state {
	case Dead:
		p.updateDead()
		return
	case Hurt:
		p.updateHurt(f)
		return
	case Attacking:
		p.updateAttack(in, f)
		return
	}

	p.face(in)
	if in.Attack && p.CanAttack() {
		p.startAttack(in)
		return
	}
	p.move(in, f)
}

// move — обычное перемещение по вводу; заодно выбирает между idle/walk/run.
func (p *Player) move(in Input, f *physics.Field) {
	dir := clampLen(in.Move)
	if dir.Len() == 0 {
		p.vel = engine.Vec2{}
		p.enter(Idle)
		return
	}
	run := in.Run
	p.vel = dir.Scale(p.Speed(run))
	if run {
		p.enter(Run)
	} else {
		p.enter(Walk)
	}
	p.step(f, 1)
}

// updateAttack ведёт замах: урон на своём кадре, движение — только если
// лоадаут умеет бить на ходу.
func (p *Player) updateAttack(in Input, f *physics.Field) {
	if !p.struck && p.player.Index() >= p.atkFrame {
		p.struck = true
		p.pending = &Hit{
			Center:    p.Pos,
			Face:      p.atkFace,
			Reach:     p.atk.Reach + p.Radius(),
			Arc:       p.atk.Arc,
			Damage:    p.atk.Damage,
			Knockback: p.atk.Knockback,
		}
	}
	if p.atkMoving {
		if d := clampLen(in.Move); d.Len() > 0 {
			p.vel = d.Scale(p.Speed(p.clip == "run_attack"))
			p.step(f, p.Cat.Base.AttackMoveScale)
		}
	}
	if p.atkTicks--; p.atkTicks <= 0 {
		p.enter(Idle) // следующий тик разберётся по вводу
	}
}

// updateHurt отыгрывает оцепенение: персонаж не слушается ввода, но отброс
// доезжает, поэтому удар видно, а не только слышно.
func (p *Player) updateHurt(f *physics.Field) {
	if p.vel.Len() > 0 {
		p.step(f, 1)
		p.vel = p.vel.Scale(0.85) // отброс гаснет
	}
	if p.lock--; p.lock <= 0 {
		p.vel = engine.Vec2{}
		p.enter(Idle)
	}
}

func (p *Player) updateDead() {
	if p.fade > 0 {
		if p.fade--; p.fade == 0 {
			p.gone = true
		}
		return
	}
	if p.player.Finished() || !p.player.Clip().Valid() {
		p.fade = p.Cat.Base.Death.FadeTicks // клип смерти доигран — труп тает
	}
}

// startAttack начинает замах: клип выбирается по тому, как персонаж двигался.
func (p *Player) startAttack(in Input) {
	p.atk = p.striker().Attack
	p.atkFace = p.faceVec()
	p.cooldown = p.atk.CooldownTicks
	p.struck = false
	p.pending = nil

	moving := clampLen(in.Move).Len() > 0 && p.atk.OnMove
	name, clip := p.attackClip(moving, in.Run)
	clip = p.swing(clip)
	p.atkMoving = moving && (name == "walk_attack" || name == "run_attack")
	p.state = Attacking
	p.clip = "" // замах перезапускается с первого кадра, даже если клип тот же
	p.play(name, clip)

	p.atkTicks = max(1, clip.Ticks())
	p.atkFrame = p.hitFrame(name, clip)
	if !p.atkMoving {
		p.vel = engine.Vec2{}
	}
}

// attackClip выбирает клип замаха с деградацией: чего нет в паке, тем и не
// бьём — но ударить можно всегда.
//
//	стоя       attack       → walk_attack → стойка (idle)
//	шагом      walk_attack  → attack      → стойка
//	бегом      run_attack   → walk_attack → attack → стойка
func (p *Player) attackClip(moving, run bool) (string, *anim.Clip) {
	var order []string
	switch {
	case moving && run:
		order = []string{"run_attack", "walk_attack", "attack"}
	case moving:
		order = []string{"walk_attack", "attack"}
	default:
		order = []string{"attack", "walk_attack"}
	}
	if n, c := p.pick(order...); c != nil {
		return n, c
	}
	// Клипа удара нет вовсе (unarmed): бьём из стойки. Походка тут не годится —
	// ускоренный run читается как «герой побежал на месте», а не как удар; сам
	// замах показывает игровой слой (след сектора). Имя нарочно не совпадает с
	// исходным: play() сравнивает имена, и без «звёздочки» переход «стою» →
	// «бью» на том же idle не перезапустил бы клип.
	if _, c := p.pick("idle", "walk", "run"); c != nil {
		return "attack*", c
	}
	return "", nil
}

// swing подгоняет клип замаха под attack.swing_ticks: длительность боевого
// действия — это баланс, а не свойство листа. fps в манифесте общий на весь
// пак, и по нему взмах мечом тянулся бы больше секунды.
//
// Длительность округляется вниз до целого числа тиков на кадр, поэтому реальный
// замах чуть короче заказанного (32 тика на 6 кадров — это 5×6=30).
func (p *Player) swing(c *anim.Clip) *anim.Clip {
	if !c.Valid() || p.atk.SwingTicks <= 0 {
		return c
	}
	return c.Retimed(p.atk.SwingTicks / len(c.Frames))
}

// hitFrame — кадр, на котором замах наносит урон. Для клипа без записи в
// hit_at (в том числе для подменыша attack*) — середина клипа.
func (p *Player) hitFrame(name string, c *anim.Clip) int {
	if f, ok := p.atk.HitAt[name]; ok && c.Valid() && f < len(c.Frames) {
		return f
	}
	if !c.Valid() {
		return 0
	}
	return len(c.Frames) / 2
}

// face поворачивает персонажа: на прицел, если он есть, иначе по движению.
func (p *Player) face(in Input) {
	switch {
	case in.HasAim:
		if d := in.Aim.Sub(p.Pos); d.Len() > 1 {
			p.dir = sprite.DirFrom(d)
		}
	case in.Move.Len() > 0:
		p.dir = sprite.DirFrom(in.Move)
	}
}

// FaceVec — направление взгляда единичным вектором. Наружу оно нужно тем, кто
// показывает намерение героя: враг видит занесённое оружие и решает, уходить ли
// с линии удара.
func (p *Player) FaceVec() engine.Vec2 { return p.faceVec() }

// faceVec — направление взгляда единичным вектором (для сектора удара).
func (p *Player) faceVec() engine.Vec2 {
	switch p.dir {
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

// knock задаёт отброс от точки from.
func (p *Player) knock(from engine.Vec2) {
	d := p.Pos.Sub(from).Normalized()
	if d.Len() == 0 {
		d = p.faceVec().Scale(-1)
	}
	p.vel = d.Scale(p.Cat.Base.Hurt.Knockback)
}

// step двигает персонажа на долю scale от скорости. Стены, скольжение вдоль
// них и этажи держит поле (internal/physics) — здесь только скорость.
//
// Мелководье вязкое: по нему герой бредёт медленнее. Замедление применяется к
// шагу, а не к Speed(), потому что оно свойство места, а не состояния, — и
// отброс от удара в воде тоже гаснет быстрее.
func (p *Player) step(f *physics.Field, scale float64) {
	if p.vel.Len() == 0 {
		return
	}
	d := p.vel.Scale(scale * f.SpeedScale(p.Pos) / config.TPS)
	p.Pos, p.floor = f.Move(p.Pos, d, p.body())
}

// body — тело персонажа для поля: круг радиуса Radius на своём этаже.
func (p *Player) body() physics.Body {
	return physics.Body{Radius: p.Radius(), Floor: p.floor, Caps: playerCaps}
}

// Push сдвигает персонажа на delta с проверкой стен: этим игровой слой
// расталкивает тела, которые налезли друг на друга.
func (p *Player) Push(f *physics.Field, delta engine.Vec2) {
	p.Pos, p.floor = f.Move(p.Pos, delta, p.body())
}

// Floor — этаж, на котором стоит персонаж (0 — низ, 1 — макушка плато).
func (p *Player) Floor() uint8 { return p.floor }

// PlaceOn ставит персонажа в точку pos, считая, что он стоит на этаже floor.
// Этаж — часть места: одна и та же точка на макушке плато и под обрывом
// проходима для разных этажей, и поставить туда персонажа, не сказав, на каком
// он из них, значит промахнуться мимо (см. Place — оно решает, поместился ли он,
// телом текущего этажа). Нужно тому, кто возвращает персонажа в сохранённую
// точку: этаж он тоже помнит.
func (p *Player) PlaceOn(f *physics.Field, pos engine.Vec2, floor uint8) {
	p.floor = floor
	p.Place(f, pos)
}

// Place ставит персонажа в точку pos, поправив её так, чтобы он туда поместился
// (появление, воскрешение). Этаж берётся из клетки, куда он встал.
func (p *Player) Place(f *physics.Field, pos engine.Vec2) {
	if q, ok := f.Place(pos, p.body(), placeSearch); ok {
		pos = q
	}
	p.Pos = pos
	p.floor = f.CellAt(pos).Floor()
}

// enter переводит в состояние s и заряжает подходящий клип. Замах и смерть
// заряжаются не здесь (у них свои клипы и таймеры), см. startAttack.
func (p *Player) enter(s State) {
	switch s {
	case Idle:
		p.vel = engine.Vec2{}
	case Hurt:
		p.lock = p.Cat.Base.Hurt.LockTicks
		p.invuln = p.Cat.Base.Hurt.InvulnTicks
		p.pending, p.struck = nil, true // замах сбит ударом
	case Dead:
		p.vel = engine.Vec2{}
		p.pending, p.struck = nil, true
		p.fade = 0
	}
	p.state = s
	p.play(p.clipFor(s))
}

// clipFor — чем показать состояние s. Здесь живёт деградация набора анимаций:
// состояние выбирается по вводу и не зависит от того, что есть в паке.
//
//	состояние   клип     если клипа нет
//	Idle        idle     walk (замирает на первом кадре)
//	Walk        walk     замедленный run
//	Run         run      ускоренный вдвое walk
//	Attacking   см. attackClip
//	Hurt        hurt     idle → walk
//	Dead        death    hurt → idle → walk (и растворение)
func (p *Player) clipFor(s State) (string, *anim.Clip) {
	switch s {
	case Idle:
		return p.pick("idle", "walk")
	case Walk:
		if n, c := p.pick("walk"); c != nil {
			return n, c
		}
		if _, c := p.pick("run"); c != nil {
			return "walk*", c.Retimed(c.FrameTicks * 2)
		}
		return p.pick("idle")
	case Run:
		if n, c := p.pick("run"); c != nil {
			return n, c
		}
		if _, c := p.pick("walk"); c != nil {
			return "run*", c.Retimed(max(1, c.FrameTicks/2))
		}
		return p.pick("idle")
	case Hurt:
		return p.pick("hurt", "idle", "walk")
	case Dead:
		return p.pick("death", "hurt", "idle", "walk")
	}
	return "", nil
}

// pick — первый существующий клип из списка имён для текущего направления.
func (p *Player) pick(names ...string) (string, *anim.Clip) {
	for _, n := range names {
		if c := p.Pack.Clip(n, p.dir); c != nil {
			return n, c
		}
	}
	return "", nil
}

// play заряжает клип, если это не тот же самый, что уже идёт.
//
// Сравниваются имя и направление, а не сам клип: подменыши (walk*, run*,
// attack*) создаются заново на каждый вызов, и сравнение по указателю
// перезапускало бы их каждый тик — походка бы дёргалась на первом кадре.
// Направление в сравнении обязательно: имя клипа при повороте то же, а кадры
// берутся из другой строки листа.
func (p *Player) play(name string, c *anim.Clip) {
	if name == p.clip && p.clipDir == p.dir && c != nil {
		return
	}
	p.clip, p.clipDir = name, p.dir
	p.player.Play(c)
}

// Draw рисует персонажа так, чтобы его точка опоры легла в screen. Мигание
// неуязвимости и растворение трупа — тоже здесь: это часть показа состояния,
// а не логики.
func (p *Player) Draw(dst *ebiten.Image, screen engine.Vec2) {
	img := p.player.Frame()
	if img == nil || p.gone {
		return
	}
	foot := p.Pack.Foot()
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(screen.X-float64(foot.X), screen.Y-float64(foot.Y))
	switch {
	case p.fade > 0:
		op.ColorScale.ScaleAlpha(float32(p.fade) / float32(p.Cat.Base.Death.FadeTicks))
	case p.invuln > 0 && p.invuln/4%2 == 0:
		op.ColorScale.Scale(2, 2, 2, 1) // кадры неуязвимости — мигание
	}
	dst.DrawImage(img, op)
}

// clampLen обрезает длину вектора до 1: диагональ на клавиатуре не должна быть
// быстрее прямой, а геймпад может прислать что угодно.
func clampLen(v engine.Vec2) engine.Vec2 {
	if v.Len() > 1 {
		return v.Normalized()
	}
	return v
}
