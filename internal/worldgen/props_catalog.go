package worldgen

// props_catalog.go — автосборка списка пропсов из PNG/Objects_separated:
// размеры меряются из самих PNG (спека врёт про футпринты), тип и параметры
// выводятся из имени файла. Результат кладётся в manifest.json (emit).

import (
	"image"
	_ "image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// propsDir — каталог отдельных спрайтов объектов и префикс пути в манифесте.
// Новая структура биома: props/. Старая (PNG/Objects_separated) — как запасной.
func propsDir(biomeDir string) (dir, prefix string) {
	if fi, err := os.Stat(filepath.Join(biomeDir, "props")); err == nil && fi.IsDir() {
		return filepath.Join(biomeDir, "props"), "props/"
	}
	return filepath.Join(biomeDir, "PNG", "Objects_separated"), "PNG/Objects_separated/"
}

// scanProps сканирует каталог спрайтов и возвращает пропсы, отсортированные
// по id (детерминированно).
func scanProps(biomeDir string) []Prop {
	dir, prefix := propsDir(biomeDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	fileset := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".png") {
			continue
		}
		names = append(names, e.Name())
		fileset[e.Name()] = true
	}
	sort.Strings(names)

	var props []Prop
	for _, n := range names {
		base := strings.TrimSuffix(n, filepath.Ext(n))
		lower := strings.ToLower(base)
		// пропускаем служебное и «ground»-варианты камней (они — цель swap)
		if strings.HasPrefix(lower, "layer") {
			continue
		}
		if strings.Contains(lower, "stone_ground") {
			continue
		}
		// reeds уходят в surface (камыш на мелководье), не в пропсы
		if strings.HasPrefix(lower, "reeds") {
			continue
		}
		// полоса кадров — не отдельный объект, её подхватит сам пропс
		if strings.HasSuffix(lower, "_anim") {
			continue
		}

		path := filepath.Join(dir, n)
		w, h := pngTiles(path)
		if w == 0 {
			continue
		}
		body, bp := propFoot(path)
		p := Prop{
			ID:        base,
			File:      prefix + n,
			Footprint: [2]int{w, h},
			Anchor:    propAnchorCell(bp, w, h),
			Body:      body,
			Base:      bp,
			On:        []string{"ground_a"},
			Weight:    10,
		}
		classifyProp(lower, base, fileset, prefix, &p)
		// покачивание: рядом лежит вертикальная полоса кадров того же спрайта
		if anim := prefix + base + "_anim.png"; fileset[base+"_anim.png"] {
			if fw, fh := pngSize(filepath.Join(dir, base+"_anim.png")); fw > 0 && fw == w*16 {
				p.File = anim
				p.Anim = &PropAnim{Frames: fh / (h * 16), MS: propAnimMS}
			}
		}
		props = append(props, p)
	}
	return props
}

// propAnimMS — период кадра покачивания. Лист художника — 11-13 кадров, при
// 150 мс полный оборот занимает около двух секунд: дерево дышит, а не трясётся.
const propAnimMS = 150

// classifyProp назначает пропсу группу, а из группы — правила размещения.
// Имя файла даёт только СМЫСЛ объекта; проходимость решает замеренное тело.
func classifyProp(lower, base string, fileset map[string]bool, prefix string, p *Prop) {
	switch {
	case strings.HasPrefix(lower, "ruin"):
		p.Group = "ruin"
		p.Prefab = true
	case strings.HasPrefix(lower, "tree"), strings.HasPrefix(lower, "broken_tree"):
		p.Group = "trunk"
	case strings.HasPrefix(lower, "bush"):
		p.Group = "bush"
	case strings.Contains(lower, "stone"):
		// глыба, кучка камней и галька живут по-разному: первая — валун в рост
		// человека, последняя — под ногами
		switch {
		case p.Footprint[0] >= 4:
			p.Group = "boulder"
		case p.Body > propLitterBody:
			p.Group = "rubble"
		default:
			p.Group = "litter"
		}
		// подмена травяного камня на «земляной» под ground_b
		if strings.Contains(lower, "stone_grass") {
			cand := strings.Replace(base, "grass", "ground", 1) + ".png"
			if fileset[cand] {
				p.SwapOnGroundB = prefix + cand
			}
		}
	default:
		p.Group = "litter" // грибы и всё мелкое
	}
	// мелочь по телу переезжает в litter независимо от вида: пень в 19 px и
	// гриб в 19 px ощущаются одинаково, то есть никак
	if p.Group != "ruin" && p.Body > 0 && p.Body <= propLitterBody {
		p.Group = "litter"
	}
	if gr, ok := propGroupByName(p.Group); ok {
		p.Collides = gr.Collides
		p.OnTrail = gr.OnTrail
		p.On = gr.On
		p.Weight = gr.Weight
	}
}

