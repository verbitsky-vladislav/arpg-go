package scene

// Сундук на карте: стоит, ждёт героя, открывается анимацией и отдаёт добычу.
//
// Живёт в сцене, а не в mob: он ничего не решает сам, у него нет поведения —
// только состояние «закрыт / открывается / открыт» и содержимое.

import (
	"encoding/json"
	"fmt"
	"image/color"
	"io/fs"
	"math"
	"math/rand/v2"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/vladislav/game/internal/assets"
	"github.com/vladislav/game/internal/atlas"
	"github.com/vladislav/game/internal/audio"
	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/engine"
	"github.com/vladislav/game/internal/item"
	"github.com/vladislav/game/internal/physics"
	"github.com/vladislav/game/internal/settings"
	"github.com/vladislav/game/internal/ui"
	"github.com/vladislav/game/internal/world"
)

const (
	chestsFile = "items/chests.json"
	// chestReach — с какого расстояния сундук можно открыть. Чуть больше
	// радиуса тела: тянуться к сундуку вплотную неудобно.
	chestReach = 30
	// Сундук ставится в поле зрения героя: игрок должен увидеть его сразу, а
	// не искать по карте. Верхняя граница меньше половины высоты экрана
	// (360/2 = 180), поэтому сундук виден с точки старта в любую сторону, а не
	// только вбок, где экран шире. Ближняя граница — чтобы он не стоял у героя
	// под ногами.
	chestNear = 80
	chestFar  = 160
	// chestTries — сколько точек перебрать: на карте с водой и обрывами
	// годных мест меньше, чем кажется.
	chestTries = 600
)

// chestKind — вид сундука из chests.json.
type chestKind struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Pack   string `json:"pack"`
	Sprite string `json:"sprite"`
	MS     int    `json:"ms"`
	Loot   struct {
		// Always — что лежит в сундуке при любом броске.
		Always []struct {
			ID  string `json:"id"`
			Min int    `json:"min"`
			Max int    `json:"max"`
		} `json:"always"`
		Rolls   [2]int `json:"rolls"`
		Entries []struct {
			ID     string `json:"id"`
			Min    int    `json:"min"`
			Max    int    `json:"max"`
			Weight int    `json:"weight"`
		} `json:"entries"`
	} `json:"loot"`
}

type chestCatalog struct {
	Kinds []*chestKind `json:"kinds"`
}

func loadChests(fsys fs.FS, p string) (*chestCatalog, error) {
	b, err := fs.ReadFile(fsys, p)
	if err != nil {
		return nil, fmt.Errorf("scene: чтение %q: %w", p, err)
	}
	var c chestCatalog
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("scene: разбор %q: %w", p, err)
	}
	if len(c.Kinds) == 0 {
		return nil, fmt.Errorf("scene: %q: не описано ни одного сундука", p)
	}
	return &c, nil
}

// chest — сундук на карте.
type chest struct {
	kind *chestKind
	pos  engine.Vec2
	pack *atlas.Pack
	inv  *item.Inventory

	frames  int // кадров в анимации открывания
	frame   int
	tick    int
	opening bool
	opened  bool // крышка уже поднята

	pulse int  // счётчик для мерцания подсветки
	near  bool // герой рядом: подсветка ярче
}

// newChest готовит сундук: грузит его пак, катает добычу и ищет место рядом с
// героем. Если места не нашлось, возвращает nil без ошибки — забег без сундука
// лучше, чем забег, который не начался.
func newChest(l *assets.Loader, m *world.Map, cat *item.Catalog, slots int,
	from engine.Vec2, rng *rand.Rand) (*chest, error) {

	cc, err := loadChests(l.FS(), chestsFile)
	if err != nil {
		return nil, err
	}
	kind := cc.Kinds[0]
	pack, err := atlas.Load(l, kind.Pack)
	if err != nil {
		return nil, fmt.Errorf("scene: сундук %q: %w", kind.ID, err)
	}
	if _, ok := pack.Sprite(kind.Sprite); !ok {
		return nil, fmt.Errorf("scene: сундук %q: в паке %q нет спрайта %q", kind.ID, kind.Pack, kind.Sprite)
	}
	pos, ok := chestSpot(m, from, rng)
	if !ok {
		return nil, nil
	}
	c := &chest{
		kind:   kind,
		pos:    pos,
		pack:   pack,
		inv:    item.NewInventory(cat, slots),
		frames: chestFrames(pack, kind.Sprite),
	}
	c.fill(rng)
	return c, nil
}

