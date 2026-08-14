package scene

// Журнал персонажа: кто он, где играет и кого убил.
//
// Открывается и из паузы (тогда показывает свежий снимок забега), и с экрана
// персонажей (тогда — то, что лежит в файле). Данные берутся из одной и той же
// записи save.Char, поэтому обе двери ведут к одному и тому же.
//
// Счёт убитых ведётся по видам, а не одним числом: «тетеревов пять, гоблинов
// один» — это память об игре, а «убито шесть» — не память ни о чём. Названия
// видов достаются из тех же таблиц, что и сами твари; вид, которого в таблицах
// больше нет, показывается своим идентификатором, а не прячется.

import (
	"fmt"
	"sort"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/vladislav/game/internal/assets"
	"github.com/vladislav/game/internal/character"
	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/mob"
	"github.com/vladislav/game/internal/progress"
	"github.com/vladislav/game/internal/save"
	"github.com/vladislav/game/internal/ui"
)

// Геометрия разворота (логическое разрешение 640×360): слева сведения, справа
// список убитых в две колонки.
const (
	jrX, jrY   = 40, 46
	jrW, jrH   = 560, 268
	jrPad      = 12
	jrLeftW    = 200
	jrRowH     = 13
	jrColGap   = 14
	jrKillCols = 2
)

// killRow — строка счётчика: название вида и сколько их.
type killRow struct {
	title string
	n     int
}

// Journal — экран журнала.
type Journal struct {
	back Scene
	c    *save.Char

	body  string // название тела героя
	kills []killRow
	page  int
	rows  int // строк в колонке (считается по высоте разворота)
}

// newJournal собирает журнал по записи персонажа. Таблицы видов читаются
// заново: журнал открывают редко, а зависеть от того, кто его открыл (забег или
// экран персонажей), он не должен.
func newJournal(l *assets.Loader, c *save.Char, back Scene) *Journal {
	j := &Journal{back: back, c: c, body: c.Body}
	if cat, err := character.Load(l.FS(), "character/character.json"); err == nil {
		if b := cat.Body(c.Body); b != nil && b.Title.RU != "" {
			j.body = b.Title.RU
		}
	}
	j.kills = killRows(l, c.Kills)
	j.rows = (jrH - 2*jrPad - 24) / jrRowH
	return j
}

// killRows переводит счётчики в строки с названиями, от частых к редким.
func killRows(l *assets.Loader, kills map[string]int) []killRow {
	species, _ := mob.LoadSpecies(l.FS(), animalsRoot+"/species.json")
	enemies, _ := mob.LoadEnemies(l.FS(), enemiesRoot+"/enemies.json")
	bosses, _ := mob.LoadEnemies(l.FS(), bossesRoot+"/bosses.json")

	title := func(key string) string {
		kind, id, tier, ok := save.SplitKill(key)
		if !ok {
			return key
		}
		if kind == save.KindAnimal {
			if species != nil {
				if sp := species.Get(id); sp != nil && sp.Title.RU != "" {
					return sp.Title.RU
				}
			}
			return id
		}
		for _, cat := range []*mob.EnemyCatalog{enemies, bosses} {
			if cat == nil {
				continue
			}
			if t := cat.Types[id]; t != nil {
				if tr := t.Tiers[tier]; tr != nil && tr.Title.RU != "" {
					return tr.Title.RU
				}
			}
		}
		return id + " " + tier
	}

	out := make([]killRow, 0, len(kills))
	for key, n := range kills {
		out = append(out, killRow{title: title(key), n: n})
	}
	// Частые сверху, равные — по алфавиту: список не должен прыгать от запуска
	// к запуску (обход карты счётчиков случаен).
	sort.Slice(out, func(a, b int) bool {
		if out[a].n != out[b].n {
			return out[a].n > out[b].n
		}
		return out[a].title < out[b].title
	})
	return out
}

// pages — сколько страниц занимает список убитых (минимум одна, пусть пустая).
func (j *Journal) pages() int {
	per := j.perPage()
	if per <= 0 || len(j.kills) == 0 {
		return 1
	}
	return (len(j.kills) + per - 1) / per
}

func (j *Journal) perPage() int { return j.rows * jrKillCols }

func (j *Journal) Update() (Scene, error) {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) || keyPressed(ebiten.KeyJ) {
		return j.backScene(), nil
	}
	if keyPressed(ebiten.KeyRight, ebiten.KeyD) {
		j.page = (j.page + 1) % j.pages()
	}
	if keyPressed(ebiten.KeyLeft, ebiten.KeyA) {
		j.page = (j.page - 1 + j.pages()) % j.pages()
	}
	return j, nil
}

