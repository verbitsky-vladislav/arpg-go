// Package world — игровая карта в рантайме: чтение map_format v1, отрисовка и
// ответы на вопросы «можно ли тут стоять» и «что это за место».
//
// Карту на каждый запуск генерирует internal/worldgen — тот же пайплайн, что
// гоняет инструмент tools/worldgen; готовых карт в ресурсах нет, лежат только
// биомы (манифест, разметка, листы тайлов). Формат результата описан в
// docs/worldgen/map_format.md.
//
// Здесь Map реализует mob.SpawnWorld (поле физики, зоны, размер, биом) — тот
// самый интерфейс, ради которого mob не знает про worldgen. Ходьбу и стены
// целиком держит physics.Field, карта только отдаёт его наружу.
// Статические слои запекаются в один холст при создании карты, поэтому кадр
// стоит одну отрисовку видимого куска.
package world

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"io/fs"
	"math"
	"path"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/vladislav/game/internal/assets"
	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/engine"
	"github.com/vladislav/game/internal/physics"
	"github.com/vladislav/game/internal/worldgen"
)

// spawnBody — тело, под которое ищется точка появления героя: чуть толще
// самого героя (7 px), чтобы он появлялся не впритирку к скале, и без Caps —
// значит, на сухой земле, а не по колено в воде.
var spawnBody = physics.Body{Radius: 8}

// biomeManifest — единственное, что игре нужно от манифеста биома: каким
// листом рисуется каждый плотный слой. В самой карте этого нет (v1 хранит
// только локальные id), поэтому роль→лист берётся оттуда же, откуда её берёт
// генератор.
type biomeManifest struct {
	Terrains map[string]struct {
		Sheet string `json:"sheet"`
	} `json:"terrains"`
}

// Map — загруженная карта.
type Map struct {
	biome string
	seed  int64
	w, h  int // размер в тайлах
	ts    int // размер тайла в пикселях

	field   *physics.Field // проходимость и этажи: вся физика мира
	ground  []uint16       // трава — для мини-карты
	mud     []uint16       // тропы — для зоны trail
	plateau []uint16       // верх плато — для зоны plateau

	canvas *ebiten.Image // все статические слои, запечённые один раз
	detail []animCell    // анимированная рябь: рисуется каждый кадр
	props  []prop        // объекты: рисуются по глубине вместе с живыми
	sheets []*ebiten.Image
	cols   []int

	spawn engine.Vec2
	tick  int

	mini map[int]*ebiten.Image // обзорные картинки мини-карты по стороне

	// src — карта в исходном формате, из которой всё это собрано. Держится
	// целиком, потому что мир персонажа сохраняется как есть (см. Source):
	// вывести его заново из сида нельзя — правка генератора сделала бы из того
	// же сида другую карту, и сохранённый герой очнулся бы в чужом мире.
	src *worldgen.MapV1
}

// animCell — анимированный тайл разрежённого слоя.
type animCell struct {
	x, y   int
	sheet  uint8
	tile   uint16
	frames int
	stride int
	ticks  int // тиков на кадр
}

// Generate создаёт карту биома на сиде seed (сторона size в тайлах) и готовит
// её к игре. Генератор читает манифест биома из тех же ресурсов, что и всё
// остальное, поэтому карта каждого запуска — своя, а лишних файлов нет.
func Generate(l *assets.Loader, biome string, seed uint64, size int) (*Map, error) {
	root := path.Join("biomes", biome)
	man, err := worldgen.LoadManifestFS(l.FS(), root)
	if err != nil {
		return nil, fmt.Errorf("world: биом %q: %w", biome, err)
	}
	return New(l, worldgen.Generate(man, seed, size))
}

// New готовит к игре уже собранную карту: грузит листы тайлов, запекает слои и
// находит точку появления.
func New(l *assets.Loader, mv *worldgen.MapV1) (*Map, error) {
	if mv.Width <= 0 || mv.Height <= 0 || mv.TileSize <= 0 {
		return nil, fmt.Errorf("world: пустая карта")
	}
	// Карта из сохранения могла быть испечена прежней геометрией. Собрать её
	// новым кодом хуже, чем не собрать вовсе: она выглядит целой, но объекты в
	// ней стоят отдельно от своих барьеров. Пусть лучше пересоберётся из сида.
	if mv.Rev != worldgen.MapRev {
		return nil, fmt.Errorf("world: карта ревизии %d, нужна %d", mv.Rev, worldgen.MapRev)
	}
	field, err := navField(mv)
	if err != nil {
		return nil, err
	}

	m := &Map{
		src:     mv,
		biome:   mv.Biome,
		seed:    mv.Seed,
		w:       mv.Width,
		h:       mv.Height,
		ts:      mv.TileSize,
		field:   field,
		ground:  mv.Layers.Ground,
		mud:     mv.Layers.Mud,
		plateau: mv.Layers.Plateau,
	}

	root := path.Join("biomes", mv.Biome)
	for _, s := range mv.Sheets {
		img, err := l.Image(path.Join(root, s.File))
		if err != nil {
			return nil, fmt.Errorf("world: лист %q: %w", s.File, err)
		}
		m.sheets = append(m.sheets, img)
		m.cols = append(m.cols, max(s.Columns, 1))
	}

	roles, err := denseSheets(l, root, mv.Sheets)
	if err != nil {
		return nil, err
	}
	m.bake(mv, roles)
	if err := m.loadProps(l, mv); err != nil {
		return nil, err
	}
	m.spawn = m.findSpawn(mv.Markers)
	return m, nil
}

