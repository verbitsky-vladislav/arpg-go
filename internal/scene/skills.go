package scene

// Украденные умения: три ячейки внизу справа, камень на земле и удар чужой
// силой.
//
// Что откуда берётся: сила описана у типа врага (enemies.json, power) и
// снимается с убитого (mob.Steal) — там же считается, на сколько ударов её
// хватит. Сцена делает остальное: роняет камень, кладёт его в свободную ячейку,
// тратит заряды и просит персонажа ударить чужим (character.Input.Cast).
//
// Ячейки — не сумка. Камень нельзя ни сложить в стопку, ни надеть; он либо
// лежит в одной из трёх ячеек, либо его нет. Поэтому и хранятся они отдельно от
// item.Inventory, а не гнездом снаряжения.

import (
	"fmt"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/vladislav/game/internal/audio"
	"github.com/vladislav/game/internal/character"
	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/engine"
	"github.com/vladislav/game/internal/mob"
	"github.com/vladislav/game/internal/settings"
	"github.com/vladislav/game/internal/ui"
)

// skillSlots — сколько умений герой носит разом. Три: больше не помещается под
// пальцами (Q, E, R по умолчанию), а меньше превращает кражу в лотерею.
const skillSlots = 3

// skillBar — ячейки умений героя.
type skillBar struct {
	slots [skillSlots]*mob.Stolen
}

// put кладёт камень в первую свободную ячейку. false — свободных нет: камень
// остаётся лежать, а не пропадает.
func (b *skillBar) put(s *mob.Stolen) bool {
	if s.Empty() {
		return false
	}
	for i, cur := range b.slots {
		if cur.Empty() {
			b.slots[i] = s
			return true
		}
	}
	return false
}

// spend тратит заряд ячейки i и возвращает силу, которой бьют. nil — ячейка
// пуста. Опустевшая ячейка очищается сразу: камень с нулём зарядов рассыпается.
func (b *skillBar) spend(i int) *mob.Stolen {
	if i < 0 || i >= skillSlots || b.slots[i].Empty() {
		return nil
	}
	s := b.slots[i]
	s.Charges--
	if s.Charges <= 0 {
		b.slots[i] = nil
	}
	return s
}

// castRequest — просьба ударить чужой силой в этом тике (nil — не просят).
//
// Заряд тратится здесь же, в момент нажатия: персонаж просьбу не отклонит —
// готовность удара проверена тут же, а откладывать списание до кадра попадания
// значило бы дать бесплатный замах тому, кого прервали.
func (g *Game) castRequest() *character.Spell {
	if !g.pl.Alive() || !g.pl.CanAttack() {
		return nil
	}
	for i, act := range settings.SkillActions {
		if !inpututil.IsKeyJustPressed(settings.Key(act)) {
			continue
		}
		s := g.skills.spend(i)
		if s == nil {
			continue
		}
		g.fx.text(g.playerHead(), s.Title, skillCast)
		g.castTint = stoneGlow(s)
		return &character.Spell{Attack: spellAttack(s), Damage: s.Rolls()}
	}
	return nil
}

// takeCastTint — цвет следа для замаха, который сейчас состоялся: цвет стихии,
// если это был удар украденной силой, иначе нулевой (свой светлый след).
//
// Цвет одноразовый: замах и его след — одно событие, и следующий удар обязан
// снова быть своим. Сбитый замах цвет не тратит, поэтому он же гасится в
// Update, когда персонаж вышел из состояния удара.
func (g *Game) takeCastTint() color.RGBA {
	c := g.castTint
	g.castTint = color.RGBA{}
	return c
}

// spellAttack переводит удар врага в удар героя. Поля совпадают один в один
// (так задуман mob.PowerAttack), кроме hit_at: там кадр чужого слоя-эффекта, а
// герой играет свой клип — кадр попадания у него свой (см. Player.hitFrame).
func spellAttack(s *mob.Stolen) character.Attack {
	a := s.Attack
	return character.Attack{
		Reach: a.Reach, Arc: a.Arc,
		SwingTicks: a.SwingTicks, CooldownTicks: a.CooldownTicks,
		Knockback: a.Knockback, OnMove: a.OnMove,
	}
}

// stealFrom роняет камень умения с убитого врага, если сила выпала.
func (g *Game) stealFrom(e *mob.Enemy) {
	s := mob.Steal(e, g.lrng)
	if s == nil {
		return
	}
	g.dropStone(e.Pos, e.Floor(), s)
}

// dropStone кладёт камень умения на землю рядом с точкой from — тем же прыжком,
// что и обычную добычу: подбирается он так же, разница только в подсветке и в
// том, куда ляжет.
func (g *Game) dropStone(from engine.Vec2, floor uint8, s *mob.Stolen) {
	if g.loot == nil || s.Empty() {
		return
	}
	id := stoneItem(s.Element)
	icon, err := g.items.Icon(g.l, id)
	if err != nil {
		icon = nil
	}
	pos := lootSpot(g.m, from, floor, g.loot.Ground.Scatter, g.lrng)
	g.drops = append(g.drops, &groundItem{
		id: id, n: 1, icon: icon, power: s,
		pos: pos, from: from, at: from, floor: floor,
	})
}

