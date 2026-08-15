package scene

import (
	"fmt"
	"image/color"
	"math"
	"math/rand/v2"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/vladislav/game/internal/assets"
	"github.com/vladislav/game/internal/audio"
	"github.com/vladislav/game/internal/character"
	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/engine"
	"github.com/vladislav/game/internal/item"
	"github.com/vladislav/game/internal/mob"
	"github.com/vladislav/game/internal/physics"
	"github.com/vladislav/game/internal/progress"
	"github.com/vladislav/game/internal/save"
	"github.com/vladislav/game/internal/settings"
	"github.com/vladislav/game/internal/sprite"
	"github.com/vladislav/game/internal/ui"
	"github.com/vladislav/game/internal/world"
)

const (
	// Карта забега: биом и сторона в тайлах. 128 тайлов по 16 px — мир 2048×2048.
	//
	// Биом экспортирован не ради игры, а ради сборщика ресурсов
	// (tools/pack): в раздачу едут только те биомы, куда игра действительно
	// ходит, и список этот он обязан спрашивать у игры, а не помнить сам.
	GameBiome = "forest"
	gameSize  = 128
	// gameLoadout — чем вооружён герой. Пока один на всех: выбирается тело.
	gameLoadout = "unarmed"
	animalsRoot = "mobs/animals"
	enemiesRoot = "mobs/enemies"
	bossesRoot  = "mobs/bosses"

	// barShow — сколько держать полоску здоровья над задетым зверем.
	barShow = 3 * config.TPS
)

// actor — то, что рисуется в общем проходе по глубине: зверь, враг или что
// угодно ещё с позицией и способом нарисоваться.
type actor struct {
	y    float64
	pos  engine.Vec2
	draw func(*ebiten.Image, engine.Vec2)
}

// Game — сама игра: карта, герой, звери. Сцена ничего не генерирует и не
// придумывает — только связывает готовые части: карту (internal/world), автомат
// персонажа (internal/character) и популяцию животных (internal/mob).
type Game struct {
	back Scene

	m   *world.Map
	pl  *character.Player
	sp  *mob.Spawner
	es  *mob.EnemySpawner
	cam *engine.Camera

	packs  map[string]*sprite.Pack // паки животных: пак на вид, грузится один раз
	epacks map[string]*sprite.Pack // паки врагов: пак на пару «тип/тир»
	l      *assets.Loader

	fx    effects             // числа урона, тряска экрана, свечение края
	sfx   *gameSound          // звук забега; nil — играем молча (см. gamesound.go)
	bars  map[*mob.Animal]int // сколько ещё показывать полоску здоровья зверя
	ebars map[*mob.Enemy]int  // то же для врагов
	hp    int                 // здоровье героя в прошлом тике — так ловится урон

	// prog — уровень, опыт и очки прокачки героя. Живёт в сцене, а не в
	// character.Player: тот отвечает за тело и анимации, а знать, кого герой
	// добил и какого тот был уровня, может только игровой слой.
	prog progress.Character

	// Сохранение забега: книга на диске, слот в ней и запись персонажа.
	// Счётчики убитых и время игры копятся прямо в записи — она и есть то, что
	// переживает выход. store == nil — забег никуда не сохраняется (тесты).
	store *save.Store
	slot  int
	char  *save.Char
	psec  int // тиков до следующей секунды игрового времени
	auto  int // тиков до автосохранения

	items *item.Catalog   // что такое выпадающие идентификаторы
	bag   *item.Inventory // сумка героя
	eq    *item.Equipment // надетое: от гнезда оружия зависит, чем герой бьёт
	chest *chest          // сундук забега (nil — места на карте не нашлось)

	loot  *lootRules    // правила добычи: разброс, притяжение, подбор
	drops []*groundItem // что уже лежит на карте
	lrng  *rand.Rand    // своё зерно на добычу

	// skills — украденные умения: три ячейки внизу справа. Живут в сцене, а не
	// в персонаже: заряды тратит игровой слой, а персонаж только машет.
	skills   skillBar
	castTint color.RGBA // цвет следа начатого удара чужой силой

	// Каталог и тело нужны после старта: надел оружие — сменился лоадаут, а
	// значит и пак анимаций пары «тело × лоадаут».
	chars *character.Catalog
	body  *character.Body

	bigMap bool // карта на весь экран (TAB)
}

