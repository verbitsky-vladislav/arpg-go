package scene

import (
	"fmt"
	"image/color"
	"math"
	"sort"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/vladislav/game/internal/anim"
	"github.com/vladislav/game/internal/assets"
	"github.com/vladislav/game/internal/character"
	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/item"
	"github.com/vladislav/game/internal/mob"
	"github.com/vladislav/game/internal/progress"
	"github.com/vladislav/game/internal/sprite"
	"github.com/vladislav/game/internal/ui"
)

// Геометрия книги (логическое разрешение 640×360). Разворот: две страницы
// по бокам от корешка, закладки разделов торчат из левого и правого обрезов.
const (
	bookX, bookY = 30, 12
	bookW, bookH = 580, 336
	coverPad     = 7  // поля обложки вокруг бумаги
	spineW       = 8  // корешок между страницами
	pagePad      = 8  // поля страницы вокруг содержимого
	headH        = 26 // шапка страницы (название раздела)
	footH        = 26 // подвал книги (листалка)
	tabW, tabH   = 22, 104
	tabGap       = 16

	paperX = bookX + coverPad
	paperY = bookY + coverPad
	paperW = bookW - 2*coverPad
	paperH = bookH - 2*coverPad
	pageW  = (paperW - spineW) / 2
)

// Цвета: тёмный стол, кожаная обложка, желтоватая бумага, коричневые чернила.
var (
	bkDesk     = color.RGBA{0x0b, 0x0e, 0x18, 0xff}
	bkCover    = color.RGBA{0x5a, 0x3a, 0x24, 0xff}
	bkCoverHi  = color.RGBA{0x7d, 0x54, 0x33, 0xff}
	bkCoverLo  = color.RGBA{0x2e, 0x1c, 0x11, 0xff}
	bkPaper    = color.RGBA{0xe8, 0xda, 0xb6, 0xff}
	bkPaperDim = color.RGBA{0xd2, 0xc0, 0x95, 0xff}
	bkFrame    = color.RGBA{0xa8, 0x8e, 0x63, 0xff}
	bkInk      = color.RGBA{0x4a, 0x38, 0x26, 0xff}
	bkInkDim   = color.RGBA{0x8a, 0x76, 0x5a, 0xff}
	bkTabOn    = color.RGBA{0xd8, 0xae, 0x54, 0xff}
	bkTabOff   = color.RGBA{0x6e, 0x4a, 0x2e, 0xff}
)

// bestEntry — одна карточка книги: существо со своей анимацией ходьбы.
type bestEntry struct {
	title string   // подпись в рамке
	sub   string   // вторая строка (мелким)
	facts []string // строки-характеристики (подробный разворот)
	note  string   // примечание из данных — абзацем под характеристиками
	pack  *sprite.Pack
	play  *anim.Player
	refH  int    // высота, по которой подбирается масштаб (см. drawFrameFit)
	err   string // пак не загрузился — карточка пустая с пометкой
}

// bestSection — раздел книги (закладка). Загружается лениво, при первом
// открытии: входить в бестиарий не должно стоить чтения всех паков сразу.
type bestSection struct {
	title   string
	side    int                          // 0 — закладка слева, 1 — справа
	perPage int                          // существ на страницу (у персонажа — 1)
	note    string                       // что писать на пустой странице
	load    func() ([]bestEntry, string) // ленивая загрузка; вторая строка — причина пустоты

	entries []bestEntry
	loaded  bool
	spread  int // текущий разворот
	detail  int // открытая карточка (-1 — обычный разворот с сеткой)
}

func (s *bestSection) ensure() {
	if s.loaded || s.load == nil {
		return
	}
	s.loaded = true
	s.entries, s.note = s.load()
}

// perSpread — сколько существ помещается на развороте.
func (s *bestSection) perSpread() int { return s.perPage * 2 }

// spreads — число разворотов (минимум один, пусть и пустой).
func (s *bestSection) spreads() int {
	if len(s.entries) == 0 {
		return 1
	}
	return (len(s.entries) + s.perSpread() - 1) / s.perSpread()
}

// Bestiary — книга существ: разделы-закладки по бокам, листалка снизу,
// на развороте — карточки с анимацией ходьбы.
type Bestiary struct {
	back Scene // куда возвращаться по ESC
	secs []*bestSection
	cur  int
}

// NewBestiary собирает книгу. Данных может не быть (нет файла, битый пак) —
// это не ошибка сцены: раздел просто окажется пустым с пояснением на странице.
func NewBestiary(l *assets.Loader, back Scene) *Bestiary {
	return &Bestiary{
		back: back,
		secs: []*bestSection{
			{title: "ПЕРСОНАЖИ", side: 0, perPage: 1, detail: -1, load: func() ([]bestEntry, string) { return loadPersons(l) }},
			{title: "ЖИВОТНЫЕ", side: 0, perPage: 4, detail: -1, load: func() ([]bestEntry, string) { return loadAnimals(l) }},
			{title: "МОБЫ", side: 1, perPage: 4, detail: -1, load: func() ([]bestEntry, string) {
				return loadEnemies(l, enemiesRoot+"/enemies.json", enemiesRoot)
			}},
			{title: "БОССЫ", side: 1, perPage: 4, detail: -1, load: func() ([]bestEntry, string) {
				return loadEnemies(l, bossesRoot+"/bosses.json", bossesRoot)
			}},
		},
	}
}

