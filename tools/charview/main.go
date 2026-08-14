// Команда charview — просмотрщик персонажа: показывает, что на самом деле лежит
// в паках assets/character и как это крутит автомат состояний internal/character.
//
// Два режима.
//
//	СЕТКА (1) — все пары «тело × лоадаут» сразу, одна и та же анимация в каждой.
//	Нужна, чтобы глазами сверить графику с данными: какие клипы есть, сколько в
//	них кадров, где точка опоры, совпадают ли наборы у мужского и женского тела.
//
//	ЖИВОЙ (2) — управляемый персонаж на площадке с прудом. Нужен, чтобы увидеть
//	сам автомат: переходы idle/walk/run/attack/hurt/death, деградацию клипов у
//	unarmed, сектор удара и кадр, на котором он наносится.
//
// Отдельная команда, а не сцена в игре: это инструмент для правки контента,
// такой же как tools/spriteanchor, и в сборку игры ему попадать незачем.
//
//	go run ./tools/charview            # сетка
//	go run ./tools/charview -live      # сразу живой режим
//	go run ./tools/charview -assets assets
package main

import (
	"flag"
	"fmt"
	"image/color"
	"log"
	"math"
	"os"
	"sort"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/vladislav/game/internal/anim"
	"github.com/vladislav/game/internal/assets"
	"github.com/vladislav/game/internal/character"
	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/engine"
	"github.com/vladislav/game/internal/physics"
	"github.com/vladislav/game/internal/sprite"
	"github.com/vladislav/game/internal/ui"
)

func main() {
	root := flag.String("assets", "assets", "каталог с ресурсами")
	live := flag.Bool("live", false, "начать с живого режима")
	flag.Parse()

	v, err := newViewer(*root, *live)
	if err != nil {
		log.Fatal(err)
	}
	ebiten.SetWindowTitle("charview — просмотрщик персонажа")
	ebiten.SetWindowSize(config.WindowW, config.WindowH)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	if err := ebiten.RunGame(v); err != nil {
		log.Fatal(err)
	}
}

// variant — одна пара «тело × лоадаут» с загруженным паком.
type variant struct {
	body    *character.Body
	loadout *character.Loadout
	pack    *sprite.Pack
	player  *anim.Player
}

func (v variant) name() string { return v.body.ID + "/" + v.loadout.ID }

type viewer struct {
	cat  *character.Catalog
	vars []variant

	live  bool
	dir   sprite.Dir
	anims []string // объединение имён клипов всех паков
	cur   int      // выбранная анимация в режиме сетки
	pause bool
	marks bool // показывать точку опоры и рамку

	// живой режим
	p       *character.Player
	world   *field
	curBody int
	curLoad int
	hit     *character.Hit
	hitLeft int // сколько тиков ещё показывать сектор удара
}

func newViewer(root string, live bool) (*viewer, error) {
	l := assets.NewLoader(os.DirFS(root))
	cat, err := character.Load(l.FS(), "character/character.json")
	if err != nil {
		return nil, err
	}
	// Просмотрщик заодно сверяет таблицу: расхождения в данных лучше увидеть
	// в консоли при старте, чем ловить их потом в поведении.
	for _, p := range cat.Validate() {
		fmt.Fprintln(os.Stderr, "character.json:", p)
	}

	v := &viewer{cat: cat, live: live, world: newField()}
	names := map[string]bool{}
	for _, bid := range cat.BodyIDs() {
		for _, lid := range cat.LoadoutIDs() {
			b, ld := cat.Body(bid), cat.Loadout(lid)
			pack, err := character.LoadPack(l, "character", b, ld)
			if err != nil {
				return nil, err
			}
			v.vars = append(v.vars, variant{body: b, loadout: ld, pack: pack, player: anim.NewPlayer(nil)})
			for _, n := range pack.Anims() {
				names[n] = true
			}
		}
	}
	for n := range names {
		v.anims = append(v.anims, n)
	}
	sort.Strings(v.anims)
	v.reload()

	v.p = character.NewPlayer(cat, v.vars[0].body, v.vars[0].loadout, v.vars[0].pack,
		engine.Vec2{X: config.ScreenW / 2, Y: config.ScreenH / 2})
	return v, nil
}

// reload перезаряжает проигрыватели сетки после смены анимации/направления.
func (v *viewer) reload() {
	for i := range v.vars {
		c := v.vars[i].pack.Clip(v.anims[v.cur], v.dir)
		v.vars[i].player.Play(c)
	}
}