// Ячеек в сумке и в сундуке — ровно столько, сколько их в соответствующем
// окне: у героя сетка 4×4, у сундука 5×4. Расхождение поймает TestBagFitsPanel.
const bagSlots, chestSlots = 16, 20

// NewGame готовит одиночный забег без сохранения: тело bodyID, случайный мир,
// чистый герой. Осталась для тестов и отладки — игра заходит через
// NewSavedGame, потому что у неё забег всегда чей-то.
func NewGame(l *assets.Loader, back Scene, bodyID string) (*Game, error) {
	return NewSavedGame(l, back, nil, -1, NewChar("", bodyID))
}

// NewSavedGame готовит забег персонажа c: поднимает его мир, заселяет карту и
// возвращает герою всё нажитое (см. restore). Забег сохраняется в слот slot
// книги st; st == nil — не сохраняется вовсе.
//
// Мир у персонажа один и тот же от забега к забегу: карта берётся из его
// сохранения целиком (loadMap), а не выводится заново из сида. Новому
// персонажу карта генерируется один раз — здесь, и тут же ложится на диск.
func NewSavedGame(l *assets.Loader, back Scene, st *save.Store, slot int, c *save.Char) (*Game, error) {
	if c == nil {
		return nil, fmt.Errorf("scene: забег без персонажа")
	}
	// Короткий сид: его видно в журнале, и по нему карта воспроизводится, если
	// сохранённой не оказалось (шум всё равно перемешивает его splitmix-хешем,
	// качество не теряется).
	seed := uint64(c.Seed)
	m, err := loadMap(l, st, slot, c)
	if err != nil {
		return nil, err
	}

	cat, err := character.Load(l.FS(), "character/character.json")
	if err != nil {
		return nil, err
	}
	body, lo := cat.Body(c.Body), cat.Loadout(gameLoadout)
	if body == nil || lo == nil {
		return nil, fmt.Errorf("scene: нет героя %q/%q", c.Body, gameLoadout)
	}
	pack, err := character.LoadPack(l, "character", body, lo)
	if err != nil {
		return nil, err
	}

	g := &Game{
		back:   back,
		m:      m,
		pl:     character.NewPlayer(cat, body, lo, pack, m.Spawn()),
		cam:    engine.NewCamera(config.ScreenW, config.ScreenH),
		packs:  map[string]*sprite.Pack{},
		epacks: map[string]*sprite.Pack{},
		l:      l,
		bars:   map[*mob.Animal]int{},
		ebars:  map[*mob.Enemy]int{},
		prog:   progress.New(),
		store:  st,
		slot:   slot,
		char:   c,
		sfx:    newGameSound(),
	}
	g.hp = g.pl.HP

	species, err := mob.LoadSpecies(l.FS(), animalsRoot+"/species.json")
	if err != nil {
		return nil, err
	}
	cfg, err := mob.LoadSpawnConfig(l.FS(), animalsRoot+"/spawn.json")
	if err != nil {
		return nil, err
	}
	// Звери сеются от сида карты: одна и та же карта заселяется одинаково. Само
	// заселение — в restoreWorld: в мир, где уже жили, возвращаются те самые
	// звери, каких игрок там оставил, и досыпать к ним случайных нельзя.
	g.sp = mob.NewSpawner(cfg, species, m, g.pack, rand.New(rand.NewPCG(seed, 0x9E3779B97F4A7C15)))

	// Враги: своя таблица, свои профили поведения и свой бюджет опасности.
	// Ночи в забеге пока нет, поэтому заселяем дневным бюджетом.
	es, err := newEnemySpawner(l, m, g.enemyPack, seed)
	if err != nil {
		return nil, err
	}
	g.es = es
	g.es.Guard(m.Spawn()) // на старте игрока никто не ждёт

	// Предметы: каталог один на забег, сумка героя пуста, и где-то рядом со
	// стартом стоит сундук. Своё зерно, чтобы добыча и место сундука не
	// зависели от того, сколько случайных чисел потратили звери и враги.
	items, err := item.Load(l.FS(), itemsFile)
	if err != nil {
		return nil, err
	}
	g.items = items
	g.bag = item.NewInventory(items, bagSlots)
	g.eq = item.NewEquipment(items)
	g.chars, g.body = cat, body
	// Правила выпадения общие на всю игру, а бросок — свой на забег: добыча не
	// должна зависеть от того, сколько случайных чисел потратили звери и враги.
	lr, err := loadLoot(l.FS(), lootFile)
	if err != nil {
		return nil, err
	}
	g.loot = lr
	g.lrng = rand.New(rand.NewPCG(seed, 0x14057B7EF767814F))
	ch, err := newChest(l, m, items, chestSlots, m.Spawn(), rand.New(rand.NewPCG(seed, 0x2545F4914F6CDD1D)))
	if err != nil {
		return nil, err
	}
	g.chest = ch

	// Всё нажитое возвращается последним: сумка и надетое уже есть, сундук
	// стоит, и накладывать сохранённое поверх готового забега безопасно.
	g.restore()

	mw, mh := m.Size()
	g.cam.Follow(g.pl.Pos, mw, mh)
	return g, nil
}

