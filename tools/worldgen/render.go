package main

// render.go — композит карты в PNG на настоящем арте. Слои рисуются снизу
// вверх (порядок §3), тайлы берутся из атласов, пропсы — из отдельных PNG
// и сортируются по sort_y. Это и есть главная валидация тула (6 разных сидов).

import (
	"image"
	"image/color"
	"image/draw"
	"path/filepath"
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

	blit := func(sheet string, localID, tx, ty int) {
		if localID < 0 {
			return
		}
		tile := a.Tile(sheet, localID)
		drawTileScaled(canvas, tile, tx*ts*scale, ty*ts*scale, scale)
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
			blit(sheet, int(v)-1, i%mp.Width, i/mp.Width)
		}
	}
	sparse := func(sl []SparseTile) {
		for _, st := range sl {
			blit(a.order[st.Sheet], int(st.Tile), st.X, st.Y)
		}
	}

	// точный порядок §3 снизу вверх: тень плато рисуется ПОД плато, но НАД
	// нижней землёй — поэтому слои чередуются, а не «все плотные, потом все разрежённые».
	// Всё, кроме воды-фона, — автотайлинг размеченных наборов (снизу вверх):
	sparse(mp.Layers.LiquidDetail)      // рябь на воде — сразу над водным фоном, под всей сушей
	dense(mp.Layers.Mud, "mud")         // грунт тропы — сплошная подложка ПОД травой
	dense(mp.Layers.Ground, "ground")   // grass_ground поверх грунта: он и вырезает тропу
	sparse(mp.Layers.Coast)             // grass_water/mud_water — берега (Water_coasts)
	sparse(mp.Layers.PlateauShadow)     // тень возвышенности (grass_shadow/mud_shadow) — НАД землёй, но ПОД плато: залита и под самой возвышенностью, иначе на кромках просветы
	dense(mp.Layers.Plateau, "plateau") // верх плато grass_cliff (зад/бока/интерьер)
	sparse(mp.Layers.Cliff)             // скала (spots_rock): grass_top-свес спереди + стенка — ПОВЕРХ grass_cliff
	sparse(mp.Layers.Stairs)            // лестницы врезаны в обрыв — рисуются ПОВЕРХ скалы и её свеса
	return canvas
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

// drawTileScaled рисует 16px тайл в позицию с целочисленным масштабом (nearest).
func drawTileScaled(dst *image.RGBA, tile *image.RGBA, dx, dy, scale int) {
	b := tile.Bounds()
	if scale == 1 {
		draw.Draw(dst, image.Rect(dx, dy, dx+b.Dx(), dy+b.Dy()), tile, b.Min, draw.Over)
		return
	}
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			c := tile.RGBAAt(b.Min.X+x, b.Min.Y+y)
			if c.A == 0 {
				continue
			}
			px, py := dx+x*scale, dy+y*scale
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