// loadPersons — карточки персонажа: по одной на пару «тело × лоадаут».
func loadPersons(l *assets.Loader) ([]bestEntry, string) {
	cat, err := character.Load(l.FS(), "character/character.json")
	if err != nil {
		return nil, "НЕТ ДАННЫХ ПЕРСОНАЖА"
	}
	var out []bestEntry
	for _, bid := range cat.BodyIDs() {
		for _, lid := range cat.LoadoutIDs() {
			b, lo := cat.Body(bid), cat.Loadout(lid)
			facts := []string{
				fmt.Sprintf("ЗДОРОВЬЕ    %d", int(float64(cat.Base.HP)*b.HPScale)),
				fmt.Sprintf("ШАГ / БЕГ   %.0f / %.0f",
					cat.Base.Speed.Walk*b.SpeedScale*lo.SpeedScale,
					cat.Base.Speed.Run*b.SpeedScale*lo.SpeedScale),
			}
			if lo.CanStrike() {
				// Урон в карточке базовый: он свойство героя, а не лоадаута —
				// сколько снимет удар на самом деле, решает вещь в руке.
				facts = append(facts,
					fmt.Sprintf("УРОН        %s + ОРУЖИЕ", cat.Base.Damage),
					fmt.Sprintf("РАЗМАХ      %.0f ПКС / %.0f ГРАД", lo.Attack.Reach, lo.Attack.Arc),
					fmt.Sprintf("ЗАМАХ       %.2f С", float64(lo.Attack.SwingTicks)/config.TPS))
			} else {
				facts = append(facts, "ЭТИМ НЕ БЬЮТ")
			}
			e := bestEntry{title: b.Title.RU, sub: lo.Title.RU, facts: facts}
			p, err := character.LoadPack(l, "character", b, lo)
			if err != nil {
				e.err = "НЕТ СПРАЙТОВ"
			} else {
				// Рост меряем по кадру, а не по рамке пикселей: у пака sword
				// рамка выше на замах клинка, и герой из-за оружия ужимался бы.
				e.pack, e.refH = p, p.Frame.H
				e.play = anim.NewPlayer(walkClip(p))
			}
			out = append(out, e)
		}
	}
	return out, ""
}

// loadAnimals — карточки животных по таблице видов.
func loadAnimals(l *assets.Loader) ([]bestEntry, string) {
	cat, err := mob.LoadSpecies(l.FS(), animalsRoot+"/species.json")
	if err != nil {
		return nil, "НЕТ ТАБЛИЦЫ ВИДОВ"
	}
	items := itemCatalog(l)
	var out []bestEntry
	for _, id := range cat.IDs() {
		sp := cat.Get(id)
		kind := "ДОМАШНЕЕ"
		if sp.Wild() {
			kind = "ДИКОЕ"
		}
		e := bestEntry{
			title: sp.Title.RU,
			sub:   fmt.Sprintf("%s  УР %d  HP %d", kind, sp.Level(), sp.Stats.HP),
			facts: speciesFacts(sp, cat, items),
			note:  sp.Notes,
		}
		p, err := sprite.Load(l, animalsRoot+"/"+sp.Art)
		if err != nil {
			e.err = "НЕТ СПРАЙТОВ"
		} else {
			e.pack, e.refH = p, p.Bounds().H
			e.play = anim.NewPlayer(walkClip(p))
		}
		out = append(out, e)
	}
	return out, ""
}

// loadEnemies — карточки врагов. Боссы грузятся этой же функцией: таблицы у
// них одного формата, и книге незачем знать, чем босс отличается от моба.
func loadEnemies(l *assets.Loader, file, root string) ([]bestEntry, string) {
	cat, err := mob.LoadEnemies(l.FS(), file)
	if err != nil {
		return nil, "НЕТ ТАБЛИЦЫ"
	}
	items := itemCatalog(l)
	var out []bestEntry
	for _, id := range cat.IDs() {
		tier := cat.Get(id)
		ty := tier.Type
		e := bestEntry{
			title: tier.Title.RU,
			sub:   fmt.Sprintf("%s  УР %d  HP %d", word(bestFamily, ty.Family), tier.Level(), tier.HP),
			facts: enemyFacts(tier, items),
			note:  ty.Notes,
		}
		p, err := sprite.Load(l, tier.PackDir(root))
		if err != nil {
			e.err = "НЕТ СПРАЙТОВ"
		} else {
			e.pack, e.refH = p, p.Bounds().H
			e.play = anim.NewPlayer(walkClip(p))
		}
		out = append(out, e)
	}
	return out, ""
}

