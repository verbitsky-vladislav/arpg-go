package scene

// Сохранение забега: снимок игры в save.Char и обратная сборка из него.
//
// Что именно переживает выход, решается здесь, и правило одно: сохраняется то,
// что игрок нажил, а не то, что игра умеет пересчитать. Карта, звери, враги и
// содержимое сундука восстанавливаются из сида — они одинаковы при каждой
// сборке того же мира. Нажитое — уровень, опыт, добыча в сумке, надетое,
// счётчики убитых, время игры — восстановить неоткуда, поэтому оно и лежит в
// файле (см. internal/save).
//
// Что НЕ сохраняется намеренно: вещи, брошенные на землю и не подобранные
// (g.drops), живые мобы и их здоровье, недоигранные эффекты. Мир пересобирается
// заново, и попытка вернуть в него ровно тех же гоблинов на тех же местах
// стоила бы больше, чем даёт.

import (
	"log"
	"math/rand/v2"
	"time"

	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/engine"
	"github.com/vladislav/game/internal/item"
	"github.com/vladislav/game/internal/physics"
	"github.com/vladislav/game/internal/progress"
	"github.com/vladislav/game/internal/save"
)

// autosaveTicks — как часто забег сам ложится на диск. Полминуты: чаще —
// дёргать файл посреди боя, реже — терять слишком много при вылете.
const autosaveTicks = 30 * config.TPS

// NewChar заводит нового персонажа: имя, тело и свой собственный мир. Сид
// короткий — его видно в журнале, и по нему карта повторяется.
func NewChar(name, bodyID string) *save.Char {
	return &save.Char{
		Name:  save.CleanName(name),
		Body:  bodyID,
		Seed:  int64(rand.Uint64N(1_000_000)),
		Biome: gameBiome,
		Level: 1,
		Kills: map[string]int{},
		Worn:  map[string]save.Slot{},
	}
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
	c.Touch(time.Now())
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

	for i, s := range c.Bag {
		g.bag.Put(i, item.Slot{ID: s.ID, N: s.N})
	}
	for _, s := range c.Worn {
		g.eq.Wear(item.Slot{ID: s.ID, N: 1})
	}
	if err := g.applyLoadout(); err != nil {
		log.Println("сохранение:", err) // пака нет — герой останется безоружным
	}
	g.chest.restore(c.Chest)

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