func (v *viewer) Update() error {
	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeyEscape):
		return ebiten.Termination
	case inpututil.IsKeyJustPressed(ebiten.Key1):
		v.live = false
	case inpututil.IsKeyJustPressed(ebiten.Key2):
		v.live = true
	case inpututil.IsKeyJustPressed(ebiten.KeyTab):
		v.dir = (v.dir + 1) % sprite.DirCount
		v.reload()
	case inpututil.IsKeyJustPressed(ebiten.KeyF):
		v.marks = !v.marks
	case inpututil.IsKeyJustPressed(ebiten.KeySpace):
		v.pause = !v.pause
	}
	if v.live {
		return v.updateLive()
	}
	return v.updateGrid()
}

func (v *viewer) updateGrid() error {
	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeyBracketLeft), inpututil.IsKeyJustPressed(ebiten.KeyLeft):
		v.cur = (v.cur - 1 + len(v.anims)) % len(v.anims)
		v.reload()
	case inpututil.IsKeyJustPressed(ebiten.KeyBracketRight), inpututil.IsKeyJustPressed(ebiten.KeyRight):
		v.cur = (v.cur + 1) % len(v.anims)
		v.reload()
	}
	if v.pause {
		return nil
	}
	for i := range v.vars {
		v.vars[i].player.Update()
	}
	return nil
}

func (v *viewer) updateLive() error {
	// Смена варианта на лету — заодно проверка Equip/SetBody: персонаж не
	// должен ни телепортироваться, ни терять здоровье.
	if inpututil.IsKeyJustPressed(ebiten.KeyE) {
		v.curLoad = (v.curLoad + 1) % len(v.cat.LoadoutIDs())
		l := v.cat.Loadout(v.cat.LoadoutIDs()[v.curLoad])
		v.p.Equip(l, v.packOf(v.p.Body.ID, l.ID))
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyG) {
		v.curBody = (v.curBody + 1) % len(v.cat.BodyIDs())
		b := v.cat.Body(v.cat.BodyIDs()[v.curBody])
		v.p.SetBody(b, v.packOf(b.ID, v.p.Loadout.ID))
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyH) {
		v.p.Damage(7, v.p.Pos.Add(engine.Vec2{X: -20}))
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyK) {
		v.p.Kill()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyR) {
		v.p.Revive(engine.Vec2{X: config.ScreenW / 2, Y: config.ScreenH / 2})
	}
	if v.pause {
		return nil
	}

	in := character.Input{Run: ebiten.IsKeyPressed(ebiten.KeyShift)}
	if ebiten.IsKeyPressed(ebiten.KeyW) {
		in.Move.Y--
	}
	if ebiten.IsKeyPressed(ebiten.KeyS) {
		in.Move.Y++
	}
	if ebiten.IsKeyPressed(ebiten.KeyA) {
		in.Move.X--
	}
	if ebiten.IsKeyPressed(ebiten.KeyD) {
		in.Move.X++
	}
	in.Attack = inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) ||
		inpututil.IsKeyJustPressed(ebiten.KeyJ)
	if mx, my := ebiten.CursorPosition(); mx > 0 || my > 0 {
		in.Aim, in.HasAim = engine.Vec2{X: float64(mx), Y: float64(my)}, true
	}

	v.p.Update(in, v.world.f)
	if h, ok := v.p.Strike(); ok {
		v.hit, v.hitLeft = &h, 12 // сектор виден недолго — это вспышка, а не поле
	}
	if v.hitLeft > 0 {
		v.hitLeft--
	}
	return nil
}

func (v *viewer) packOf(body, loadout string) *sprite.Pack {
	for _, x := range v.vars {
		if x.body.ID == body && x.loadout.ID == loadout {
			return x.pack
		}
	}
	return nil
}

func (v *viewer) Draw(dst *ebiten.Image) {
	if v.live {
		v.drawLive(dst)
		return
	}
	v.drawGrid(dst)
}

const (
	cols  = 2
	cellW = config.ScreenW / cols
	cellH = 130
	top   = 26
)