// pack — загрузчик паков для спавнера (с кэшем: один вид — один пак на забег).
func (g *Game) pack(art string) (*sprite.Pack, error) {
	if p, ok := g.packs[art]; ok {
		return p, nil
	}
	p, err := sprite.Load(g.l, animalsRoot+"/"+art)
	if err != nil {
		return nil, err
	}
	g.packs[art] = p
	return p, nil
}

func (g *Game) Update() (Scene, error) {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		return newPause(g), nil
	}
	if inpututil.IsKeyJustPressed(settings.Key(settings.ActMap)) {
		g.bigMap = !g.bigMap
	}
	if inpututil.IsKeyJustPressed(settings.Key(settings.ActBag)) {
		if v := newEquipView(g); v != nil {
			return v, nil
		}
	}
	g.m.Update()
	g.fx.update()
	g.tickBars()
	g.tickEnemyBars()
	g.tickSave() // время игры и автосохранение считаются только в самом забеге

	g.pl.Update(g.input(), g.m.Field())
	g.strike()
	// Замах сбили — цвет украденного следа не должен достаться следующему
	// удару, уже своему.
	if g.pl.State() != character.Attacking {
		g.castTint = color.RGBA{}
	}
	g.sp.Update(g.pl.Pos, g.pl.Alive(), false)
	g.es.Update(g.target(), g.pl.Alive(), g.pl.Floor(), false)
	g.animalHits()
	g.enemyHits()
	// Расталкивание — после ударов: попадания этого кадра уже посчитаны по
	// позициям до него, и выталкивание не уводит цель из-под засчитанного удара.
	g.separate()
	g.separateEnemies()

	// Добыча ведётся после расталкивания: герой уже стоит там, где закончил
	// шаг, и подбор считается по итоговому положению, а не по промежуточному.
	g.updateLoot()

	if next := g.useChest(); next != nil {
		return next, nil
	}

	// Урон герою ловится по изменению здоровья: неважно, кто именно ударил —
	// показать надо одно и то же.
	if g.pl.HP < g.hp {
		g.fx.hurt()
		g.fx.pop(engine.Vec2{X: g.pl.Pos.X, Y: g.pl.Pos.Y - 34}, g.hp-g.pl.HP, fxHurt)
		// Не PlayAt: собственный урон — единственное, что обязано пробиться
		// через кашу боя, и приглушать его дистанцией нечем — он всегда здесь.
		g.sfx.play(audio.Hurt)
	}
	g.hp = g.pl.HP

	// Смерть: экран поражения ждёт, пока доиграет анимация и растает труп.
	if g.pl.Gone() {
		g.died() // смерть попадает в сохранение сразу: отсюда часто уходят
		return newDeath(g), nil
	}

	w, h := g.m.Size()
	g.cam.Follow(g.pl.Pos, w, h)
	g.cam.Pos = g.cam.Pos.Add(g.fx.offset()) // тряска — поверх слежения за героем
	// Звук — последним: шаги считаются по итоговому положению героя, а уши
	// стоят там же, где он закончил кадр.
	g.sfx.tick(g)
	return g, nil
}

// toMenu уводит в меню, запомнив забег: мир, герой и звери остаются в памяти
// сцены, и ESC в меню возвращает игрока ровно туда, где он вышел.
func (g *Game) toMenu() Scene {
	g.persist() // из меню игрок может и не вернуться — забег ложится на диск
	back := g.back
	if back == nil {
		back = NewMenu()
	}
	if m, ok := back.(*Menu); ok {
		m.Resume = g
	}
	return back
}