// navField собирает поле физики из nav-сетки карты. Сторона под-клетки —
// тайл/scale: сетка мельче тайла и уже сдвинута под картинку (см. buildNav).
func navField(mv *worldgen.MapV1) (*physics.Field, error) {
	n := mv.Nav
	if n.Scale <= 0 {
		return nil, fmt.Errorf("world: в карте нет сетки физики (nav.scale=%d)", n.Scale)
	}
	if n.Width != mv.Width*n.Scale || n.Height != mv.Height*n.Scale {
		return nil, fmt.Errorf("world: nav %dx%d не сходится с картой %dx%d при scale %d",
			n.Width, n.Height, mv.Width, mv.Height, n.Scale)
	}
	cells := make([]physics.Cell, len(n.Cells))
	for i, v := range n.Cells {
		if v > uint8(physics.Solid) {
			return nil, fmt.Errorf("world: nav: неизвестная клетка %d", v)
		}
		cells[i] = physics.Cell(v)
	}
	f := physics.NewField(n.Width, n.Height, float64(mv.TileSize)/float64(n.Scale), cells)
	if f == nil {
		return nil, fmt.Errorf("world: nav %d клеток при сетке %dx%d",
			len(n.Cells), n.Width, n.Height)
	}
	return f, nil
}

// denseSheets — индекс листа для каждой роли плотного слоя (ground, mud,
// plateau) по манифесту биома.
func denseSheets(l *assets.Loader, root string, sheets []worldgen.SheetRef) (map[string]int, error) {
	mp := path.Join(root, "manifest.json")
	b, err := fs.ReadFile(l.FS(), mp)
	if err != nil {
		return nil, fmt.Errorf("world: чтение %q: %w", mp, err)
	}
	var bm biomeManifest
	if err := json.Unmarshal(b, &bm); err != nil {
		return nil, fmt.Errorf("world: разбор %q: %w", mp, err)
	}
	out := map[string]int{}
	for role, t := range bm.Terrains {
		for i, s := range sheets {
			if s.Name == t.Sheet {
				out[role] = i
				break
			}
		}
	}
	return out, nil
}

// bake рисует все статические слои в один холст размером с карту. Дальше кадр
// стоит одну отрисовку куска холста: перебирать тайлы каждый кадр незачем —
// карта не меняется, а анимирована в ней только рябь на воде.
func (m *Map) bake(mv *worldgen.MapV1, roles map[string]int) {
	m.canvas = ebiten.NewImage(m.w*m.ts, m.h*m.ts)
	m.drawWater(mv)

	dense := func(data []uint16, role string) {
		si, ok := roles[role]
		if !ok || len(data) != m.w*m.h {
			return
		}
		for i, v := range data {
			if v == 0 {
				continue
			}
			m.blit(m.canvas, si, int(v)-1, i%m.w, i/m.w)
		}
	}
	sparse := func(cells []worldgen.SparseTile) {
		for _, c := range cells {
			m.blitRot(m.canvas, int(c.Sheet), int(c.Tile), c.X, c.Y, c.Rot)
		}
	}

	// Порядок снизу вверх — тот же, что у генератора (worldgen.spec §3):
	// тень плато лежит над нижней землёй, но под самим плато.
	dense(mv.Layers.Mud, "mud")
	dense(mv.Layers.Ground, "ground")
	sparse(mv.Layers.GroundSpots)
	sparse(mv.Layers.Coast)
	sparse(mv.Layers.Bridges) // брод по камням — поверх берега и воды
	sparse(mv.Layers.SurfaceLiquid)
	sparse(mv.Layers.PlateauShadow)
	dense(mv.Layers.Plateau, "plateau")
	sparse(mv.Layers.GroundDecor) // кустики травы — над плато: на макушке лежат на нём, на нижней земле его слой пуст
	sparse(mv.Layers.Cliff)
	sparse(mv.Layers.Stairs)
	sparse(mv.Layers.Hangers)

	// Рябь анимирована, поэтому в холст не запекается.
	for _, c := range mv.Layers.LiquidDetail {
		if c.Anim == nil || c.Anim.Frames <= 1 {
			m.blit(m.canvas, int(c.Sheet), int(c.Tile), c.X, c.Y)
			continue
		}
		m.detail = append(m.detail, animCell{
			x: c.X, y: c.Y, sheet: c.Sheet, tile: c.Tile,
			frames: c.Anim.Frames, stride: c.Anim.Stride,
			ticks: max(c.Anim.MS*config.TPS/1000, 1),
		})
	}
}