func (j *Journal) backScene() Scene {
	if j.back != nil {
		return j.back
	}
	return NewMenu()
}

func (j *Journal) Draw(screen *ebiten.Image) {
	drawMenuBack(screen)
	ui.PixelTextCentered(screen, "ЖУРНАЛ", config.ScreenW/2, 20, 2, menuTitle)

	vector.FillRect(screen, jrX, jrY, jrW, jrH, menuPlate, false)
	vector.StrokeRect(screen, jrX+0.5, jrY+0.5, jrW-1, jrH-1, 1, menuEdge, false)
	// Разделитель разворота: слева про героя, справа про убитых.
	vector.FillRect(screen, jrX+jrLeftW, jrY+jrPad, 1, jrH-2*jrPad, menuFrame, false)

	j.drawAbout(screen)
	j.drawKills(screen)

	hint := "ESC - НАЗАД"
	if j.pages() > 1 {
		hint = fmt.Sprintf("СТРАНИЦА %d/%d,  СТРЕЛКИ - ЛИСТАТЬ,  ESC - НАЗАД", j.page+1, j.pages())
	}
	ui.PixelTextCentered(screen, hint, config.ScreenW/2, config.ScreenH-22, 1, menuEdge)
	ui.Cursor = ui.CursorArrow
}

// drawAbout — левая половина: кто герой и чем живёт.
func (j *Journal) drawAbout(dst *ebiten.Image) {
	c := j.c
	x, y := float64(jrX+jrPad), float64(jrY+jrPad)
	ui.PixelText(dst, ui.PixelTextFit(c.Name, jrLeftW-2*jrPad, 2), x, y, 2, menuTextSel)
	y += ui.PixelTextHeight(2) + 8

	xp := fmt.Sprintf("%d", c.XP)
	if n := progress.Need(c.Level); n > 0 {
		xp = fmt.Sprintf("%d / %d", c.XP, n)
	}
	rows := [][2]string{
		{"ТЕЛО", j.body},
		{"УРОВЕНЬ", fmt.Sprintf("%d", c.Level)},
		{"ОПЫТ", xp},
		{"ОЧКИ", fmt.Sprintf("%d", c.Points)},
		{"", ""},
		{"БИОМ", c.Biome},
		{"СИД КАРТЫ", fmt.Sprintf("%d", c.Seed)},
		{"", ""},
		{"УБИТО ВСЕГО", fmt.Sprintf("%d", c.KillTotal())},
		{"ВИДОВ", fmt.Sprintf("%d", len(j.kills))},
		{"СМЕРТЕЙ", fmt.Sprintf("%d", c.Deaths)},
		{"В ИГРЕ", playtime(c.Playtime)},
	}
	for _, r := range rows {
		if r[0] == "" {
			y += jrRowH / 2
			continue
		}
		ui.PixelText(dst, r[0], x, y, 1, menuEdge)
		v := ui.PixelTextFit(r[1], jrLeftW-2*jrPad-70, 1)
		ui.PixelText(dst, v, float64(jrX+jrLeftW)-float64(jrPad)-ui.PixelTextWidth(v, 1), y, 1, menuText)
		y += jrRowH
	}
}

// drawKills — правая половина: кого этот герой извёл.
func (j *Journal) drawKills(dst *ebiten.Image) {
	x0 := float64(jrX + jrLeftW + jrPad)
	y0 := float64(jrY + jrPad)
	ui.PixelText(dst, "УБИТО ПО ВИДАМ", x0, y0, 1, menuTitle)
	y0 += ui.PixelTextHeight(1) + 8

	if len(j.kills) == 0 {
		ui.PixelText(dst, "ПОКА НИКОГО", x0, y0, 1, menuEdge)
		return
	}

	colW := (float64(jrW) - float64(jrLeftW) - 2*jrPad - jrColGap) / jrKillCols
	start := j.page * j.perPage()
	for i := 0; i < j.perPage() && start+i < len(j.kills); i++ {
		r := j.kills[start+i]
		cx := x0 + float64(i/j.rows)*(colW+jrColGap)
		cy := y0 + float64(i%j.rows)*jrRowH
		n := fmt.Sprintf("%d", r.n)
		ui.PixelText(dst, ui.PixelTextFit(r.title, colW-ui.PixelTextWidth(n, 1)-6, 1), cx, cy, 1, menuText)
		ui.PixelText(dst, n, cx+colW-ui.PixelTextWidth(n, 1), cy, 1, menuTextSel)
	}
}