func (v *viewer) drawGrid(dst *ebiten.Image) {
	dst.Fill(color.RGBA{0x10, 0x12, 0x1a, 0xff})
	name := v.anims[v.cur]
	ui.TextCentered(dst, fmt.Sprintf("АНИМАЦИЯ  %s        НАПРАВЛЕНИЕ  %s%s",
		name, v.dir, pausedMark(v.pause)), config.ScreenW/2, 6)

	for i, x := range v.vars {
		cx := float64(i%cols)*cellW + cellW/2
		cy := float64(i/cols)*cellH + top + cellH/2

		has := x.pack.Has(name)
		box := color.RGBA{0x22, 0x26, 0x33, 0xff}
		if !has {
			box = color.RGBA{0x3a, 0x1e, 0x1e, 0xff} // клипа нет — видно сразу
		}
		vector.FillRect(dst, float32(cx-cellW/2+4), float32(cy-cellH/2+4),
			cellW-8, cellH-8, box, false)

		if img := x.player.Frame(); img != nil {
			foot := x.pack.Foot()
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Translate(cx-float64(foot.X), cy+30-float64(foot.Y))
			dst.DrawImage(img, op)
			if v.marks {
				drawMarks(dst, x.pack, cx-float64(foot.X), cy+30-float64(foot.Y))
			}
		}

		ui.DrawText(dst, x.name(), cx-cellW/2+10, cy-cellH/2+14, color.White)
		info := "нет клипа — деградация"
		if def, ok := x.pack.Animations[name]; ok {
			info = fmt.Sprintf("%d кадр. / %d fps", def.Frames, def.FPS)
		}
		ui.DrawText(dst, info, cx-cellW/2+10, cy-cellH/2+26, color.RGBA{0x9a, 0xa4, 0xb8, 0xff})
		ui.DrawText(dst, fmt.Sprintf("клипы: %d", len(x.pack.Anims())),
			cx-cellW/2+10, cy+cellH/2-16, color.RGBA{0x6a, 0x74, 0x88, 0xff})
	}
	ui.TextCentered(dst, "[ ] анимация   TAB направление   F опора/рамка   ПРОБЕЛ пауза   2 живой режим   ESC выход",
		config.ScreenW/2, config.ScreenH-14)
}

func (v *viewer) drawLive(dst *ebiten.Image) {
	dst.Fill(color.RGBA{0x1b, 0x2a, 0x1c, 0xff})
	v.world.draw(dst)

	if v.hitLeft > 0 && v.hit != nil {
		drawSector(dst, *v.hit)
	}
	v.p.Draw(dst, v.p.Pos)
	if v.marks {
		foot := v.p.Pack.Foot()
		drawMarks(dst, v.p.Pack, v.p.Pos.X-float64(foot.X), v.p.Pos.Y-float64(foot.Y))
		vector.StrokeCircle(dst, float32(v.p.Pos.X), float32(v.p.Pos.Y),
			float32(v.p.Radius()), 1, color.RGBA{0x00, 0xff, 0xff, 0xaa}, false)
	}

	a := v.p.Loadout.Attack
	lines := []string{
		fmt.Sprintf("%s / %s%s", v.p.Body.ID, v.p.Loadout.ID, pausedMark(v.pause)),
		fmt.Sprintf("состояние %-7s клип %-12s взгляд %s", v.p.State(), v.p.Clip(), v.p.Dir()),
		fmt.Sprintf("hp %d/%d%s   скорость %.0f/%.0f", v.p.HP, v.p.MaxHP,
			invulnMark(v.p.Invulnerable()), v.p.Speed(false), v.p.Speed(true)),
		fmt.Sprintf("удар: урон %d  радиус %.0f  сектор %.0f°  замах %d т.  откат %d т.  на ходу %v",
			a.Damage, a.Reach, a.Arc, a.SwingTicks, a.CooldownTicks, a.OnMove),
	}
	for i, s := range lines {
		ui.DrawText(dst, s, 8, float64(8+i*12), color.White)
	}
	ui.TextCentered(dst, "WASD ход   SHIFT бег   ЛКМ/J удар   E оружие   G тело   H урон   K смерть   R подъём   F метки   1 сетка",
		config.ScreenW/2, config.ScreenH-14)
}

// drawMarks показывает, откуда персонаж «растёт»: рамку непрозрачных пикселей
// и точку опоры. Ошибки в них видно только так — в игре они прячутся за тем,
// что спрайт всё равно нарисован.
func drawMarks(dst *ebiten.Image, p *sprite.Pack, x, y float64) {
	b := p.Bounds()
	vector.StrokeRect(dst, float32(x+float64(b.X)), float32(y+float64(b.Y)),
		float32(b.W), float32(b.H), 1, color.RGBA{0xff, 0xd0, 0x40, 0x88}, false)
	f := p.Foot()
	vector.FillCircle(dst, float32(x+float64(f.X)), float32(y+float64(f.Y)), 2,
		color.RGBA{0xff, 0x40, 0x40, 0xff}, false)
}

