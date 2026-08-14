package scene

// Добыча на земле: что происходит между «моб умер» и «вещь в сумке».
//
// Живёт в сцене, а не в mob или item: сам моб ничего не роняет, он только знает
// свою таблицу (mob.Drop), а сумка не знает про карту. Свести труп, поле и
// сумку может только игровой слой — здесь.

import (
	"encoding/json"
	"fmt"
	"image/color"
	"io/fs"
	"math"
	"math/rand/v2"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/engine"
	"github.com/vladislav/game/internal/mob"
	"github.com/vladislav/game/internal/physics"
	"github.com/vladislav/game/internal/ui"
	"github.com/vladislav/game/internal/world"
)

const (
	lootFile = "items/loot.json"
	// lootTries — сколько мест перебрать под одну вещь. Первые броски держатся
	// трупа, дальние ищут землю в стороне: моб мог умереть в воде или на самом
	// краю обрыва.
	lootTries = 12
	// lootIcon — во что вписывается иконка на земле. Иконки в паках разного
	// размера (12..17 px), а лежащие вещи должны читаться одинаково.
	lootIcon = 12
)

// lootRules — правила добычи, общие для всех мобов (assets/items/loot.json).
// Сами таблицы лежат у мобов: добыча описывает моба, а не предмет.
type lootRules struct {
	Roll struct {
		EliteChanceBonus float64 `json:"elite_chance_bonus"`
		EliteExtraRolls  int     `json:"elite_extra_rolls"`
	} `json:"roll"`
	Ground struct {
		Scatter float64 `json:"scatter"`
		HopMS   int     `json:"hop_ms"`
		HopLift float64 `json:"hop_lift"`
	} `json:"ground"`
	Pickup struct {
		Radius  float64 `json:"radius"`
		Magnet  float64 `json:"magnet"`
		Speed   float64 `json:"speed"`
		DelayMS int     `json:"delay_ms"`
	} `json:"pickup"`
}

// loadLoot читает правила добычи. Пустые числа — ошибка, а не умолчание: вещь с
// нулевым радиусом подбора просто нельзя поднять, и понять это по игре трудно.
func loadLoot(fsys fs.FS, p string) (*lootRules, error) {
	b, err := fs.ReadFile(fsys, p)
	if err != nil {
		return nil, fmt.Errorf("scene: чтение %q: %w", p, err)
	}
	var r lootRules
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("scene: разбор %q: %w", p, err)
	}
	switch {
	case r.Pickup.Radius <= 0:
		return nil, fmt.Errorf("scene: %q: нулевой радиус подбора", p)
	case r.Pickup.Magnet < r.Pickup.Radius:
		return nil, fmt.Errorf("scene: %q: притяжение %.0f меньше радиуса подбора %.0f",
			p, r.Pickup.Magnet, r.Pickup.Radius)
	case r.Pickup.Speed <= 0:
		return nil, fmt.Errorf("scene: %q: нулевая скорость притяжения", p)
	case r.Ground.Scatter <= 0:
		return nil, fmt.Errorf("scene: %q: нулевой разброс: вещи лягут друг в друга", p)
	}
	return &r, nil
}

// ticks переводит миллисекунды правил в тики забега.
func ticks(ms int) int { return ms * config.TPS / 1000 }

// groundItem — стопка, лежащая на карте.
type groundItem struct {
	id string
	n  int

	icon  *ebiten.Image
	pos   engine.Vec2 // куда легла
	from  engine.Vec2 // откуда прыгнула (точка смерти)
	at    engine.Vec2 // где она в этот кадр: прыжок и притяжение
	lift  float64     // насколько поднята над землёй сейчас
	floor uint8       // этаж: с плато вещь вниз не падает
	age   int         // тиков с падения
	full  bool        // о полной сумке сказали один раз
}

// depth — глубина для общей сортировки тел.
func (d *groundItem) depth() float64 { return d.at.Y }

// draw рисует лежащую вещь: тень на земле и иконку над ней. Тень нужна не для
// красоты — по ней видно, куда вещь ляжет, пока она ещё в прыжке.
func (d *groundItem) draw(dst *ebiten.Image, at engine.Vec2) {
	x, y := math.Round(at.X), math.Round(at.Y)
	vector.FillRect(dst, float32(x)-4, float32(y)-1, 8, 2, lootShadow, false)
	if d.icon == nil {
		// Предмет без иконки не должен пропадать с карты: он всё ещё лежит и
		// всё ещё подбирается.
		vector.FillRect(dst, float32(x)-3, float32(y)-8, 6, 6, lootNoIcon, false)
		return
	}
	b := d.icon.Bounds()
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(x-float64(b.Dx())/2, y-d.lift-float64(b.Dy()))
	dst.DrawImage(d.icon, op)
	// Число в стопке — там же, где в ячейке сумки: у правого нижнего угла
	// иконки, с тенью в пиксель, иначе на траве его не видно.
	if d.n > 1 {
		n := strconv.Itoa(d.n)
		tx, ty := x+float64(b.Dx())/2-ui.PixelTextWidth(n, 1), y-d.lift-ui.PixelTextHeight(1)
		ui.PixelText(dst, n, tx+1, ty+1, 1, chestShadow)
		ui.PixelText(dst, n, tx, ty, 1, chestCount)
	}
}