// grabStone прячет камень в ячейку умений. false — все три заняты: камень
// остаётся лежать, и об этом говорят один раз.
func (g *Game) grabStone(d *groundItem) bool {
	if g.skills.put(d.power) {
		g.fx.text(g.lootHead(d), d.power.Title+" "+fmt.Sprintf("×%d", d.power.Charges), skillCast)
		g.sfx.at(audio.Pickup, d.at)
		d.full = false
		return true
	}
	if !d.full {
		d.full = true
		g.fx.text(g.lootHead(d), "ЯЧЕЙКИ УМЕНИЙ ЗАНЯТЫ", fxHurt)
		g.sfx.play(audio.UIDenied)
	}
	return false
}

// stoneItem — каким камнем показывается сила стихии element. Стихий у врагов
// больше, чем видов урона, и картинка у каждой своя; незнакомая стихия
// показывается камнем клинка — камень без картинки хуже, чем чужая картинка.
func stoneItem(element string) string {
	switch element {
	case "fire", "slash", "magic", "spectral", "smoke":
		return "stone_" + element
	}
	return "stone_slash"
}

// Цвета умений: подсветка камня по стихии и надпись о касте.
var (
	skillCast  = color.RGBA{0xc8, 0xa8, 0xff, 0xff}
	skillGlows = map[string]color.RGBA{
		"fire":     {0xff, 0x9a, 0x40, 0xff},
		"slash":    {0xff, 0xe0, 0x8a, 0xff},
		"magic":    {0xc8, 0x7a, 0xff, 0xff},
		"spectral": {0x7a, 0xe8, 0xd8, 0xff},
		"smoke":    {0xa8, 0xb8, 0xd8, 0xff},
	}
)

// stoneGlow — цвет ореола камня. Своя окраска силы (tint у рубящих) идёт
// вперёд общей: игрок должен различать камни на земле, не подходя к ним.
func stoneGlow(s *mob.Stolen) color.RGBA {
	if s == nil {
		return skillCast
	}
	if s.Tint != "" {
		if c, err := parseHexColor(s.Tint); err == nil {
			return c
		}
	}
	if c, ok := skillGlows[s.Element]; ok {
		return c
	}
	return skillCast
}

// parseHexColor разбирает "#rrggbb".
func parseHexColor(s string) (color.RGBA, error) {
	var r, g, b uint8
	if _, err := fmt.Sscanf(s, "#%02x%02x%02x", &r, &g, &b); err != nil {
		return color.RGBA{}, err
	}
	return color.RGBA{r, g, b, 0xff}, nil
}

// Ячейки умений на экране: правый нижний угол, квадраты со стороной skillCell.
const (
	skillCell = 26
	skillGap  = 4
	skillPad  = hudPad
)

// drawSkills рисует ячейки умений: рамка, камень, остаток зарядов и клавиша.
// Пустые ячейки не прячутся — по ним видно, что умения вообще бывают и сколько
// их носят.
func (g *Game) drawSkills(dst *ebiten.Image) {
	for i := range skillSlots {
		x, y := g.skillCellAt(i)
		s := g.skills.slots[i]

		vector.FillRect(dst, x, y, skillCell, skillCell, hudBack, false)
		edge := hudEdge
		if !s.Empty() {
			edge = stoneGlow(s)
		}
		vector.StrokeRect(dst, x+0.5, y+0.5, skillCell-1, skillCell-1, 1, edge, false)

		if !s.Empty() {
			if icon, err := g.items.Icon(g.l, stoneItem(s.Element)); err == nil {
				b := icon.Bounds()
				op := &ebiten.DrawImageOptions{}
				op.GeoM.Translate(
					math.Round(float64(x)+(skillCell-float64(b.Dx()))/2),
					math.Round(float64(y)+(skillCell-float64(b.Dy()))/2)-1)
				dst.DrawImage(icon, op)
			}
			n := fmt.Sprint(s.Charges)
			nx := float64(x) + skillCell - ui.PixelTextWidth(n, 1) - 2
			ny := float64(y) + skillCell - ui.PixelTextHeight(1) - 2
			ui.PixelText(dst, n, nx+1, ny+1, 1, hudShadow)
			ui.PixelText(dst, n, nx, ny, 1, hudText)
		}

		key := settings.KeyLabel(settings.Key(settings.SkillActions[i]))
		ui.PixelTextCentered(dst, key, float64(x)+skillCell/2,
			float64(y)-ui.PixelTextHeight(1)-2, 1, hudDim)
	}
}

// skillCellAt — левый верхний угол ячейки i. Считается от правого нижнего угла
// экрана: ячейки прижаты к нему, как в любой игре этого жанра.
func (g *Game) skillCellAt(i int) (x, y float32) {
	total := float32(skillSlots*skillCell + (skillSlots-1)*skillGap)
	x = config.ScreenW - skillPad - total + float32(i)*(skillCell+skillGap)
	return x, config.ScreenH - skillPad - skillCell
}

// playerHead — точка над героем: оттуда всплывают надписи о нём самом.
func (g *Game) playerHead() engine.Vec2 {
	return engine.Vec2{X: g.pl.Pos.X, Y: g.pl.Pos.Y - 34}
}