// propAnchorCell — клетка футпринта, которой объект СТОИТ на земле: та, куда
// попала точка опоры рисунка.
//
// Раньше якорем считался низ-центр холста, и для лежащего бревна это было на три
// клетки ниже самого бревна: генератор проверял землю, тропу и лестницу под
// пустотой, а сортировал объект по глубине так, будто он стоит там, где его не
// видно. Якорь по рисунку возвращает всем этим проверкам смысл.
//
// Соглашение прежнее: контактная клетка — (X+Anchor[0], Y+Anchor[1]-1), поэтому
// по вертикали хранится номер строки плюс один.
func propAnchorCell(base [2]int, w, h int) [2]int {
	const ts = 16
	ax := base[0] / ts
	ay := (base[1]-1)/ts + 1
	if ax < 0 {
		ax = 0
	}
	if ax > w-1 {
		ax = w - 1
	}
	if ay < 1 {
		ay = 1
	}
	if ay > h {
		ay = h
	}
	return [2]int{ax, ay}
}

// propFoot — пятно, которым объект стоит на земле, замеренное по самому рисунку:
// его ширина (body) и точка опоры внутри спрайта (base).
//
// Меряется САМАЯ ШИРОКАЯ строка нижней трети рисунка — это и есть то, обо что
// игрок стукается, в отличие от габарита картинки (у дерева крона вчетверо шире
// ствола). Её ширина даёт body, её середина — X опоры, а низ рисунка — Y опоры.
//
// Два прежних промаха, оба видны на лежащем бревне:
//   - холст врёт. Рисунок почти никогда не доходит до его низа (у Broken_tree1 —
//     на 46 px выше и на 5 px левее центра), и барьер по низу-центру холста
//     уезжал ниже и правее самого бревна;
//   - нижние строки рисунка тоже врут. У лежащего наискось бревна ниже всех
//     оказывается один его конец, и барьер, выстроенный по нему, съезжал вбок с
//     толстой части ствола. Широкая строка — это ствол целиком.
func propFoot(path string) (body int, base [2]int) {
	f, err := os.Open(path)
	if err != nil {
		return 0, [2]int{}
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		return 0, [2]int{}
	}
	b := src.Bounds()
	// границы строк рисунка
	rowLo := make([]int, b.Dy())
	rowHi := make([]int, b.Dy())
	top, bottom := -1, -1
	for y := b.Min.Y; y < b.Max.Y; y++ {
		lo, hi := b.Max.X, -1
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := src.At(x, y).RGBA(); a > 0 {
				if x < lo {
					lo = x
				}
				if x > hi {
					hi = x
				}
			}
		}
		rowLo[y-b.Min.Y], rowHi[y-b.Min.Y] = lo, hi
		if hi >= 0 {
			if top < 0 {
				top = y
			}
			bottom = y
		}
	}
	if top < 0 {
		return 0, [2]int{b.Dx() / 2, b.Dy()}
	}
	bestW, bestMid := 0, (b.Dx())/2
	for y := bottom - (bottom-top)/3; y <= bottom; y++ {
		lo, hi := rowLo[y-b.Min.Y], rowHi[y-b.Min.Y]
		if hi < 0 || hi-lo+1 <= bestW {
			continue
		}
		bestW, bestMid = hi-lo+1, (lo+hi)/2-b.Min.X
	}
	return bestW, [2]int{bestMid, bottom - b.Min.Y + 1}
}

// pngSize — размер PNG в пикселях.
func pngSize(path string) (int, int) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

// pngTiles возвращает размер PNG в тайлах (округление вверх к 16px).
func pngTiles(path string) (int, int) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0
	}
	tw := (cfg.Width + 15) / 16
	th := (cfg.Height + 15) / 16
	if tw < 1 {
		tw = 1
	}
	if th < 1 {
		th = 1
	}
	return tw, th
}