// enemyFacts — строки статьи о враге. Как и у животных, ничего не выдумывает:
// всё это лежит в enemies.json. Единственное, чего нет у зверей, — блок силы:
// ради него игрок и полезет в книгу перед охотой.
func enemyFacts(t *mob.Tier, items *item.Catalog) []string {
	ty := t.Type
	f := []string{
		fmt.Sprintf("РАЗМЕР      %s", word(bestSize, ty.SizeClass)),
		fmt.Sprintf("УРОН        %d, ДОСТАЁТ ЗА %.0f", t.Damage, ty.Threat.Reach),
		fmt.Sprintf("ХАРАКТЕР    %s", word(bestTemper, ty.Temper)),
		fmt.Sprintf("АКТИВЕН     %s", word(bestActivity, ty.Activity)),
		fmt.Sprintf("СКОРОСТЬ    ШАГ %.0f, БЕГ %.0f", t.Speed.Walk, t.Speed.Run),
		fmt.Sprintf("ЗАМЕЧАЕТ ЗА %.0f", ty.Threat.Sight),
	}
	if ty.Locomotion.Air {
		f = append(f, "ЛЕТАЕТ      ВОДА И ОБРЫВЫ НЕ ПРЕГРАДА")
	}
	if !ty.Threat.Unprovoked {
		f = append(f, "ПЕРВЫМ НЕ НАПАДАЕТ")
	}
	if ty.Threat.FleeHP > 0 {
		f = append(f, fmt.Sprintf("ОТСТУПАЕТ   НИЖЕ %.0f%% ЗДОРОВЬЯ", ty.Threat.FleeHP*100))
	}
	if ty.Habitat.Group.Max > 1 {
		f = append(f, fmt.Sprintf("ДЕРЖИТСЯ    ГРУППОЙ %d-%d", ty.Habitat.Group.Min, ty.Habitat.Group.Max))
	}
	if len(ty.Habitat.Biomes) > 0 {
		f = append(f, "ВОДИТСЯ     "+words(bestBiome, byWeight(ty.Habitat.Biomes)))
	}
	if len(ty.Habitat.Zones) > 0 {
		f = append(f, "МЕСТА       "+words(bestZone, ty.Habitat.Zones))
	}
	// Первая фаза — обычное состояние, писать о ней нечего: интересны переходы.
	for _, ph := range ty.Phases[min(1, len(ty.Phases)):] {
		f = append(f, fmt.Sprintf("НИЖЕ %.0f%%    МЕНЯЕТ АТАКУ, СКОРОСТЬ ×%.2f",
			ph.AtHP*100, ph.SpeedScale))
	}
	f = append(f, fmt.Sprintf("ОПЫТ        %d-%d (ДО %d УР.)",
		t.XP.Min, t.XP.Max, t.Level()+progress.OutlevelGap-1))
	if len(ty.Drops) > 0 {
		out := make([]string, 0, len(ty.Drops))
		for _, d := range ty.Drops {
			out = append(out, fmt.Sprintf("%s %d%%", items.Name(d.ID), int(d.Chance*100)))
		}
		f = append(f, "ДОБЫЧА      "+strings.Join(out, ", "))
	}
	if p := ty.Power; p != nil {
		// Урон в карточке — как у самого слабого тира: с кого сняли, тем сила и
		// крепче, а заряды считаются обратно её крепости (mob.PowerCharges).
		dmg := t.StolenDamage()
		f = append(f,
			fmt.Sprintf("СИЛА        %s, УРОН %d", word(bestElement, p.Element), dmg),
			fmt.Sprintf("            РАЗМАХ %.0f ПКС / %.0f ГРАД", p.Attack.Reach, p.Attack.Arc),
			fmt.Sprintf("ОТДАЁТ ЕЁ   С ШАНСОМ %d%%, ЗАРЯДОВ %d",
				int(p.StealChance*100), mob.PowerCharges(dmg)))
	} else {
		f = append(f, "СИЛЫ НЕ ОСТАВЛЯЕТ")
	}
	return f
}

// byWeight — биомы от частого к редкому: в книге сначала должно стоять то
// место, где эту тварь встретишь скорее всего.
func byWeight(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		if m[out[i]] != m[out[j]] {
			return m[out[i]] > m[out[j]]
		}
		return out[i] < out[j]
	})
	return out
}