// drawWater заливает холст цветом воды по полосам глубины: сплошного водного
// тайла в наборах нет. Полоса пишется картинкой в один пиксель на тайл и
// растягивается — это одна отрисовка вместо 16 тысяч заливок.
func (m *Map) drawWater(mv *worldgen.MapV1) {
	pal := waterPalette(mv.WaterColors)
	deep := pal[len(pal)-1]
	m.canvas.Fill(deep)

	shade := mv.Layers.LiquidShade
	if len(shade) != m.w*m.h {
		return
	}
	img := image.NewRGBA(image.Rect(0, 0, m.w, m.h))
	for i, v := range shade {
		c := deep
		if v > 0 {
			c = pal[min(int(v)-1, len(pal)-1)]
		}
		img.Set(i%m.w, i/m.w, c)
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(float64(m.ts), float64(m.ts))
	m.canvas.DrawImage(ebiten.NewImageFromImage(img), op)
}

func waterPalette(cols [][]int) []color.RGBA {
	out := make([]color.RGBA, 0, len(cols))
	for _, c := range cols {
		if len(c) < 3 {
			continue
		}
		out = append(out, color.RGBA{uint8(c[0]), uint8(c[1]), uint8(c[2]), 0xff})
	}
	if len(out) == 0 {
		out = append(out, color.RGBA{0x35, 0x57, 0x78, 0xff})
	}
	return out
}

// blit рисует тайл localID листа sheet в клетку (tx,ty) приёмника.
func (m *Map) blit(dst *ebiten.Image, sheet, localID, tx, ty int) {
	m.blitRot(dst, sheet, localID, tx, ty, 0)
}

// blitRot — то же с поворотом на rot четвертей по часовой стрелке. Поворот
// кратен прямому углу, поэтому пиксели не интерполируются: тайл встаёт в свою
// же клетку, просто развёрнутым. Нужен там, где у художника есть набор одной
// ориентации, а на карте требуется другая (брод собран сверху вниз, а реку
// случается пересекать поперёк).
func (m *Map) blitRot(dst *ebiten.Image, sheet, localID, tx, ty int, rot uint8) {
	img := m.tile(sheet, localID)
	if img == nil {
		return
	}
	op := &ebiten.DrawImageOptions{}
	if r := rot % 4; r != 0 {
		s := float64(m.ts)
		op.GeoM.Translate(-s/2, -s/2)
		op.GeoM.Rotate(float64(r) * math.Pi / 2)
		op.GeoM.Translate(s/2, s/2)
	}
	op.GeoM.Translate(float64(tx*m.ts), float64(ty*m.ts))
	dst.DrawImage(img, op)
}

// tile — окно в атлас (SubImage, без копирования пикселей).
func (m *Map) tile(sheet, localID int) *ebiten.Image {
	if sheet < 0 || sheet >= len(m.sheets) || localID < 0 {
		return nil
	}
	src := m.sheets[sheet]
	cols := m.cols[sheet]
	x, y := (localID%cols)*m.ts, (localID/cols)*m.ts
	r := image.Rect(x, y, x+m.ts, y+m.ts).Add(src.Bounds().Min)
	if !r.In(src.Bounds()) {
		return nil
	}
	return src.SubImage(r).(*ebiten.Image)
}

// findSpawn — точка старта игрока: маркер spawn, а если его нет или герой в нём
// не помещается — ближайшее подходящее место от центра карты.
//
// Клетка маркера — та, что НАРИСОВАНА в этом месте, поэтому мировая точка
// берётся со сдвигом на полтайла: маркер стоит в координатах сетки уровней, а
// не картинки (см. buildNav).
func (m *Map) findSpawn(ms []worldgen.Marker) engine.Vec2 {
	center := func(tx, ty int) engine.Vec2 {
		return engine.Vec2{X: float64(tx*m.ts + m.ts), Y: float64(ty*m.ts + m.ts)}
	}
	for _, mk := range ms {
		if mk.Kind != "spawn" {
			continue
		}
		if p, ok := m.field.Place(center(mk.X, mk.Y), spawnBody, float64(8*m.ts)); ok {
			return p
		}
	}
	if p, ok := m.field.Place(center(m.w/2, m.h/2), spawnBody, float64(max(m.w, m.h)*m.ts)); ok {
		return p
	}
	return center(m.w/2, m.h/2)
}

// --- интерфейс карты для игрового слоя (mob.SpawnWorld) ---

// Field — сетка физики мира: по ней ходят и герой, и звери.
func (m *Map) Field() *physics.Field { return m.field }

// Biome — идентификатор биома (ключ в habitat.biomes видов).
func (m *Map) Biome() string { return m.biome }

// Seed — сид, на котором сгенерирована карта: по нему её можно повторить.
func (m *Map) Seed() int64 { return m.seed }

// Source — карта в исходном формате (map_format v1), из которой она собрана.
// Ею мир персонажа и сохраняется: тайлы, объекты, сетка физики и точка старта —
// всё, что делает мир этим миром, а не другим на том же сиде.
func (m *Map) Source() *worldgen.MapV1 { return m.src }

// Size — размеры мира в пикселях.
func (m *Map) Size() (float64, float64) {
	return float64(m.w * m.ts), float64(m.h * m.ts)
}

// Spawn — точка появления игрока.
func (m *Map) Spawn() engine.Vec2 { return m.spawn }

// Walkable — можно ли стоять в точке. Ответ без тела приблизительный (герой,
// утка и кабан проходят по-разному), поэтому это «сюда в принципе можно
// ступить»: не глубокая вода и не скала.
func (m *Map) Walkable(p engine.Vec2) bool {
	if _, ok := m.index(p); !ok {
		return false
	}
	switch m.field.CellAt(p) {
	case physics.Deep, physics.Solid:
		return false
	}
	return true
}

// Water — мелководье под точкой (по глубокой воде ходить нельзя, и до Water
// она не доходит).
func (m *Map) Water(p engine.Vec2) bool {
	return m.field.CellAt(p) == physics.Shallow
}

// Zone — что за место под точкой в терминах habitat.zones видов.
// Леса (woods) и двора (farmyard) на карте пока нет: пропсы генератор не
// расставляет, поселений в биоме нет — домашние виды поэтому и не заводятся.
func (m *Map) Zone(p engine.Vec2) string {
	i, ok := m.index(p)
	switch c := m.field.CellAt(p); {
	case !ok:
		return ""
	case c == physics.Deep || c == physics.Solid:
		return "water"
	case c == physics.Shallow:
		return "shore"
	case i < len(m.plateau) && m.plateau[i] != 0:
		return "plateau"
	case i < len(m.mud) && m.mud[i] != 0:
		return "trail"
	}
	return "meadow"
}

// index — номер клетки под мировой точкой.
func (m *Map) index(p engine.Vec2) (int, bool) {
	tx, ty := int(p.X)/m.ts, int(p.Y)/m.ts
	if p.X < 0 || p.Y < 0 || tx >= m.w || ty >= m.h {
		return 0, false
	}
	return ty*m.w + tx, true
}

// --- отрисовка ---

// Update продвигает анимацию карты (рябь на воде) на тик.
func (m *Map) Update() { m.tick++ }

// Draw рисует видимый камерой кусок карты.
func (m *Map) Draw(dst *ebiten.Image, cam *engine.Camera) {
	left := cam.Pos.X - cam.ViewW/2
	top := cam.Pos.Y - cam.ViewH/2
	x0 := clamp(int(left), 0, m.w*m.ts)
	y0 := clamp(int(top), 0, m.h*m.ts)
	x1 := clamp(int(left+cam.ViewW)+1, 0, m.w*m.ts)
	y1 := clamp(int(top+cam.ViewH)+1, 0, m.h*m.ts)
	if x1 <= x0 || y1 <= y0 {
		return
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(x0)-left, float64(y0)-top)
	dst.DrawImage(m.canvas.SubImage(image.Rect(x0, y0, x1, y1)).(*ebiten.Image), op)

	// Рябь: только то, что попало на экран.
	tx0, ty0, tx1, ty1 := x0/m.ts, y0/m.ts, x1/m.ts+1, y1/m.ts+1
	for _, c := range m.detail {
		if c.x < tx0 || c.x >= tx1 || c.y < ty0 || c.y >= ty1 {
			continue
		}
		frame := (m.tick / c.ticks) % c.frames
		img := m.tile(int(c.sheet), int(c.tile)+frame*c.stride)
		if img == nil {
			continue
		}
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(float64(c.x*m.ts)-left, float64(c.y*m.ts)-top)
		dst.DrawImage(img, op)
	}
}

func clamp(v, lo, hi int) int { return min(max(v, lo), hi) }