// dropLoot высыпает добычу таблицы drops на землю в точке from.
//
// Вызывается ровно в тик смерти. Не по исчезновению трупа: игрок связывает
// выпавшее с ударом, который убил, а к моменту растворения он уже дерётся с
// другим мобом.
func (g *Game) dropLoot(from engine.Vec2, floor uint8, drops []mob.Drop, rolls int, elite bool) {
	if g.loot == nil || len(drops) == 0 {
		return
	}
	bonus, extra := 0.0, 0
	if elite {
		bonus, extra = g.loot.Roll.EliteChanceBonus, g.loot.Roll.EliteExtraRolls
	}
	for _, l := range mob.RollDrops(drops, rolls+extra, bonus, g.lrng) {
		g.dropStack(from, floor, l.ID, l.N)
	}
}

// dropStack кладёт одну стопку на землю рядом с точкой from.
func (g *Game) dropStack(from engine.Vec2, floor uint8, id string, n int) {
	if n <= 0 {
		return
	}
	pos := lootSpot(g.m, from, floor, g.loot.Ground.Scatter, g.lrng)
	// Иконка грузится сразу: пак предмета всё равно понадобится, а в бою
	// незаметная задержка на первом мясе лучше, чем на каждом кадре.
	icon, err := g.items.Icon(g.l, id)
	if err != nil {
		icon = nil
	}
	g.drops = append(g.drops, &groundItem{
		id: id, n: n, icon: icon,
		pos: pos, from: from, at: from, floor: floor,
	})
}

// lootSpot ищет, куда лечь вещи: рядом с трупом, на твёрдой земле того же
// этажа. Радиус поиска растёт с попытками — моб мог умереть над водой (летуны)
// или у самого обрыва. Не нашлось ничего — точка смерти как есть: вещь в
// неудобном месте лучше, чем пропавшая добыча.
func lootSpot(m *world.Map, from engine.Vec2, floor uint8, scatter float64, rng *rand.Rand) engine.Vec2 {
	f := m.Field()
	w, h := m.Size()
	body := physics.Body{Radius: 3, Floor: floor}
	for i := range lootTries {
		a := rng.Float64() * 2 * math.Pi
		d := scatter * (0.35 + 0.65*rng.Float64()) * (1 + float64(i)/3)
		p := engine.Vec2{X: from.X + math.Cos(a)*d, Y: from.Y + math.Sin(a)*d}
		if p.X < 8 || p.Y < 8 || p.X >= w-8 || p.Y >= h-8 {
			continue
		}
		if !f.Fits(p, body) {
			continue
		}
		return p
	}
	return from
}

// updateLoot ведёт лежащую добычу: прыжок при падении, притяжение к герою и
// подбор. Отдельного действия у игрока нет — подойти достаточно.
func (g *Game) updateLoot() {
	if len(g.drops) == 0 {
		return
	}
	r := g.loot
	f := g.m.Field()
	hop, delay := ticks(r.Ground.HopMS), ticks(r.Pickup.DelayMS)

	live := g.drops[:0]
	for _, d := range g.drops {
		d.age++
		// Прыжок: вещь летит от трупа к своему месту и опускается.
		if k := float64(d.age) / float64(max(hop, 1)); k < 1 {
			d.at = engine.Vec2{
				X: engine.Lerp(d.from.X, d.pos.X, k),
				Y: engine.Lerp(d.from.Y, d.pos.Y, k),
			}
			d.lift = math.Sin(k*math.Pi) * r.Ground.HopLift
		} else {
			d.at, d.lift = d.pos, 0
		}

		if d.age >= max(hop, delay) && g.pl.Alive() && g.pl.Floor() == d.floor {
			switch dist := engine.Dist(d.pos, g.pl.Pos); {
			case dist <= r.Pickup.Radius:
				if g.grab(d) {
					continue // ушла в сумку целиком
				}
			case dist <= r.Pickup.Magnet:
				// Ползёт через поле, а не по прямой: иначе вещь протискивалась
				// бы к герою сквозь скалу или соскальзывала в воду.
				step := r.Pickup.Speed / config.TPS
				dir := g.pl.Pos.Sub(d.pos).Normalized()
				d.pos, _ = f.Move(d.pos, dir.Scale(step), physics.Body{Radius: 2, Floor: d.floor})
				d.at = d.pos
			}
		}
		live = append(live, d)
	}
	g.drops = live
}

// grab прячет вещь в сумку. false — влезла не вся: остаток продолжает лежать,
// добыча не пропадает от того, что сумка полна.
func (g *Game) grab(d *groundItem) bool {
	left := g.bag.Add(d.id, d.n)
	if got := d.n - left; got > 0 {
		g.fx.text(g.lootHead(d), "+"+strconv.Itoa(got)+" "+g.items.Name(d.id), fxLoot)
		d.full = false
	}
	d.n = left
	if left == 0 {
		return true
	}
	// Про полную сумку говорят один раз на вещь: герой стоит на ней, и
	// сообщение иначе повторялось бы каждый тик.
	if !d.full {
		d.full = true
		g.fx.text(g.lootHead(d), "СУМКА ПОЛНА", fxHurt)
	}
	return false
}

// lootHead — точка над вещью: оттуда всплывает надпись о подборе.
func (g *Game) lootHead(d *groundItem) engine.Vec2 {
	return engine.Vec2{X: d.at.X, Y: d.at.Y - lootIcon - 2}
}

// Цвета лежащей добычи.
var (
	lootShadow = color.RGBA{0x0d, 0x10, 0x18, 0x88}
	lootNoIcon = color.RGBA{0xe8, 0xd0, 0x70, 0xff}
	fxLoot     = color.RGBA{0xb8, 0xe8, 0x90, 0xff} // надпись о подобранном
)