// Словарь значений species.json и enemies.json. Английские ключи — это данные,
// а книга — текст для человека, поэтому перевод живёт здесь, а не в таблицах.
var (
	bestTemper = map[string]string{
		"passive": "СПОКОЙНЫЙ", "skittish": "ПУГЛИВЫЙ", "territorial": "ТЕРРИТОРИАЛЬНЫЙ",
		"predator": "ХИЩНИК", "tame": "РУЧНОЙ",
		"aggressive": "АГРЕССИВНЫЙ", "ambusher": "ИЗ ЗАСАДЫ", "ranged": "БЬЁТ ИЗДАЛИ",
	}
	bestFamily = map[string]string{
		"undead": "НЕЖИТЬ", "demon": "ДЕМОН", "spirit": "ДУХ", "beast": "ЗВЕРЬ",
		"humanoid": "ГУМАНОИД", "plant": "РАСТЕНИЕ", "fungus": "ГРИБ",
		"construct": "ГОЛЕМ", "ooze": "СЛИЗЬ", "aberration": "ТВАРЬ",
	}
	bestElement = map[string]string{
		"fire": "ОГОНЬ", "magic": "МАГИЯ", "smoke": "ДЫМ",
		"spectral": "ПРИЗРАЧНАЯ", "slash": "КЛИНОК",
	}
	bestBiome = map[string]string{
		"cave": "ПЕЩЕРЫ", "cave_glowing": "СВЕТЯЩИЕСЯ ПЕЩЕРЫ", "cursed_area": "ПРОКЛЯТЫЕ ЗЕМЛИ",
		"dead_island": "МЁРТВЫЙ ОСТРОВ", "desert": "ПУСТЫНЯ", "dungeon_1": "ПОДЗЕМЕЛЬЕ I",
		"dungeon_2": "ПОДЗЕМЕЛЬЕ II", "dungeon_3": "ПОДЗЕМЕЛЬЕ III", "farm": "ФЕРМА",
		"fishing-docks": "ПРИСТАНЬ", "flying_islands": "ЛЕТУЧИЕ ОСТРОВА", "forest": "ЛЕС",
		"glades": "ПОЛЯНЫ", "rocky": "СКАЛЫ", "ruined_tample": "РУИНЫ ХРАМА",
		"sewer": "КАНАЛИЗАЦИЯ", "swamp": "БОЛОТО", "winterland": "ЗИМНИЕ ЗЕМЛИ",
	}
	bestActivity = map[string]string{"day": "ДНЁМ", "night": "НОЧЬЮ", "any": "ЛЮБОЕ ВРЕМЯ"}
	bestSize     = map[string]string{
		"tiny": "КРОШЕЧНЫЙ", "small": "МЕЛКИЙ", "medium": "СРЕДНИЙ", "large": "КРУПНЫЙ",
	}
	bestZone = map[string]string{
		"water": "ВОДА", "shore": "БЕРЕГ", "meadow": "ЛУГ", "woods": "ЛЕС",
		"trail": "ТРОПА", "plateau": "ПЛАТО", "farmyard": "ДВОР", "indoor": "ПОД КРЫШЕЙ",
	}
)

// itemsFile — каталог предметов: имена добычи берутся оттуда, а не из словаря
// рядом. Идентификаторы вроде raw_meat раздают таблицы мобов, и объяснять, что
// это такое, должен один файл на всю игру.
const itemsFile = "items/items.json"

// itemCatalog — каталог для подписей. Книга не должна падать из-за него: без
// каталога добыча покажется сырыми идентификаторами, и это сразу видно.
func itemCatalog(l *assets.Loader) *item.Catalog {
	c, err := item.Load(l.FS(), itemsFile)
	if err != nil {
		return nil
	}
	return c
}

// word переводит значение по словарю, оставляя как есть незнакомое: новое
// значение в данных должно быть видно, а не потеряно.
func word(dict map[string]string, key string) string {
	if v, ok := dict[key]; ok {
		return v
	}
	return strings.ToUpper(key)
}

func words(dict map[string]string, keys []string) string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, word(dict, k))
	}
	return strings.Join(out, ", ")
}