// chestFrames — сколько кадров у открывания. Кадровку листа знает манифест
// пака (её вытащили из разметки Tiled), поэтому здесь её не задают.
func chestFrames(p *atlas.Pack, sprite string) int {
	for _, sh := range p.Sheets {
		for _, s := range sh.Sprites {
			if s.ID != sprite {
				continue
			}
			if sh.Anim != nil && sh.Anim.Frames > 1 {
				return sh.Anim.Frames
			}
			return 1
		}
	}
	return 1
}

// fill катает содержимое по таблице вида.
func (c *chest) fill(rng *rand.Rand) {
	lt := c.kind.Loot
	// Сначала обязательное: первый клинок должен найтись в сундуке всегда,
	// иначе герой останется безоружным и драться ему будет нечем.
	for _, e := range lt.Always {
		c.inv.Add(e.ID, amount(e.Min, e.Max, rng))
	}
	total := 0
	for _, e := range lt.Entries {
		if e.Weight > 0 {
			total += e.Weight
		}
	}
	if total == 0 || len(lt.Entries) == 0 {
		return
	}
	lo, hi := lt.Rolls[0], lt.Rolls[1]
	if hi < lo {
		lo, hi = hi, lo
	}
	rolls := lo
	if hi > lo {
		rolls += rng.IntN(hi - lo + 1)
	}
	for range rolls {
		r := rng.IntN(total)
		for _, e := range lt.Entries {
			if e.Weight <= 0 {
				continue
			}
			if r -= e.Weight; r >= 0 {
				continue
			}
			c.inv.Add(e.ID, amount(e.Min, e.Max, rng))
			break
		}
	}
}

// amount — сколько штук выпало по строке таблицы.
func amount(lo, hi int, rng *rand.Rand) int {
	if lo < 1 {
		lo = 1
	}
	if hi <= lo {
		return lo
	}
	return lo + rng.IntN(hi-lo+1)
}

// chestSpot ищет точку под сундук: рядом с героем, но за краем экрана, на
// нижнем этаже и на твёрдой земле.
//
// Плато исключается намеренно: туда ведёт единственная лестница, и сундук на
// макушке — это не «рядом», а «в обход через полкарты».
func chestSpot(m *world.Map, from engine.Vec2, rng *rand.Rand) (engine.Vec2, bool) {
	f := m.Field()
	w, h := m.Size()
	body := physics.Body{Radius: 10, Floor: physics.FloorLow}
	for range chestTries {
		a := rng.Float64() * 2 * math.Pi
		d := chestNear + rng.Float64()*(chestFar-chestNear)
		p := engine.Vec2{X: from.X + math.Cos(a)*d, Y: from.Y + math.Sin(a)*d}
		if p.X < 16 || p.Y < 16 || p.X >= w-16 || p.Y >= h-16 {
			continue
		}
		if f.CellAt(p) != physics.Ground || m.Zone(p) == "plateau" {
			continue
		}
		// Тело должно поместиться целиком: иначе сундук окажется наполовину в
		// обрыве, и подойти к нему будет нельзя.
		if !f.Fits(p, body) {
			continue
		}
		return p, true
	}
	return engine.Vec2{}, false
}

// nearPlayer — стоит ли герой достаточно близко, чтобы открыть сундук.
func (c *chest) nearPlayer(p engine.Vec2) bool {
	return c != nil && engine.Dist(c.pos, p) <= chestReach
}

// begin запускает открывание (или сразу говорит, что сундук уже открыт).
func (c *chest) begin() {
	if c.opened || c.opening {
		return
	}
	if c.frames <= 1 {
		c.opened = true
		return
	}
	c.opening, c.frame, c.tick = true, 0, 0
}

// update крутит анимацию открывания. Возвращает true в тот тик, когда крышка
// поднялась: сцене это нужно, чтобы показать содержимое ровно тогда.
func (c *chest) update() bool {
	if c == nil || !c.opening {
		return false
	}
	ms := c.kind.MS
	if ms <= 0 {
		ms = 120
	}
	c.tick++
	if c.tick*1000 < ms*config.TPS {
		return false
	}
	c.tick = 0
	c.frame++
	if c.frame >= c.frames-1 {
		c.frame = c.frames - 1
		c.opening, c.opened = false, true
		return true
	}
	return false
}

