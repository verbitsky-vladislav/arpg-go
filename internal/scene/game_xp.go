package scene

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/vladislav/game/internal/audio"
	"github.com/vladislav/game/internal/engine"
	"github.com/vladislav/game/internal/progress"
	"github.com/vladislav/game/internal/ui"
)

// Опыт героя в забеге: кто сколько дал и как это показано.
//
// Вся арифметика (цена уровня, уровень твари, отсечка по разнице уровней) живёт
// в internal/progress. Сцена только сводит: она одна знает и уровень героя, и
// то, кого он сейчас добил.

// reward начисляет опыт за добитую тварь уровня lvl, отдающую xp, и показывает
// это над точкой at.
//
// Сколько именно достанется, решает progress.Gain: переросшему добычу не
// достаётся ничего. Ноль тоже показывается — серым нулём: иначе игрок решит,
// что опыт просто не начислился.
func (g *Game) reward(at engine.Vec2, lvl, xp int) {
	got := progress.Gain(g.prog.Level, lvl, xp)
	g.fx.xp(at, got)
	if g.prog.Add(got) > 0 {
		g.fx.level(engine.Vec2{X: g.pl.Pos.X, Y: g.pl.Pos.Y - 46}, g.prog.Level)
		// Уровень — не про место в мире, а про игрока: звучит по центру и
		// поверх всего, как и надпись.
		g.sfx.play(audio.LevelUp)
	}
}

// drawTargetName рисует над целью строку «имя  УР N», центрируя её по cx.
//
// Уровень стоит рядом с именем, потому что от него зависит, стоит ли драка
// времени, а узнавать это из бестиария посреди боя игрок не пойдёт. Цвет
// уровня несёт смысл, и тот же самый, что и везде: голубой — как полоса опыта
// и как всплывающее «+N», серый — как ноль над трупом. Так «с этого опыта не
// будет» видно до драки, а не после неё.
//
// Имя не гаснет вместе с уровнем: кто именно тебя бьёт, важно независимо от
// того, платят за него или нет.
func (g *Game) drawTargetName(dst *ebiten.Image, name string, lvl int, cx, y float64) {
	// Правило разрыва спрашивается у самого progress.Gain, а не пересчитывается
	// здесь: показанное на экране и начисленное после удара обязаны совпасть.
	col := hudXP
	if progress.Gain(g.prog.Level, lvl, 1) == 0 {
		col = fxNoXP
	}

	lv := fmt.Sprintf("УР %d", lvl)
	nw := ui.PixelTextWidth(name+"  ", 1)
	x := cx - (nw+ui.PixelTextWidth(lv, 1))/2

	// Тень в пиксель, как под числами урона: подпись висит на чём попало — на
	// траве, на тропе, на самом мобе. Погашенному уровню она нужнее всех:
	// тусклый он по замыслу, а нечитаемым быть не должен.
	ui.PixelText(dst, name, x+1, y+1, 1, hudShadow)
	ui.PixelText(dst, lv, x+nw+1, y+1, 1, hudShadow)
	ui.PixelText(dst, name, x, y, 1, hudText)
	ui.PixelText(dst, lv, x+nw, y, 1, col)
}

// Полоса опыта под полосой здоровья. Тонкая нарочно: здоровье надо видеть
// краем глаза в драке, а опыт — между драками.
const (
	xpBarH   = 4
	xpBarGap = 3
)

// drawXPBar рисует полосу опыта и подпись «УР N» под полосой здоровья. x, y —
// левый верхний угол полосы, w — её ширина (та же, что у здоровья).
func (g *Game) drawXPBar(dst *ebiten.Image, x, y, w float32) {
	if frac := g.prog.Frac(); frac > 0 {
		vector.FillRect(dst, x, y, w*float32(frac), xpBarH, hudXP, false)
	}
	vector.StrokeRect(dst, x-0.5, y-0.5, w+1, xpBarH+1, 1, hudEdge, false)

	// Слева уровень, справа — остаток до следующего. Очки прокачки
	// показываются, только когда они есть: тратить их пока некуда, и пустой
	// счётчик в углу был бы обещанием, которого игра не сдержит.
	lvl := fmt.Sprintf("УР %d", g.prog.Level)
	if g.prog.Points > 0 {
		lvl += fmt.Sprintf("  +%d ОЧК", g.prog.Points)
	}
	ty := float64(y+xpBarH) + 3
	ui.PixelText(dst, lvl, float64(x), ty, 1, hudText)
	// Счётчик прижат к правому краю и уступает уровню: на поздних уровнях числа
	// разрастаются (14370 / 14370), и наезжать на подпись слева им нельзя.
	if n := progress.Need(g.prog.Level); n > 0 {
		right := fmt.Sprintf("%d / %d", g.prog.XP, n)
		rw := ui.PixelTextWidth(right, 1)
		if ui.PixelTextWidth(lvl, 1)+6+rw <= float64(w) {
			ui.PixelText(dst, right, float64(x+w)-rw, ty, 1, hudDim)
		}
	}
}

// xpBlockH — сколько места блок опыта занимает под полосой здоровья.
func xpBlockH() float32 { return xpBarGap + xpBarH + 6 + float32(ui.PixelTextHeight(1)) }