// speciesFacts — строки статьи о виде. Ничего не выдумывает: всё это лежит в
// species.json, просто никогда не показывалось игроку.
func speciesFacts(s *mob.Species, cat *mob.Catalog, items *item.Catalog) []string {
	name := func(id string) string {
		if o := cat.Get(id); o != nil {
			return o.Title.RU
		}
		return id
	}
	names := func(ids []string) string {
		out := make([]string, 0, len(ids))
		for _, id := range ids {
			out = append(out, name(id))
		}
		return strings.Join(out, ", ")
	}

	f := []string{
		fmt.Sprintf("РАЗМЕР      %s", word(bestSize, s.SizeClass)),
		fmt.Sprintf("ЗДОРОВЬЕ    %d", s.Stats.HP),
		fmt.Sprintf("ХАРАКТЕР    %s", word(bestTemper, s.Temper)),
		fmt.Sprintf("АКТИВЕН     %s", word(bestActivity, s.Activity)),
		fmt.Sprintf("СКОРОСТЬ    ШАГ %.0f", s.Stats.Speed.Walk),
	}
	if s.Stats.Speed.Run > 0 {
		f[len(f)-1] += fmt.Sprintf(", БЕГ %.0f", s.Stats.Speed.Run)
	}
	if s.Locomotion.Water {
		f = append(f, "ПЛАВАЕТ")
	}
	if len(s.Habitat.Zones) > 0 {
		f = append(f, "ВОДИТСЯ     "+words(bestZone, s.Habitat.Zones))
	}
	if s.Habitat.NeedsSettlement {
		f = append(f, "ЖИВЁТ ТОЛЬКО ВО ДВОРЕ")
	}
	if s.Spawn.Group.Max > 1 {
		f = append(f, fmt.Sprintf("ДЕРЖИТСЯ    ГРУППОЙ %d-%d", s.Spawn.Group.Min, s.Spawn.Group.Max))
	}
	if s.Threat.Attacks {
		f = append(f, fmt.Sprintf("НАПАДАЕТ    УРОН %d, ЗАМЕЧАЕТ ЗА %.0f",
			s.Threat.Damage, s.Threat.Sight))
	} else {
		f = append(f, "НЕ НАПАДАЕТ")
	}
	if len(s.Threat.Prey) > 0 {
		f = append(f, "ОХОТИТСЯ НА "+names(s.Threat.Prey))
	}
	f = append(f, fmt.Sprintf("ОПЫТ        %d-%d (ДО %d УР.)",
		s.XP.Min, s.XP.Max, s.Level()+progress.OutlevelGap-1))
	if s.GrowsInto != "" {
		f = append(f, "ВЫРАСТАЕТ В "+name(s.GrowsInto))
	}
	if s.YoungForm != "" {
		f = append(f, "ДЕТЁНЫШ     "+name(s.YoungForm))
	}
	if s.Use.Tameable {
		f = append(f, "ПРИРУЧАЕТСЯ")
	}
	if s.Use.Rideable {
		f = append(f, "ЕЗДОВОЕ")
	}
	if len(s.Use.Products) > 0 {
		f = append(f, "ДАЁТ        "+items.Names(s.Use.Products))
	}
	if len(s.Drops) > 0 {
		out := make([]string, 0, len(s.Drops))
		for _, d := range s.Drops {
			out = append(out, fmt.Sprintf("%s %d%%", items.Name(d.ID), int(d.Chance*100)))
		}
		f = append(f, "ДОБЫЧА      "+strings.Join(out, ", "))
	}
	return f
}

// walkClip — клип ходьбы лицом к читателю. Если ходьбы в паке нет (птица,
// рыба, недоделанный пак) — стойка, а нет и её — первая попавшаяся анимация:
// пустая рамка в книге хуже, чем не та анимация.
func walkClip(p *sprite.Pack) *anim.Clip {
	for _, name := range []string{"walk", "idle"} {
		if c := p.Clip(name, sprite.Down); c.Valid() {
			return c
		}
	}
	for _, name := range p.Anims() {
		if c := p.Clip(name, sprite.Down); c.Valid() {
			return c
		}
	}
	return nil
}

func (b *Bestiary) section() *bestSection { return b.secs[b.cur] }

func (b *Bestiary) Update() (Scene, error) {
	sec := b.section()
	sec.ensure()
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		uiCancel()
		if sec.detail >= 0 {
			sec.detail = -1 // из статьи о виде — обратно к сетке
			return b, nil
		}
		if b.back != nil {
			return b.back, nil
		}
		return NewMenu(), nil
	}

	// Закладки: мышь по всему ярлыку, клавиши — перебор разделов по кругу.
	mx, my := ebiten.CursorPosition()
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		for i := range b.secs {
			if x, y, w, h := b.tabRect(i); inRect(mx, my, x, y, w, h) {
				b.cur = i
				b.section().ensure()
			}
		}
	}
	if keyPressed(ebiten.KeyDown, ebiten.KeyS) {
		b.selectStep(1)
	}
	if keyPressed(ebiten.KeyUp, ebiten.KeyW) {
		b.selectStep(-1)
	}

	// Листалка: стрелки, колесо и кнопки в подвале.
	turn := 0
	if keyPressed(ebiten.KeyRight, ebiten.KeyD) {
		turn++
	}
	if keyPressed(ebiten.KeyLeft, ebiten.KeyA) {
		turn--
	}
	if _, wy := ebiten.Wheel(); wy != 0 {
		turn -= int(math.Copysign(1, wy))
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		x, y, w, h := arrowRect(-1)
		if inRect(mx, my, x, y, w, h) {
			turn--
		}
		if x, y, w, h := arrowRect(1); inRect(mx, my, x, y, w, h) {
			turn++
		}
	}
	sec = b.section()
	if sec.detail >= 0 {
		sec.detail = clampInt(sec.detail+turn, 0, len(sec.entries)-1)
		sec.spread = sec.detail / sec.perSpread() // вернёмся на страницу с этим видом
	} else {
		sec.spread = clampInt(sec.spread+turn, 0, sec.spreads()-1)
	}

	// Клик по карточке открывает статью о виде: в данных про него гораздо
	// больше, чем помещается в рамку.
	if sec.detail < 0 && sec.perPage > 1 && inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		if i := b.cardAt(sec, mx, my); i >= 0 {
			sec.detail = i
		}
	}

	// Крутим только то, что видно на развороте.
	for _, e := range b.visible() {
		if e.play == nil {
			continue
		}
		e.play.Update()
		if e.play.Finished() { // незацикленный клип (idle без loop) — по кругу
			e.play.Play(e.play.Clip())
		}
	}
	return b, nil
}

