package scene

// Сохранение забега: снимок игры в save.Char и обратная сборка из него.
//
// Сохраняется всё, что игрок застал на экране: сам герой (уровень, опыт,
// здоровье, место), нажитое (сумка, надетое, сундук, счётчики убитых) и мир
// вокруг — карта, звери, враги и вещи, брошенные на землю. Возвращаясь, игрок
// попадает не в «такой же» мир, а в свой: тот же волк с отгрызенным боком у той
// же скалы.
//
// Карта живёт отдельным файлом (тяжёлая и неизменная), население — в записи
// персонажа (лёгкое и меняется каждый тик); почему так — в docs/save.md.
//
// Не сохраняются только сиюминутные мелочи: недоигранные эффекты боя, полоски
// здоровья над задетыми, кто на кого разозлился. Их цена — секунда игры.

import (
	"log"
	"math/rand/v2"
	"time"

	"github.com/vladislav/game/internal/assets"
	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/engine"
	"github.com/vladislav/game/internal/item"
	"github.com/vladislav/game/internal/mob"
	"github.com/vladislav/game/internal/physics"
	"github.com/vladislav/game/internal/progress"
	"github.com/vladislav/game/internal/save"
	"github.com/vladislav/game/internal/world"
)

// autosaveTicks — как часто забег сам ложится на диск. Полминуты: чаще —
// дёргать файл посреди боя, реже — терять слишком много при вылете.
const autosaveTicks = 30 * config.TPS

// lootLanded — возраст вещи, вернувшейся из сохранения: заведомо больше и
// прыжка, и задержки подбора (обе задаются в items/loot.json миллисекундами и
// счётом идут на десятки тиков). Час игры в тиках — число, которого ни то ни
// другое не достигнет.
const lootLanded = 60 * 60 * config.TPS

// NewChar заводит нового персонажа: имя, тело и свой собственный мир. Сид
// короткий — его видно в журнале, и по нему карта повторяется.
func NewChar(name, bodyID string) *save.Char {
	return &save.Char{
		Name:  save.CleanName(name),
		Body:  bodyID,
		Seed:  int64(rand.Uint64N(1_000_000)),
		Biome: GameBiome,
		Level: 1,
		Kills: map[string]int{},
		Worn:  map[string]save.Slot{},
	}
}

// loadMap поднимает мир персонажа: сохранённую карту, если она есть, и
// свежесгенерированную, если персонаж новый (или его карта пропала).
//
// Сгенерированная карта тут же ложится на диск: мир персонажа заводится один
// раз и дальше не пересобирается никогда. Не записалась — забег всё равно
// начинается: играть в несохраняемом мире хуже, чем не играть вовсе, но лучше,
// чем упереться в ошибку на входе.
func loadMap(l *assets.Loader, st *save.Store, slot int, c *save.Char) (*world.Map, error) {
	if st != nil && slot >= 0 {
		if mv, ok := st.LoadMap(slot); ok {
			m, err := world.New(l, mv)
			if err == nil {
				return m, nil
			}
			// Карта есть, но не собирается (сменился формат, пропал тайлсет).
			// Это потеря мира, и молчать о ней нельзя — но и запирать игрока в
			// сломанном сохранении тоже: собираем заново из сида.
			log.Println("сохранённый мир не собрался, карта будет новая:", err)
		}
	}
	biome := c.Biome
	if biome == "" {
		biome = GameBiome
	}
	m, err := world.Generate(l, biome, uint64(c.Seed), gameSize)
	if err != nil {
		return nil, err
	}
	if st != nil && slot >= 0 {
		if err := st.SaveMap(slot, m.Source()); err != nil {
			log.Println("сохранение мира:", err)
		}
	}
	return m, nil
}

// count засчитывает убитого. Ключи строит save (KillAnimal/KillEnemy): счёт
// ведётся по видам, а не по «сколько всего», потому что игроку интересно
// именно «тетеревов пять, орков один».
func (g *Game) count(key string) {
	if g.char != nil {
		g.char.Kill(key)
	}
}

// tickSave ведёт время игры и роняет забег на диск по расписанию. Секунды
// считаются тиками, а не часами: пауза, меню и окно сундука — не игра.
func (g *Game) tickSave() {
	if g.char == nil {
		return
	}
	if g.psec++; g.psec >= config.TPS {
		g.psec = 0
		g.char.Playtime++
	}
	if g.auto++; g.auto >= autosaveTicks {
		g.auto = 0
		g.persist()
	}
}

// died отмечает смерть героя: она часть его биографии, и сохраняется сразу —
// именно на этом месте забег чаще всего и бросают.
func (g *Game) died() {
	if g.char == nil {
		return
	}
	g.char.Deaths++
	g.persist()
}