// drawSector рисует сектор состоявшегося удара — то самое, что игровой слой
// сверяет с целями через Hit.Covers.
func drawSector(dst *ebiten.Image, h character.Hit) {
	base := h.Face.Angle()
	half := h.Arc / 2 * math.Pi / 180
	var path vector.Path
	path.MoveTo(float32(h.Center.X), float32(h.Center.Y))
	const steps = 16
	for i := 0; i <= steps; i++ {
		a := base - half + 2*half*float64(i)/steps
		path.LineTo(float32(h.Center.X+math.Cos(a)*h.Reach), float32(h.Center.Y+math.Sin(a)*h.Reach))
	}
	path.Close()

	op := &vector.DrawPathOptions{}
	op.ColorScale.ScaleWithColor(color.RGBA{0xff, 0xd9, 0x4d, 0x59})
	vector.FillPath(dst, &path, nil, op)
}

func (v *viewer) Layout(int, int) (int, int) { return config.ScreenW, config.ScreenH }

// field — площадка живого режима: прямоугольник суши, за ним «обрыв» (стена), а
// справа пруд с каймой мелководья. Пруд нужен, чтобы было видно, что персонаж на
// глубину не выходит, а по мели бредёт медленнее.
//
// Собрана она из настоящего physics.Field — того же, по которому персонаж ходит
// в игре. Своей проходимости у просмотрщика нет: правила должны совпадать, иначе
// он показывает не то поведение, которое будет в забеге.
type field struct {
	x, y, w, h     float64
	px, py, pw, ph float64 // пруд (вместе с каймой мели)
	f              *physics.Field
}

// fieldSub — сторона под-клетки площадки. Как в картах биомов: полтайла.
const fieldSub = 8.0

func newField() *field {
	fl := &field{
		x: 24, y: 40, w: config.ScreenW - 48, h: config.ScreenH - 90,
		px: config.ScreenW - 150, py: 80, pw: 100, ph: 90,
	}
	sw := int(config.ScreenW/fieldSub) + 1
	sh := int(config.ScreenH/fieldSub) + 1
	cells := make([]physics.Cell, sw*sh)
	for sy := range sh {
		for sx := range sw {
			cx, cy := (float64(sx)+0.5)*fieldSub, (float64(sy)+0.5)*fieldSub
			cells[sy*sw+sx] = fl.cellAt(cx, cy)
		}
	}
	fl.f = physics.NewField(sw, sh, fieldSub, cells)
	return fl
}

// cellAt — что на площадке в этой точке.
func (f *field) cellAt(x, y float64) physics.Cell {
	inRect := func(rx, ry, rw, rh float64) bool {
		return x >= rx && x <= rx+rw && y >= ry && y <= ry+rh
	}
	switch {
	case !inRect(f.x, f.y, f.w, f.h):
		return physics.Solid // за краем площадки — стена
	case inRect(f.px+shoreBand, f.py+shoreBand, f.pw-2*shoreBand, f.ph-2*shoreBand):
		return physics.Deep
	case inRect(f.px, f.py, f.pw, f.ph):
		return physics.Shallow
	}
	return physics.Ground
}

// shoreBand — ширина каймы мелководья вокруг пруда.
const shoreBand = 12.0

func (f *field) draw(dst *ebiten.Image) {
	vector.StrokeRect(dst, float32(f.x), float32(f.y), float32(f.w), float32(f.h), 1,
		color.RGBA{0x3c, 0x5a, 0x3c, 0xff}, false)
	vector.FillRect(dst, float32(f.px), float32(f.py), float32(f.pw), float32(f.ph),
		color.RGBA{0x33, 0x66, 0x9e, 0xff}, false) // мель
	vector.FillRect(dst, float32(f.px+shoreBand), float32(f.py+shoreBand),
		float32(f.pw-2*shoreBand), float32(f.ph-2*shoreBand),
		color.RGBA{0x22, 0x44, 0x7a, 0xff}, false) // глубина
}

func pausedMark(p bool) string {
	if p {
		return "   [ПАУЗА]"
	}
	return ""
}

func invulnMark(inv bool) string {
	if inv {
		return " (неуязвим)"
	}
	return ""
}