// revive поднимает героя на точке старта — забег продолжается.
func (g *Game) revive() Scene {
	g.pl.Revive(g.m.Spawn())
	g.hp = g.pl.HP
	g.fx = effects{}
	return g
}

// input переводит клавиши и мышь в намерения персонажа.
func (g *Game) input() character.Input {
	var in character.Input
	// Клавиши берутся из настроек — их игрок переназначает сам. Стрелки оставлены
	// вторым, неизменяемым набором: чем бы ни было занято WASD, ходить можно.
	if ebiten.IsKeyPressed(settings.Key(settings.ActLeft)) || ebiten.IsKeyPressed(ebiten.KeyLeft) {
		in.Move.X--
	}
	if ebiten.IsKeyPressed(settings.Key(settings.ActRight)) || ebiten.IsKeyPressed(ebiten.KeyRight) {
		in.Move.X++
	}
	if ebiten.IsKeyPressed(settings.Key(settings.ActUp)) || ebiten.IsKeyPressed(ebiten.KeyUp) {
		in.Move.Y--
	}
	if ebiten.IsKeyPressed(settings.Key(settings.ActDown)) || ebiten.IsKeyPressed(ebiten.KeyDown) {
		in.Move.Y++
	}
	in.Run = ebiten.IsKeyPressed(settings.Key(settings.ActRun))
	in.Attack = inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) ||
		inpututil.IsKeyJustPressed(settings.Key(settings.ActAttack))
	// Ячейки умений: нажатие сразу тратит заряд, поэтому просьба собирается
	// здесь, а не в Draw или в отдельном проходе, — тик один, и списание с
	// ударом обязаны случиться в нём же (см. castRequest).
	in.Cast = g.castRequest()

	// Прицел мышью: герой смотрит туда, где курсор, и бьёт туда же — сектор
	// удара берётся из направления взгляда. Ходьба от этого не зависит: идти
	// можно в любую сторону, хоть спиной вперёд, отступая от того, кого бьёшь.
	mx, my := ebiten.CursorPosition()
	in.Aim = g.cam.ScreenToWorld(engine.Vec2{X: float64(mx), Y: float64(my)})
	in.HasAim = true
	return in
}

// strike раздаёт урон удара героя: сектор отдаёт автомат персонажа, а кого он
// задел — решает игровой слой (у персонажа нет списка целей).
//
// Одиночное оружие (h.Single) достаётся одной цели — ближайшей из накрытых:
// палкой нельзя задеть троих одним взмахом. Площадное бьёт всех, кого накрыло.
func (g *Game) strike() {
	h, ok := g.pl.Strike()
	if !ok {
		return
	}
	// След замаха рисуется всегда, попал герой или нет: промах должно быть видно
	// так же ясно, как попадание, а лоадаут может не иметь клипа удара вовсе.
	// Цвет говорит, чем ударили: украденная сила светится своей стихией.
	g.fx.swing(h.Center, h.Face, h.Reach, h.Arc, g.takeCastTint())
	// Замах слышен всегда, как и виден: промах — тоже событие, и без звука
	// непонятно, случился ли удар вообще.
	g.sfx.at(g.swingSound(), h.Center)
	if h.Single {
		g.strikeNearest(h)
		return
	}
	for _, a := range g.sp.Animals() {
		if a.Alive() && h.Covers(a.Pos, a.Radius()) {
			g.hitAnimal(a, h)
		}
	}
	g.strikeEnemies(h)
}

// strikeNearest бьёт одну цель — ближайшую к герою из накрытых сектором. Звери
// и враги перебираются вместе: удар не знает, кто перед ним, и выбирать между
// оленем и гоблином по списку, в котором они лежат, было бы случайностью.
func (g *Game) strikeNearest(h character.Hit) {
	var (
		best   = math.Inf(1)
		animal *mob.Animal
		enemy  *mob.Enemy
	)
	for _, a := range g.sp.Animals() {
		if !a.Alive() || !h.Covers(a.Pos, a.Radius()) {
			continue
		}
		if d := engine.Dist(h.Center, a.Pos); d < best {
			best, animal, enemy = d, a, nil
		}
	}
	for _, e := range g.es.Enemies() {
		if !e.Alive() || !h.Covers(e.Pos, e.Radius()) {
			continue
		}
		if d := engine.Dist(h.Center, e.Pos); d < best {
			best, animal, enemy = d, nil, e
		}
	}
	switch {
	case animal != nil:
		g.hitAnimal(animal, h)
	case enemy != nil:
		g.hitEnemy(enemy, h)
	}
}