// persist пишет забег в свой слот. Best-effort: не сохранилось — игра
// продолжается, а причина уходит в лог. Ронять забег из-за занятого диска
// было бы худшим из возможных ответов.
func (g *Game) persist() {
	if g.store == nil || g.char == nil || g.slot < 0 {
		return // забег без сохранения (тесты, отладочный запуск)
	}
	g.snapshot()
	book := g.store.Load()
	book.Put(g.slot, g.char)
	if err := g.store.Save(book); err != nil {
		log.Println("сохранение:", err)
	}
}

// snapshot переписывает запись персонажа текущим состоянием забега.
func (g *Game) snapshot() {
	c := g.char
	if c == nil {
		return
	}
	c.Level, c.XP, c.Points = g.prog.Level, g.prog.XP, g.prog.Points

	// Мёртвый герой сохраняется живым на точке старта: смерть в этой игре стоит
	// пройденного пути, а не персонажа, и вернуться в сохранение, где ты уже
	// труп, значило бы потерять его совсем.
	c.HP, c.Pos, c.Floor = g.pl.HP, [2]float64{g.pl.Pos.X, g.pl.Pos.Y}, g.pl.Floor()
	if !g.pl.Alive() {
		spawn := g.m.Spawn()
		c.HP, c.Pos, c.Floor = g.pl.MaxHP, [2]float64{spawn.X, spawn.Y}, physics.FloorLow
	}
	c.Seed = int64(g.m.Seed())
	c.Biome = g.m.Biome()

	c.Bag = make([]save.Slot, 0, g.bag.Size())
	for _, s := range g.bag.Slots() {
		c.Bag = append(c.Bag, save.Slot{ID: s.ID, N: s.N})
	}
	c.Worn = map[string]save.Slot{}
	for slot, s := range g.eq.Worn() {
		c.Worn[slot] = save.Slot{ID: s.ID, N: s.N}
	}
	c.Chest = g.chest.state()
	g.snapshotWorld()
	c.Touch(time.Now())
}

// snapshotWorld запоминает живой мир: кто по нему ходит и что на нём лежит.
//
// Трупы и растворяющиеся не сохраняются: они уже отдали и добычу, и опыт, и
// возвращать их — значит показать игроку смерть, которой не было. По той же
// причине не сохраняется здоровье выше предела: раненый возвращается раненым,
// но не наоборот.
func (g *Game) snapshotWorld() {
	c := g.char

	c.Beasts = c.Beasts[:0]
	for _, a := range g.sp.Animals() {
		if !a.Alive() {
			continue
		}
		c.Beasts = append(c.Beasts, save.Beast{
			Species: a.Species.ID,
			Pos:     [2]float64{a.Pos.X, a.Pos.Y},
			Floor:   a.Floor(),
			HP:      a.HP,
		})
	}

	c.Foes = c.Foes[:0]
	for _, e := range g.es.Enemies() {
		if !e.Alive() {
			continue
		}
		c.Foes = append(c.Foes, save.Foe{
			Type:  e.Type.ID,
			Tier:  e.Tier.ID,
			Pos:   [2]float64{e.Pos.X, e.Pos.Y},
			Floor: e.Floor(),
			HP:    e.HP,
			Elite: e.Elite,
		})
	}

	// Вещи на земле сохраняются в конечной точке (pos), а не там, где они
	// сейчас в прыжке: в новом забеге прыгать им уже неоткуда.
	c.Ground = c.Ground[:0]
	for _, d := range g.drops {
		c.Ground = append(c.Ground, save.Drop{
			ID:    d.id,
			N:     d.n,
			Pos:   [2]float64{d.pos.X, d.pos.Y},
			Floor: d.floor,
			Skill: savedSkill(d.power),
		})
	}

	// Ячейки умений: пустые тоже записываются, иначе после выхода умения
	// съедут на клавиши, к которым игрок не привык.
	c.Skills = c.Skills[:0]
	for _, s := range g.skills.slots {
		sk := savedSkill(s)
		if sk == nil {
			sk = &save.Skill{}
		}
		c.Skills = append(c.Skills, *sk)
	}
}

// savedSkill — украденная сила в виде записи сохранения (nil — силы нет).
func savedSkill(s *mob.Stolen) *save.Skill {
	if s.Empty() {
		return nil
	}
	return &save.Skill{Type: s.Type, Tier: s.Tier, Left: s.Charges}
}

// restoreSkill собирает силу обратно по имени того, с кого её сняли. Числа
// приходят из таблицы врагов, а не из сохранения: баланс живёт в данных, и
// сохранённые числа разошлись бы с ним при первой же правке. nil — тип или тир
// из данных пропали, или зарядов не осталось.
func (g *Game) restoreSkill(sk *save.Skill) *mob.Stolen {
	if sk == nil || sk.Type == "" || sk.Left <= 0 {
		return nil
	}
	s := g.es.Power(sk.Type, sk.Tier)
	if s == nil {
		return nil
	}
	s.Charges = min(sk.Left, s.Charges)
	return s
}