// cardAt — номер карточки под точкой (mx,my) на текущем развороте; -1, если
// курсор мимо.
func (b *Bestiary) cardAt(sec *bestSection, mx, my int) int {
	from := sec.spread * sec.perSpread()
	for page := range 2 {
		for i := range sec.perPage {
			n := from + page*sec.perPage + i
			if n >= len(sec.entries) {
				return -1
			}
			if x, y, w, h := cellRect(page, i, sec.perPage); inRect(mx, my, x, y, w, h) {
				return n
			}
		}
	}
	return -1
}

// selectStep переводит выбор на соседний раздел (по кругу).
func (b *Bestiary) selectStep(d int) {
	b.cur = ((b.cur+d)%len(b.secs) + len(b.secs)) % len(b.secs)
	b.section().ensure()
}

// visible — карточки текущего разворота.
func (b *Bestiary) visible() []*bestEntry {
	sec := b.section()
	if sec.detail >= 0 && sec.detail < len(sec.entries) {
		return []*bestEntry{&sec.entries[sec.detail]}
	}
	from := sec.spread * sec.perSpread()
	var out []*bestEntry
	for i := from; i < from+sec.perSpread() && i < len(sec.entries); i++ {
		out = append(out, &sec.entries[i])
	}
	return out
}

func (b *Bestiary) Draw(screen *ebiten.Image) {
	screen.Fill(bkDesk)
	drawBook(screen)

	sec := b.section()
	for page := range 2 {
		b.drawPage(screen, sec, page)
	}
	b.drawFooter(screen, sec)

	for i, s := range b.secs {
		b.drawTab(screen, i, s)
	}
}

// drawBook — обложка, обрез страниц и корешок.
func drawBook(dst *ebiten.Image) {
	vector.FillRect(dst, bookX, bookY, bookW, bookH, bkCover, false)
	vector.StrokeRect(dst, bookX+0.5, bookY+0.5, bookW-1, bookH-1, 1, bkCoverLo, false)
	vector.StrokeRect(dst, bookX+2.5, bookY+2.5, bookW-5, bookH-5, 1, bkCoverHi, false)

	for page := range 2 {
		x := float32(pageX(page))
		vector.FillRect(dst, x, paperY, pageW, paperH, bkPaper, false)
		// Тень у корешка: страница уходит в сгиб.
		if page == 0 {
			vector.FillRect(dst, x+pageW-4, paperY, 4, paperH, bkPaperDim, false)
		} else {
			vector.FillRect(dst, x, paperY, 4, paperH, bkPaperDim, false)
		}
	}
	cx := float32(bookX + bookW/2)
	vector.FillRect(dst, cx-spineW/2, paperY, spineW, paperH, bkCoverLo, false)
	vector.FillRect(dst, cx-1, paperY, 2, paperH, bkCover, false)
}

// pageX — левый край страницы (0 — левая, 1 — правая).
func pageX(page int) float64 {
	if page == 0 {
		return paperX
	}
	return paperX + pageW + spineW
}

// drawPage рисует шапку страницы и её карточки.
func (b *Bestiary) drawPage(dst *ebiten.Image, sec *bestSection, page int) {
	px := float32(pageX(page))
	cx := float64(px) + pageW/2

	ui.PixelTextCentered(dst, sec.title, cx, paperY+5, 2, bkInk)
	vector.FillRect(dst, px+pagePad, paperY+headH-4, pageW-2*pagePad, 1, bkFrame, false)

	if sec.detail >= 0 && sec.detail < len(sec.entries) {
		b.drawArticle(dst, &sec.entries[sec.detail], page)
		return
	}

	from := sec.spread*sec.perSpread() + page*sec.perPage
	// Раздел закрыт или пуст — вместо карточек пояснение по центру страницы.
	if len(sec.entries) == 0 {
		note := sec.note
		if note == "" {
			note = "ПУСТО"
		}
		ui.PixelTextCentered(dst, note, cx, paperY+paperH/2-7, 2, bkInkDim)
		return
	}

	for i := range sec.perPage {
		if from+i >= len(sec.entries) {
			return
		}
		x, y, w, h := cellRect(page, i, sec.perPage)
		drawCard(dst, &sec.entries[from+i], x, y, w, h, sec.perPage == 1, sec.perPage == 1)
	}
}

