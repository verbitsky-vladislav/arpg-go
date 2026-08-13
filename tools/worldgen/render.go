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

	// фон — сплошной водный цвет (в Water_coasts нет сплошного водного тайла;
	// берега — полупрозрачные тайлы поверх этого цвета).
	wc := color.RGBA{53, 87, 120, 255}
	if len(a.m.WaterColor) == 3 {
		wc = color.RGBA{uint8(a.m.WaterColor[0]), uint8(a.m.WaterColor[1]), uint8(a.m.WaterColor[2]), 255}
	}
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{wc}, image.Point{}, draw.Src)

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
	dense(mp.Layers.GroundUnder, "ground_under") // grass_underlay (подложка травы)
	dense(mp.Layers.Ground, "ground")            // grass_ground (интерьер травы)
	dense(mp.Layers.MudUnder, "mud_under")       // mud_underlay (подложка тропы)
	dense(mp.Layers.Mud, "mud")                  // mud (тропа поверх травы)
	sparse(mp.Layers.Coast)                      // grass_water/mud_water — берега (Water_coasts)
	dense(mp.Layers.Plateau, "plateau")          // верх плато grass_cliff (зад/бока/интерьер)
	sparse(mp.Layers.Cliff)                      // скала (spots_rock): grass_top-свес спереди + стенка — ПОВЕРХ grass_cliff
	return canvas
}

// liquidRole — имя жидкой роли (solid для пещер).
func liquidRole(m *Manifest) string {
	if m.EdgeMode == "wall" {
		return "solid"
	}
	return "liquid"
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

func drawProps(dst *image.RGBA, mp *MapV1, m *Manifest, scale int) {
	pc := &propCache{dir: m.dir, imgs: map[string]*image.RGBA{}}
	ts := mp.TileSize
	for _, p := range mp.Props {
		img := pc.get(p.File)
		if img == nil {
			continue
		}
		// низ-центр футпринта совмещаем с якорем клетки
		px := (p.X * ts) * scale
		py := (p.Y * ts) * scale
		drawTileScaled(dst, img, px, py, scale)
	}
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
