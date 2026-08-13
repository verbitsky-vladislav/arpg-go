package main

// stage_shadow.go — тень возвышенности на нижней земле (worldgen.spec §5 шаг 12).
//
// Наборы grass_shadow/mud_shadow в Ground_grass.tsx — это НЕ подложка материала.
// Раньше их клали под каждую клетку суши как «underlay», и они просто затемняли
// всю карту ровным фоном; на деле это затенённый вариант травы и грунта, которым
// художник обводит возвышенность — см. эталон elevated_test.tsx.tmx.
//
// НАПРАВЛЕНИЕ. Тень не кольцо вокруг плато, а шлейф в одну сторону: источник
// стоит на юго-востоке, поэтому тень уходит на СЕВЕРО-ЗАПАД — вверх и чуть
// влево, а снизу и справа от возвышенности её нет вовсе. Вертикальная составляющая
// втрое длиннее горизонтальной: в самих тайлах перепад яркости сверху вниз
// (заливка обрыва 118 → 80, травяной гребень ~147 → ~82) втрое сильнее, чем
// поперёк (92 → 86), то есть источник близок к зениту и лишь немного смещён вбок.
//
// Тень строится протяжкой силуэта (плато вместе с телом обрыва) по вектору
// света: объединение сдвигов от (0,0) до (shadowOffsetX, shadowOffsetY). Нулевой
// сдвиг входит в объединение намеренно — тень заливается и ПОД самой
// возвышенностью, иначе на кромках и в проёмах силуэта остаются непрокрашенные
// просветы. Слой рисуется ПОД плато и скалой, они его закрывают.

const (
	shadowOffsetX = -1 // на запад
	shadowOffsetY = -3 // на север
)

// spanTo — диапазон смещений между нулём и v включительно, в возрастающем порядке.
func spanTo(v int) (int, int) {
	if v < 0 {
		return v, 0
	}
	return 0, v
}

func (g *Generator) stagePlateauShadow() {
	grass := g.Manifest.Terrains["ground_shadow"]
	mud := g.Manifest.Terrains["mud_shadow"]
	if len(grass.Corner) == 0 {
		return
	}
	W, H := g.P.Width, g.P.Height

	// силуэт возвышенности: верх плато вместе с телом обрыва
	sil := make([]bool, W*H)
	for i, lv := range g.Level.Data {
		sil[i] = lv == Plateau
	}
	for c := range g.Cliff {
		if g.Level.In(c[0], c[1]) {
			sil[c[1]*W+c[0]] = true
		}
	}
	// протяжка силуэта по вектору света. Смещения знаковые, поэтому диапазон
	// берётся между нулём и значением константы — направление задаётся её знаком.
	x0, x1 := spanTo(shadowOffsetX)
	y0, y1 := spanTo(shadowOffsetY)
	shadow := make([]bool, W*H)
	for i, on := range sil {
		if !on {
			continue
		}
		x, y := i%W, i/W
		for dy := y0; dy <= y1; dy++ {
			for dx := x0; dx <= x1; dx++ {
				nx, ny := x+dx, y+dy
				if nx >= 0 && ny >= 0 && nx < W && ny < H {
					shadow[ny*W+nx] = true
				}
			}
		}
	}

	inShadow := func(x, y int) bool { return g.Level.In(x, y) && shadow[y*W+x] }
	for y := 0; y <= H; y++ {
		for x := 0; x <= W; x++ {
			// на воде тени нет: там свой берег и заливка цветом
			if !g.Level.In(x, y) || g.Level.At(x, y).isLiquid() {
				continue
			}
			k := cornerKey(inShadow, x, y)
			if !anyCorner(k) {
				continue
			}
			t := grass
			if g.Trail[[2]int{x, y}] && len(mud.Corner) > 0 {
				t = mud // на грунтовой тропе тень своего материала
			}
			ids, ok := g.resolve(t.Wangset, t.Corner, k)
			if !ok {
				continue
			}
			g.addSparse("plateau_shadow", t.Sheet, x, y, variantAt(g.Seed, x, y, 1, ids), nil)
		}
	}
}