// drawArticle — разворот про один вид: слева портрет, справа всё, что про него
// известно из данных. Затем и книга, чтобы не пересказывать species.json
// вручную.
func (b *Bestiary) drawArticle(dst *ebiten.Image, e *bestEntry, page int) {
	x, y, w, h := cellRect(page, 0, 1)
	if page == 0 {
		drawCard(dst, e, x, y, w, h, true, false)
		return
	}

	vector.FillRect(dst, x, y, w, h, bkPaper, false)
	vector.StrokeRect(dst, x+0.5, y+0.5, w-1, h-1, 1, bkFrame, false)
	vector.StrokeRect(dst, x+2.5, y+2.5, w-5, h-5, 1, bkPaperDim, false)

	ty := float64(y) + 12
	ui.PixelTextCentered(dst, "ЧТО ИЗВЕСТНО", float64(x+w/2), ty, 2, bkInk)
	vector.FillRect(dst, x+20, y+30, w-40, 1, bkFrame, false)

	ty = float64(y) + 40
	for _, f := range e.facts {
		ui.PixelText(dst, f, float64(x)+14, ty, 1, bkInk)
		ty += 10
	}
	if e.note == "" {
		return
	}
	ty += 6
	for _, line := range wrapPixel(e.note, float64(w)-28, 1) {
		ui.PixelText(dst, line, float64(x)+14, ty, 1, bkInkDim)
		ty += 9
	}
}

// wrapPixel режет текст на строки, влезающие в ширину maxW пиксельным шрифтом.
func wrapPixel(s string, maxW, scale float64) []string {
	var out []string
	line := ""
	for _, word := range strings.Fields(s) {
		try := word
		if line != "" {
			try = line + " " + word
		}
		if ui.PixelTextWidth(try, scale) > maxW && line != "" {
			out = append(out, line)
			line = word
			continue
		}
		line = try
	}
	if line != "" {
		out = append(out, line)
	}
	return out
}

// cellRect — рамка i-й карточки на странице page (сетка 2×2, либо одна на всю
// страницу).
func cellRect(page, i, perPage int) (x, y, w, h float32) {
	ix := float32(pageX(page)) + pagePad
	iy := float32(paperY + headH)
	iw := float32(pageW - 2*pagePad)
	ih := float32(paperH - headH - footH)
	if perPage == 1 {
		return ix, iy, iw, ih
	}
	w, h = (iw-6)/2, (ih-6)/2
	return ix + float32(i%2)*(w+6), iy + float32(i/2)*(h+6), w, h
}

// drawCard — карточка существа: рамка, кадр анимации и подписи. big — карточка
// на всю страницу (персонаж, портрет статьи); facts — печатать ли под портретом
// характеристики (в сетке 2×2 они не поместятся, а в статье живут на соседней
// странице).
func drawCard(dst *ebiten.Image, e *bestEntry, x, y, w, h float32, big, facts bool) {
	vector.FillRect(dst, x, y, w, h, bkPaper, false)
	vector.StrokeRect(dst, x+0.5, y+0.5, w-1, h-1, 1, bkFrame, false)
	vector.StrokeRect(dst, x+2.5, y+2.5, w-5, h-5, 1, bkPaperDim, false)

	nameScale, subScale := 2.0, 1.0
	nameY := float64(y+h) - 27 // ниже — вторая строка, обе внутри рамки
	if big {
		nameScale, subScale = 3.0, 2.0
		nameY = float64(y) + 10
	}

	// Портрет: «пол» карточки — над подписями (или под ними на большой странице).
	base := nameY - 6
	top := float64(y) + 6
	if big {
		top = nameY + 34
		base = float64(y+h) - 16
		if facts {
			base -= float64(len(e.facts)) * 10
		}
	}
	// Рост, к которому приводятся все существа: кадры в паках от 16 до 64 px,
	// и без общей цели цыплёнок вышел бы с буйвола.
	target := 52.0
	if big {
		target = 190
	}
	if e.err != "" {
		ui.PixelTextCentered(dst, e.err, float64(x+w/2), (top+base)/2, 1, bkInkDim)
	} else if img := e.play.Frame(); img != nil {
		drawFrameFit(dst, img, e.pack.Bounds(), e.refH, float64(x+w/2), base, target, float64(w)-14, base-top)
	}

	ui.PixelTextCentered(dst, ui.PixelTextFit(e.title, float64(w)-10, nameScale), float64(x+w/2), nameY, nameScale, bkInk)
	if e.sub != "" {
		ui.PixelTextCentered(dst, ui.PixelTextFit(e.sub, float64(w)-10, subScale),
			float64(x+w/2), nameY+ui.PixelTextHeight(nameScale)+3, subScale, bkInkDim)
	}
	if big {
		vector.FillRect(dst, x+20, y+52, w-40, 1, bkFrame, false)
	}
	if !facts {
		return
	}
	for i, f := range e.facts {
		ui.PixelText(dst, f, float64(x)+14, float64(y+h)-float64(len(e.facts)-i)*10-6, 1, bkInk)
	}
}