// hitAnimal проводит попадание по зверю: урон, добыча с туши, опыт.
func (g *Game) hitAnimal(a *mob.Animal, h character.Hit) {
	dmg := h.Damage.Total()
	// По зверю звучит плоть независимо от оружия: брони на нём нет, и звон
	// металла здесь означал бы попадание не туда.
	g.sfx.at(audio.HitFlesh, a.Pos)
	if a.Hit(dmg) {
		// Добил — с туши падает добыча. У зверя нет тиров, поэтому бросок
		// один: сколько раз кидать кости, сказано только у врагов.
		g.dropLoot(a.Pos, a.Floor(), a.Species.Drops, 1, false)
		// Число опыта заменяет собой число урона: добил — и всё.
		g.reward(g.headOf(a), a.Level, a.XP)
		g.count(save.KillAnimal(a.Species.ID))
	} else {
		g.fx.pop(g.headOf(a), dmg, fxDamage)
	}
	g.bars[a] = barShow
}

// animalHits — ответный урон: зверь в состоянии Attack бьёт того, до кого
// дотянулся. Неуязвимость после удара считает сам персонаж, поэтому проверять
// перезарядку здесь не нужно.
func (g *Game) animalHits() {
	if !g.pl.Alive() {
		return
	}
	for _, a := range g.sp.Animals() {
		t := a.Species.Threat
		if a.State() != mob.Attack || !t.Attacks {
			continue
		}
		if engine.Dist(a.Pos, g.pl.Pos) <= a.Radius()+g.pl.Radius()+8 {
			g.pl.Damage(t.Damage, a.Pos)
		}
	}
}

// Расталкивание тел. playerShare — какую долю перекрытия при столкновении со
// зверем забирает герой: тело у него тяжелее, и стадо оленей не должно возить
// игрока по карте. Остаток берёт на себя зверь, суммарно всегда полное
// перекрытие, иначе тела остались бы внахлёст.
const playerShare = 0.25

// separate разводит тела, налезшие друг на друга: каждое отходит на свою долю
// перекрытия. Ни импульсов, ни масс — для топ-дауна этого хватает, а толпа
// перестаёт слипаться в одну точку.
//
// Смещение идёт через Push, то есть через то же поле: вытолкнуть кого-то в воду
// или в скалу нельзя, а тому, кому отходить некуда, разойдётся сосед.
//
// Этажи обязательно сравниваются: зверь на макушке и зверь под обрывом
// разделены стеной, и толкать друг друга сквозь неё они не могут.
//
// Зазор между телами (r₁+r₂) меньше, чем дистанция удара зверя (r+7+8), так что
// драться расталкивание не мешает: разведённый зверь всё ещё достаёт героя.
func (g *Game) separate() {
	f := g.m.Field()
	animals := g.sp.Animals()
	for i, a := range animals {
		if !a.Alive() {
			continue // труп не толкается: он уже растворяется
		}
		for _, b := range animals[i+1:] {
			if !b.Alive() || a.Floor() != b.Floor() {
				continue
			}
			if d, ok := physics.Separate(a.Pos, a.Radius(), b.Pos, b.Radius()); ok {
				a.Push(f, d)
				b.Push(f, d.Scale(-1))
			}
		}
	}
	if !g.pl.Alive() {
		return
	}
	for _, a := range animals {
		if !a.Alive() || a.Floor() != g.pl.Floor() {
			continue
		}
		if d, ok := physics.Separate(g.pl.Pos, g.pl.Radius(), a.Pos, a.Radius()); ok {
			g.pl.Push(f, d.Scale(2*playerShare))
			a.Push(f, d.Scale(-2*(1-playerShare)))
		}
	}
}

