package worldgen

// render.go — композит карты в PNG на настоящем арте. Слои рисуются снизу
// вверх (порядок §3), тайлы берутся из атласов, пропсы — из отдельных PNG
// и сортируются по sort_y. Это и есть главная валидация тула (6 разных сидов).

import (
	"image"
	"image/color"
	"image/draw"
	"path/filepath"
	"sort"
)

// denseRoleSheet — какой лист рисует каждый плотный слой (одна роль → один лист).
func (a *AtlasSet) denseRoleSheet(role string) (string, bool) {
	t, ok := a.m.Terrains[role]
	if !ok {
		return "", false
	}
	return t.Sheet, true
}

// RenderMap собирает изображение карты в масштабе scale (пиксель арта → scale px).
func RenderMap(mp *MapV1, a *AtlasSet, scale int) image.Image {
	ts := mp.TileSize
	W := mp.Width * ts * scale
	H := mp.Height * ts * scale
	canvas := image.NewRGBA(image.Rect(0, 0, W, H))

	blit := func(sheet string, localID, tx, ty int, rot uint8) {
		if localID < 0 {
			return
		}
		tile := a.Tile(sheet, localID)
		drawTileScaled(canvas, tile, tx*ts*scale, ty*ts*scale, scale, rot)
	}

	// фон — водный цвет (в Water_coasts нет сплошного водного тайла; берега —
	// полупрозрачные тайлы поверх этого цвета). Не заливка, а полосы глубины:
	// liquid_shade даёт на каждый тайл индекс
	// в палитре от мели к глубине. Нет слоя (карта старого формата) — вся вода
	// красится последним цветом палитры, как раньше.
	pal := waterPaletteRGBA(a.m)
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{pal[len(pal)-1]}, image.Point{}, draw.Src)
	if sh := mp.Layers.LiquidShade; len(sh) == mp.Width*mp.Height {
		for i, v := range sh {
			if v == 0 {
				continue
			}
			c := pal[clampInt(int(v)-1, 0, len(pal)-1)]
			fillBlock(canvas, (i%mp.Width)*ts*scale, (i/mp.Width)*ts*scale, ts*scale, c)
		}
	}

	dense := func(data []uint16, role string) {
		sheet, ok := a.denseRoleSheet(role)
		if !ok || data == nil {
			return
		}
		for i, v := range data {
			if v == 0 {
				continue
			}
			blit(sheet, int(v)-1, i%mp.Width, i/mp.Width, 0)
		}
	}
	sparse := func(sl []SparseTile) {
		for _, st := range sl {
			blit(a.order[st.Sheet], int(st.Tile), st.X, st.Y, st.Rot)
		}
	}

	// точный порядок §3 снизу вверх: тень плато рисуется ПОД плато, но НАД
	// нижней землёй — поэтому слои чередуются, а не «все плотные, потом все разрежённые».
	// Всё, кроме воды-фона, — автотайлинг размеченных наборов (снизу вверх):
	sparse(mp.Layers.LiquidDetail)      // рябь на воде — сразу над водным фоном, под всей сушей
	dense(mp.Layers.Mud, "mud")         // грунт тропы — сплошная подложка ПОД травой
	dense(mp.Layers.Ground, "ground")   // grass_ground поверх грунта: он и вырезает тропу
	sparse(mp.Layers.GroundSpots)       // напольные пятна на грунте троп
	sparse(mp.Layers.Coast)             // grass_water/mud_water — берега (Water_coasts)
	sparse(mp.Layers.Bridges)           // брод по камням — поверх берега и воды
	sparse(mp.Layers.SurfaceLiquid)     // кувшинки/камыш на воде
	sparse(mp.Layers.PlateauShadow)     // тень возвышенности (grass_shadow/mud_shadow) — НАД землёй, но ПОД плато: залита и под самой возвышенностью, иначе на кромках просветы
	dense(mp.Layers.Plateau, "plateau") // верх плато grass_cliff (зад/бока/интерьер)
	sparse(mp.Layers.GroundDecor)       // кустики травы — НАД плато: на макушке они лежат на нём, на нижней земле его слой пуст
	sparse(mp.Layers.Cliff)             // скала (spots_rock): grass_top-свес спереди + стенка — ПОВЕРХ grass_cliff
	sparse(mp.Layers.Stairs)            // лестницы врезаны в обрыв — рисуются ПОВЕРХ скалы и её свеса
	sparse(mp.Layers.Hangers)           // лианы со свеса обрыва
	drawProps(canvas, mp, a.m.dir, ts, scale)
	return canvas
}