// drawFrameFit рисует кадр целым масштабом: середина непрозрачной части кадра
// приходится на cx, низ — на base, а высота refH стремится к targetH. Дробный масштаб
// испортил бы пиксели, поэтому берётся ближайший целый, при котором существо
// ещё влезает в рамку.
func drawFrameFit(dst *ebiten.Image, img *ebiten.Image, bb sprite.Rect, refH int, cx, base, targetH, maxW, maxH float64) {
	bw, bh := float64(max(bb.W, 1)), float64(max(bb.H, 1))
	sc := math.Round(targetH / float64(max(refH, 1)))
	sc = math.Max(1, math.Min(4, sc))
	for sc > 1 && (bw*sc > maxW || bh*sc > maxH) {
		sc--
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(sc, sc)
	op.GeoM.Translate(cx-(float64(bb.X)+bw/2)*sc, base-float64(bb.Y+bb.H)*sc)
	dst.DrawImage(img, op)
}

// drawFooter — листалка и подсказки в подвале книги.
func (b *Bestiary) drawFooter(dst *ebiten.Image, sec *bestSection) {
	y := float32(paperY + paperH - footH + 4)
	for _, d := range []int{-1, 1} {
		x, ay, w, h := arrowRect(d)
		cur, last := sec.spread, sec.spreads()-1
		if sec.detail >= 0 {
			cur, last = sec.detail, len(sec.entries)-1
		}
		on := (d < 0 && cur > 0) || (d > 0 && cur < last)
		col, ink := bkPaperDim, bkInkDim
		if on {
			col, ink = bkTabOn, bkInk
		}
		vector.FillRect(dst, x, ay, w, h, col, false)
		vector.StrokeRect(dst, x+0.5, ay+0.5, w-1, h-1, 1, bkFrame, false)
		s := ">"
		if d < 0 {
			s = "<"
		}
		ui.PixelTextCentered(dst, s, float64(x+w/2), float64(ay)+4, 2, ink)
	}
	// Счётчик разворотов — на поле левой страницы: по центру его разрезал бы
	// корешок.
	counter := fmt.Sprintf("%d / %d", sec.spread+1, sec.spreads())
	hint := "ESC - НАЗАД,  СТРЕЛКИ - ЛИСТАТЬ,  КЛИК ПО КАРТОЧКЕ - ПОДРОБНО"
	if sec.detail >= 0 {
		counter = fmt.Sprintf("%d / %d", sec.detail+1, len(sec.entries))
		hint = "ESC - К СПИСКУ,  СТРЕЛКИ - СЛЕДУЮЩИЙ ВИД"
	}
	ui.PixelText(dst, counter, pageX(0)+pagePad, float64(y)+5, 1, bkInkDim)
	ui.PixelTextCentered(dst, hint, config.ScreenW/2, config.ScreenH-9, 1, bkFrame)
}

// arrowRect — кнопка листалки: d<0 — назад, d>0 — вперёд.
func arrowRect(d int) (x, y, w, h float32) {
	const aw, ah = 20, 16
	cx := float32(bookX + bookW/2)
	y = float32(paperY + paperH - footH + 2)
	if d < 0 {
		return cx - 44 - aw, y, aw, ah
	}
	return cx + 44, y, aw, ah
}

// tabRect — ярлык закладки i-го раздела. Разделы делятся по обрезам книги:
// side 0 — левый, side 1 — правый; порядок внутри стороны — сверху вниз.
func (b *Bestiary) tabRect(i int) (x, y, w, h float32) {
	s := b.secs[i]
	n := 0 // номер закладки на своей стороне
	for j := range i {
		if b.secs[j].side == s.side {
			n++
		}
	}
	total := 0
	for _, o := range b.secs {
		if o.side == s.side {
			total++
		}
	}
	top := float32(bookY) + (bookH-float32(total*tabH+(total-1)*tabGap))/2
	y = top + float32(n)*(tabH+tabGap)
	x = float32(bookX - tabW + 4)
	if s.side == 1 {
		x = float32(bookX + bookW - 4)
	}
	return x, y, tabW, tabH
}

// drawTab рисует закладку: выбранная выезжает наружу, закрытая — тусклая.
func (b *Bestiary) drawTab(dst *ebiten.Image, i int, s *bestSection) {
	x, y, w, h := b.tabRect(i)
	sel := i == b.cur
	col, ink := bkTabOff, bkPaper
	if sel {
		col, ink = bkTabOn, bkCoverLo
	}
	if sel { // выбранная закладка торчит из книги сильнее
		if s.side == 0 {
			x -= 3
		}
		w += 3
	}
	vector.FillRect(dst, x, y, w, h, col, false)
	vector.StrokeRect(dst, x+0.5, y+0.5, w-1, h-1, 1, bkCoverLo, false)

	label := s.title
	tx := float64(x) + (float64(w)-ui.PixelTextHeight(1))/2
	ty := float64(y+h)/1 - (float64(h)-ui.PixelTextWidth(label, 1))/2
	ui.PixelTextRot90(dst, label, tx, ty, 1, ink)
}

func inRect(mx, my int, x, y, w, h float32) bool {
	fx, fy := float32(mx), float32(my)
	return fx >= x && fx < x+w && fy >= y && fy < y+h
}

func clampInt(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	return min(max(v, lo), hi)
}