// tickBars гасит полоски здоровья зверей, которых давно не трогали.
func (g *Game) tickBars() {
	for a, ttl := range g.bars {
		if ttl <= 1 || !a.Alive() || a.Gone() {
			delete(g.bars, a)
			continue
		}
		g.bars[a] = ttl - 1
	}
}

// headOf — точка над зверем (макушка): туда всплывают числа и там висит полоска.
func (g *Game) headOf(a *mob.Animal) engine.Vec2 {
	return engine.Vec2{X: a.Pos.X, Y: a.Pos.Y - float64(a.Pack.Bounds().H) - 4}
}

// aimAt — зверь под курсором. Рамка берётся по непрозрачным пикселям кадра:
// целиться приходится в зверя, а не в его пустой угол.
func (g *Game) aimAt() *mob.Animal {
	mx, my := ebiten.CursorPosition()
	p := g.cam.ScreenToWorld(engine.Vec2{X: float64(mx), Y: float64(my)})
	var best *mob.Animal
	bestD := math.Inf(1)
	for _, a := range g.sp.Animals() {
		if !a.Alive() {
			continue
		}
		b := a.Pack.Bounds()
		w, h := float64(b.W)/2, float64(b.H)
		if p.X < a.Pos.X-w || p.X > a.Pos.X+w || p.Y < a.Pos.Y-h || p.Y > a.Pos.Y+4 {
			continue
		}
		if d := engine.Dist(a.Pos, p); d < bestD {
			best, bestD = a, d
		}
	}
	return best
}

func (g *Game) Draw(screen *ebiten.Image) {
	ui.Cursor = ui.CursorAim // в игре целимся мышью
	g.m.Draw(screen, g.cam)

	// Всё живое рисуется по глубине: кто ниже на карте, тот ближе к зрителю.
	// Звери и враги идут одним списком — между собой они тоже перекрываются.
	order := make([]actor, 0, len(g.sp.Animals())+len(g.es.Enemies()))
	for _, a := range g.sp.Animals() {
		order = append(order, actor{y: a.Pos.Y, pos: a.Pos, draw: a.Draw})
	}
	for _, e := range g.es.Enemies() {
		order = append(order, actor{y: e.Pos.Y, pos: e.Pos, draw: e.Draw})
	}
	if c := g.chest; c != nil {
		// Сундук — такое же тело в проходе по глубине: стоящий выше героя он
		// обязан уйти за спину, а не лечь поверх.
		order = append(order, actor{y: c.depth(), pos: c.pos, draw: c.draw})
	}
	// Лежащая добыча — в том же проходе: вещь за деревом или за спиной героя
	// обязана скрыться, иначе она читается как значок интерфейса, а не как
	// предмет на земле.
	for _, d := range g.drops {
		order = append(order, actor{y: d.depth(), pos: d.at, draw: d.draw})
	}
	slices.SortFunc(order, func(a, b actor) int {
		switch {
		case a.y < b.y:
			return -1
		case a.y > b.y:
			return 1
		}
		return 0
	})
	// Объекты карты (деревья, камни, руины) идут в этом же проходе: дерево ниже
	// героя обязано его закрыть, выше — уйти за спину. В запечённый холст их
	// поэтому не положить, они рисуются покадрово и уже отсортированы по глубине.
	pi := 0
	propsUpTo := func(y float64) {
		for pi < g.m.PropCount() && g.m.PropDepth(pi) <= y {
			g.m.DrawProp(screen, g.cam, pi)
			pi++
		}
	}
	drawn := false
	for _, a := range order {
		if !drawn && a.y > g.pl.Pos.Y {
			propsUpTo(g.pl.Pos.Y)
			g.pl.Draw(screen, g.cam.WorldToScreen(g.pl.Pos))
			drawn = true
		}
		propsUpTo(a.y)
		a.draw(screen, g.cam.WorldToScreen(a.pos))
	}
	if !drawn {
		propsUpTo(g.pl.Pos.Y)
		g.pl.Draw(screen, g.cam.WorldToScreen(g.pl.Pos))
	}
	propsUpTo(math.Inf(1)) // всё, что ниже последнего живого

	g.drawTargets(screen)
	g.drawChestHint(screen)
	g.drawEnemyBars(screen)
	g.fx.drawSlashes(screen, g.cam)
	g.fx.drawPopups(screen, g.cam)
	g.fx.drawFlash(screen)
	g.drawHUD(screen)
	g.drawSkills(screen)
	if g.bigMap {
		g.drawBigMap(screen)
	}
}