// restoreWorld возвращает в мир его население и брошенные вещи. Мира, где никто
// не жил (новый персонаж или сохранение до этой возможности), это не касается —
// такой заселяется обычным порядком, спавнерами.
func (g *Game) restoreWorld() {
	c := g.char
	if len(c.Beasts) == 0 && len(c.Foes) == 0 {
		g.sp.Populate()
		g.es.Populate(false)
		return
	}
	for _, b := range c.Beasts {
		g.sp.RestoreAnimal(b.Species, engine.Vec2{X: b.Pos[0], Y: b.Pos[1]}, b.Floor, b.HP)
	}
	for _, f := range c.Foes {
		g.es.RestoreEnemy(f.Type, f.Tier, engine.Vec2{X: f.Pos[0], Y: f.Pos[1]}, f.Floor, f.HP, f.Elite)
	}
	for _, d := range c.Ground {
		g.dropSaved(d)
	}
	for i, sk := range c.Skills {
		if i >= skillSlots {
			break // ячеек стало меньше: лишние умения теряются, как лишние вещи
		}
		g.skills.slots[i] = g.restoreSkill(&sk)
	}
}

// dropSaved кладёт обратно вещь, оставленную на земле. Она ложится сразу и
// насовсем: прыжок с трупа отыгран в прошлом забеге, а подбирать её можно с
// первого же тика — герой мог выйти из игры, стоя прямо над ней.
func (g *Game) dropSaved(d save.Drop) {
	if d.ID == "" || d.N <= 0 {
		return
	}
	// Камень умения, чья сила не восстановилась (тип пропал из данных), — это
	// пустой камень: класть его обратно незачем, поднимать нечего.
	power := g.restoreSkill(d.Skill)
	if d.Skill != nil && power == nil {
		return
	}
	icon, err := g.items.Icon(g.l, d.ID)
	if err != nil {
		icon = nil // без иконки вещь всё равно лежит и подбирается
	}
	p := engine.Vec2{X: d.Pos[0], Y: d.Pos[1]}
	g.drops = append(g.drops, &groundItem{
		id: d.ID, n: d.N, icon: icon, power: power,
		pos: p, from: p, at: p, floor: d.Floor,
		age: lootLanded,
	})
}

// restore накладывает сохранённое на только что собранный забег. Порядок важен:
// сначала вещи, потом оружие (от него зависит пак анимаций), и только потом
// герой ставится на своё место.
func (g *Game) restore() {
	c := g.char
	if c == nil {
		return
	}
	g.prog = progress.Character{Level: max(c.Level, 1), XP: c.XP, Points: c.Points}

	// Вещи возвращаются по своим ячейкам, но сумка могла с тех пор ужаться (её
	// размер — разметка окна), а вещь — перестать надеваться. Ничего из этого не
	// повод потерять добычу: не влезшее в ячейку кладётся куда придётся, не
	// надевшееся — в сумку.
	for i, s := range c.Bag {
		if s.Empty() {
			continue
		}
		if i < g.bag.Size() {
			g.bag.Put(i, item.Slot{ID: s.ID, N: s.N})
			continue
		}
		g.bag.Add(s.ID, s.N)
	}
	for _, s := range c.Worn {
		if _, _, ok := g.eq.Wear(item.Slot{ID: s.ID, N: 1}); !ok {
			g.bag.Add(s.ID, 1)
		}
	}
	if err := g.applyLoadout(); err != nil {
		log.Println("сохранение:", err) // пака нет — герой останется безоружным
	}
	g.chest.restore(c.Chest)
	g.restoreWorld()

	// Точка выхода: нулевая означает «персонаж ещё не играл» — такой начинает со
	// старта карты. Place поправит место, если там теперь не поместиться.
	if c.Pos != [2]float64{} {
		g.pl.PlaceOn(g.m.Field(), engine.Vec2{X: c.Pos[0], Y: c.Pos[1]}, c.Floor)
	}
	if c.HP > 0 {
		g.pl.HP = min(c.HP, g.pl.MaxHP)
	}
	g.hp = g.pl.HP
}

// state — что в сундуке изменил игрок. Сам сундук (вид, место, первоначальная
// добыча) восстановится из сида, поэтому сохраняется только это.
func (c *chest) state() *save.Chest {
	if c == nil {
		return nil
	}
	st := &save.Chest{Opened: c.opened}
	for _, s := range c.inv.Slots() {
		st.Slots = append(st.Slots, save.Slot{ID: s.ID, N: s.N})
	}
	return st
}

// restore возвращает сундуку сохранённое содержимое и вид: разграбленный
// сундук обязан остаться разграбленным и с поднятой крышкой.
func (c *chest) restore(st *save.Chest) {
	if c == nil || st == nil {
		return
	}
	for i := range c.inv.Size() {
		c.inv.Take(i)
	}
	for i, s := range st.Slots {
		c.inv.Put(i, item.Slot{ID: s.ID, N: s.N})
	}
	if st.Opened {
		c.opened, c.opening = true, false
		c.frame = max(c.frames-1, 0)
	}
}