// Подсветка сундука. Ореол рисуется самим спрайтом, размноженным по восьми
// сторонам со сложением цвета: у пиксель-арта это дешевле и честнее отдельной
// маски — контур повторяет силуэт кадра, каким бы он ни был.
var chestGlowDirs = [8]engine.Vec2{
	{X: 1}, {X: -1}, {Y: 1}, {Y: -1},
	{X: 1, Y: 1}, {X: 1, Y: -1}, {X: -1, Y: 1}, {X: -1, Y: -1},
}

// chestGlow — цвет ореола (тёплое золото) и его сила в трёх положениях.
var chestGlow = color.RGBA{0xff, 0xd2, 0x6a, 0xff}

const (
	chestGlowIdle   = 0.30 // просто стоит и зовёт
	chestGlowNear   = 0.85 // герой рядом — можно открывать
	chestGlowOpened = 0.10 // уже вскрыт: тлеет, чтобы не терялся на карте
)

// draw рисует сундук: точка pos — его основание, как у зверей. Перед самим
// спрайтом кладётся ореол, иначе сундук теряется среди травы и камней.
func (c *chest) draw(dst *ebiten.Image, at engine.Vec2) {
	if c == nil {
		return
	}
	img, ok := c.pack.Frame(c.kind.Sprite, c.frame)
	if !ok {
		return
	}
	b := img.Bounds()
	x := math.Round(at.X - float64(b.Dx())/2)
	y := math.Round(at.Y - float64(b.Dy()))

	if k := c.glow(); k > 0 {
		for _, d := range chestGlowDirs {
			op := &ebiten.DrawImageOptions{Blend: ebiten.BlendLighter}
			op.GeoM.Translate(x+d.X, y+d.Y)
			op.ColorScale.ScaleWithColor(chestGlow)
			op.ColorScale.ScaleAlpha(float32(k))
			dst.DrawImage(img, op)
		}
	}

	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(x, y)
	dst.DrawImage(img, op)
}

// glow — сила подсветки в этот кадр. Мерцание медленное и неглубокое: сундук
// должен притягивать взгляд, а не мигать как аварийка.
func (c *chest) glow() float64 {
	k := chestGlowIdle
	switch {
	case c.opened:
		k = chestGlowOpened
	case c.near:
		k = chestGlowNear
	}
	return k * (0.8 + 0.2*math.Sin(float64(c.pulse)/18))
}

// depth — глубина для сортировки с прочими телами.
func (c *chest) depth() float64 { return c.pos.Y }

// useChest ведёт сундук в такте забега: крутит открывание и решает, не пора ли
// показать содержимое. Возвращает сцену, если окно сундука открылось.
func (g *Game) useChest() Scene {
	c := g.chest
	if c == nil {
		return nil
	}
	c.pulse++
	c.near = g.pl.Alive() && c.nearPlayer(g.pl.Pos)
	show := c.update() // крышка поднялась именно в этот тик
	if g.pl.Alive() && c.nearPlayer(g.pl.Pos) &&
		inpututil.IsKeyJustPressed(settings.Key(settings.ActUse)) {
		if c.opened {
			show = true // уже открыт — просто заглянуть снова
		} else {
			c.begin()
			g.sfx.at(audio.ChestOpen, c.pos)
		}
	}
	if !show {
		return nil
	}
	if v := newChestView(g, c); v != nil {
		return v
	}
	return nil
}

// chestHintLift — на сколько поднять подсказку над основанием сундука.
const chestHintLift = 30

// drawChestHint пишет над сундуком, чем его открыть. Клавиша берётся из
// настроек: игрок мог её переназначить, и обещать ему «E» было бы враньём.
func (g *Game) drawChestHint(dst *ebiten.Image) {
	c := g.chest
	if c == nil || c.opening || !c.nearPlayer(g.pl.Pos) || !g.pl.Alive() {
		return
	}
	what := "ОТКРЫТЬ"
	if c.opened {
		what = "ЗАГЛЯНУТЬ"
	}
	txt := settings.KeyLabel(settings.Key(settings.ActUse)) + " - " + what
	p := g.cam.WorldToScreen(engine.Vec2{X: c.pos.X, Y: c.pos.Y - chestHintLift})
	w := ui.PixelTextWidth(txt, 1)
	vector.FillRect(dst, float32(p.X-w/2)-3, float32(p.Y)-2,
		float32(w)+6, float32(ui.PixelTextHeight(1))+4, hudBack, false)
	ui.PixelTextCentered(dst, txt, p.X, p.Y, 1, hudText)
}