// drawTargets — полоски здоровья над зверями: над тем, кто под курсором (ещё и
// с названием вида), и над теми, кого недавно задели.
func (g *Game) drawTargets(dst *ebiten.Image) {
	aim := g.aimAt()
	for a := range g.bars {
		if a != aim {
			g.drawBeastBar(dst, a, false)
		}
	}
	if aim != nil {
		g.drawBeastBar(dst, aim, true)
	}
}

func (g *Game) drawBeastBar(dst *ebiten.Image, a *mob.Animal, named bool) {
	const w, h = 26, 3
	head := g.cam.WorldToScreen(g.headOf(a))
	x, y := float32(head.X)-w/2, float32(head.Y)
	if x < -w || x > config.ScreenW || y < -h || y > config.ScreenH {
		return
	}
	frac := float32(a.HP) / float32(max(a.Species.Stats.HP, 1))
	col := hudBeastCalm
	if a.Species.Threat.Attacks {
		col = hudLow
	}
	vector.FillRect(dst, x-1, y-1, w+2, h+2, hudBack, false)
	vector.FillRect(dst, x, y, w*frac, h, col, false)
	if named {
		g.drawTargetName(dst, a.Species.Title.RU, a.Level, head.X, float64(y)-9)
	}
}

// Геометрия HUD: слева вверху полоса здоровья и строка состояния, справа
// вверху — мини-карта. Карта убрана из левого угла нарочно: она перекрывала
// обзор в той стороне, куда игрок чаще идёт, а по TAB всё равно открывается
// крупная.
const (
	hudPad     = 8
	hudMini    = 56  // сторона мини-карты в правом верхнем углу
	hudBigMini = 288 // сторона карты на весь экран (TAB)
	// Запасные размеры полосы: работают, только если арт полос не загрузился.
	hudBarW = 120
	hudBarH = 12
	hudBarY = hudPad
)

// Цвета HUD: тёмная подложка, зелёная полоса здоровья.
var (
	hudBack      = color.RGBA{0x0d, 0x10, 0x18, 0xc8}
	hudEdge      = color.RGBA{0x8a, 0x96, 0xb8, 0xff}
	hudHP        = color.RGBA{0x6f, 0xc0, 0x54, 0xff}
	hudLow       = color.RGBA{0xd0, 0x5a, 0x40, 0xff}
	hudText      = color.RGBA{0xe8, 0xef, 0xff, 0xff}
	hudDim       = color.RGBA{0x9a, 0xa6, 0xc4, 0xff}
	hudBeast     = color.RGBA{0xe8, 0xd0, 0x70, 0xff}
	hudXP        = color.RGBA{0x5c, 0xa8, 0xe0, 0xff}
	hudShadow    = color.RGBA{0x00, 0x00, 0x00, 0xc0} // подложка под подписи в мире
	hudBeastCalm = color.RGBA{0x9a, 0xc8, 0x70, 0xff}
	hudSelf      = color.RGBA{0xff, 0xff, 0xff, 0xff}
	hudView      = color.RGBA{0xff, 0xff, 0xff, 0x88}
)