// drawProps рисует объекты поверх тайлов в порядке sort_y (низ спрайта = глубина),
// при равном sort_y — по X, чтобы картинка не зависела от порядка расстановки.
func drawProps(dst *image.RGBA, mp *MapV1, biomeDir string, ts, scale int) {
	if len(mp.Props) == 0 {
		return
	}
	order := make([]int, len(mp.Props))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(i, j int) bool {
		a, b := mp.Props[order[i]], mp.Props[order[j]]
		if a.SortY != b.SortY {
			return a.SortY < b.SortY
		}
		return a.X < b.X
	})
	pc := &propCache{dir: biomeDir, imgs: map[string]*image.RGBA{}}
	for _, i := range order {
		p := mp.Props[i]
		img := pc.get(p.File)
		if img == nil {
			continue
		}
		// у анимированного пропса файл — вертикальная полоса кадров; в статичную
		// картинку идёт первый кадр
		if p.Frames > 1 {
			h := img.Bounds().Dy() / p.Frames
			img = img.SubImage(image.Rect(0, 0, img.Bounds().Dx(), h)).(*image.RGBA)
		}
		// футпримт задан в тайлах, спрайт может быть меньше кратного — выравниваем
		// по низу-центру футпринта, то есть по якорю, как считает генератор
		dx := p.X*ts*scale + (p.W*ts*scale-img.Bounds().Dx()*scale)/2
		dy := (p.Y+p.H)*ts*scale - img.Bounds().Dy()*scale
		drawTileScaled(dst, img, dx, dy, scale, 0)
	}
}

// waterPaletteRGBA — палитра воды манифеста в цветах рендера (мель → глубина).
func waterPaletteRGBA(m *Manifest) []color.RGBA {
	pal := m.waterPalette()
	out := make([]color.RGBA, 0, len(pal))
	for _, c := range pal {
		out = append(out, color.RGBA{c[0], c[1], c[2], 255})
	}
	return out
}

// drawTileScaled рисует 16px тайл в позицию с целочисленным масштабом (nearest)
// и поворотом на rot четвертей по часовой стрелке. Поворот кратен прямому углу,
// поэтому это перестановка пикселей без интерполяции.
func drawTileScaled(dst *image.RGBA, tile *image.RGBA, dx, dy, scale int, rot uint8) {
	b := tile.Bounds()
	if scale == 1 && rot%4 == 0 {
		draw.Draw(dst, image.Rect(dx, dy, dx+b.Dx(), dy+b.Dy()), tile, b.Min, draw.Over)
		return
	}
	w, h := b.Dx(), b.Dy()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := tile.RGBAAt(b.Min.X+x, b.Min.Y+y)
			if c.A == 0 {
				continue
			}
			// (x,y) исходного тайла → место в повёрнутом
			tx, ty := x, y
			switch rot % 4 {
			case 1:
				tx, ty = h-1-y, x
			case 2:
				tx, ty = w-1-x, h-1-y
			case 3:
				tx, ty = y, w-1-x
			}
			px, py := dx+tx*scale, dy+ty*scale
			for sy := 0; sy < scale; sy++ {
				for sx := 0; sx < scale; sx++ {
					dst.SetRGBA(px+sx, py+sy, c)
				}
			}
		}
	}
}

// propCache кэширует загруженные PNG пропсов на время рендера.
type propCache struct {
	dir  string
	imgs map[string]*image.RGBA
}

func (pc *propCache) get(file string) *image.RGBA {
	if img, ok := pc.imgs[file]; ok {
		return img
	}
	src, err := loadImage(filepath.Join(pc.dir, file))
	if err != nil {
		pc.imgs[file] = nil
		return nil
	}
	rgba := image.NewRGBA(src.Bounds())
	draw.Draw(rgba, src.Bounds(), src, src.Bounds().Min, draw.Src)
	// перенести в 0,0-основанный прямоугольник
	b := rgba.Bounds()
	norm := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(norm, norm.Bounds(), rgba, b.Min, draw.Src)
	pc.imgs[file] = norm
	return norm
}