func (g *Game) drawHUD(dst *ebiten.Image) {
	// В левом верхнем углу — только здоровье: счётчики и сид оттуда убраны, они
	// мешали смотреть в ту сторону, куда идёшь. FPS при нужде включается
	// отдельным счётчиком в настройках.
	style := settings.Get().BarStyle
	barW, barH := ui.BarSize(style)
	styled := barW > 0
	if !styled {
		barW, barH = hudBarW, hudBarH
	}
	hp := fmt.Sprintf("%d / %d", g.pl.HP, g.pl.MaxHP)

	// В жёлоб полосы числа влезают не у всякого стиля: у тонких они уходят
	// строкой под полосу, и от этого зависит высота подложки.
	slotX, slotY, slotW, slotH, hasSlot := ui.BarSlot(style)
	inSlot := !styled || (hasSlot && float64(slotH) >= ui.PixelTextHeight(1))

	panelH := float32(barH) + 8
	if !inSlot {
		panelH += float32(ui.PixelTextHeight(1)) + 4
	}
	// Полоса опыта живёт под блоком здоровья и той же ширины: это одна панель
	// героя, а не два прибора рядом.
	xpY := float32(hudPad-4) + panelH + xpBarGap
	panelH += xpBlockH()
	vector.FillRect(dst, hudPad-4, hudPad-4, float32(barW)+8, panelH, hudBack, false)
	g.drawXPBar(dst, hudPad, xpY, float32(barW))

	g.drawMapView(dst, config.ScreenW-hudPad-hudMini, hudPad, hudMini)

	frac := 0.0
	if g.pl.MaxHP > 0 {
		frac = float64(g.pl.HP) / float64(g.pl.MaxHP)
	}
	if styled {
		ui.DrawBar(dst, style, hudPad, hudBarY, frac)
		if inSlot {
			ui.PixelTextCentered(dst, hp, hudPad+float64(slotX)+float64(slotW)/2,
				hudBarY+float64(slotY)+(float64(slotH)-ui.PixelTextHeight(1))/2, 1, hudText)
		} else {
			ui.PixelText(dst, hp, hudPad, float64(hudBarY+barH)+4, 1, hudText)
		}
		return
	}
	col := hudHP
	if frac <= 0.3 {
		col = hudLow
	}
	vector.FillRect(dst, hudPad, hudBarY, float32(barW)*float32(frac), float32(barH), col, false)
	vector.StrokeRect(dst, hudPad+0.5, hudBarY+0.5, float32(barW)-1, float32(barH)-1, 1, hudEdge, false)
	ui.PixelText(dst, hp, hudPad+4, hudBarY+3, 1, hudText)
}

// drawBigMap — карта на весь экран: та же картинка, только крупнее. Мир при
// этом живёт: карта не пауза, а взгляд по сторонам.
func (g *Game) drawBigMap(dst *ebiten.Image) {
	vector.FillRect(dst, 0, 0, config.ScreenW, config.ScreenH, ovDimPause, false)
	x := float32(config.ScreenW-hudBigMini) / 2
	y := float32(config.ScreenH-hudBigMini)/2 - 6
	g.drawMapView(dst, x, y, hudBigMini)
	ui.PixelTextCentered(dst, "КАРТА "+g.m.Biome()+" - TAB ЗАКРЫТЬ",
		config.ScreenW/2, float64(y+hudBigMini)+10, 1, hudDim)
	ui.Cursor = ui.CursorArrow
}

// drawMapView рисует обзор карты стороной side: сама карта — готовая картинка из
// world, а поверх неё то, что меняется каждый кадр.
func (g *Game) drawMapView(dst *ebiten.Image, x, y, side float32) {
	// Тёмная подложка под рамкой: на светлой траве однопиксельная рамка
	// пропадает, а мини-карта должна читаться как отдельный прибор.
	vector.FillRect(dst, x-2, y-2, side+4, side+4, hudBack, false)
	if img := g.m.Minimap(int(side)); img != nil {
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(float64(x), float64(y))
		dst.DrawImage(img, op)
	}

	dot := float32(1)
	if side > hudMini {
		dot = 2 // на большой карте точка в один пиксель теряется
	}
	for _, a := range g.sp.Animals() {
		if !a.Alive() {
			continue
		}
		px, py := g.m.MiniPos(a.Pos, int(side))
		col := hudBeast
		if a.Species.Threat.Attacks {
			col = hudLow
		}
		vector.FillRect(dst, x+px, y+py, dot, dot, col, false)
	}

	// Рамка обзора: что из карты сейчас на экране.
	mw, mh := g.m.Size()
	vx, vy := g.m.MiniPos(engine.Vec2{
		X: g.cam.Pos.X - g.cam.ViewW/2,
		Y: g.cam.Pos.Y - g.cam.ViewH/2,
	}, int(side))
	vector.StrokeRect(dst, x+vx+0.5, y+vy+0.5,
		float32(g.cam.ViewW/mw*float64(side)), float32(g.cam.ViewH/mh*float64(side)),
		1, hudView, false)

	px, py := g.m.MiniPos(g.pl.Pos, int(side))
	vector.FillRect(dst, x+px-dot, y+py-dot, dot*2+1, dot*2+1, hudSelf, false)

	vector.StrokeRect(dst, x-0.5, y-0.5, side+1, side+1, 1, hudEdge, false)
}
