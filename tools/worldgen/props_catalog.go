package main

// props_catalog.go — автосборка списка пропсов из PNG/Objects_separated:
// размеры меряются из самих PNG (спека врёт про футпринты), тип и параметры
// выводятся из имени файла. Результат кладётся в manifest.json (emit).

import (
	"image"
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

		w, h := pngTiles(filepath.Join(dir, n))
		if w == 0 {
			continue
		}
		p := Prop{
			ID:        base,
			File:      prefix + n,
			Footprint: [2]int{w, h},
			Anchor:    [2]int{w / 2, h},
			On:        []string{"ground_a"},
			Weight:    10,
		}
		classifyProp(lower, base, fileset, prefix, &p)
		props = append(props, p)
	}
	return props
}

// classifyProp настраивает параметры пропса по имени.
func classifyProp(lower, base string, fileset map[string]bool, prefix string, p *Prop) {
	switch {
	case strings.HasPrefix(lower, "tree"), strings.HasPrefix(lower, "broken_tree"):
		p.Collides = true
		p.Zone = []string{"dense", "mid"}
		p.Weight = 30
	case strings.HasPrefix(lower, "bush"):
		p.Collides = false
		p.Zone = []string{"dense", "mid", "open"}
		p.Weight = 24
	case strings.Contains(lower, "mushroom"):
		p.Collides = false
		p.Zone = []string{"dense"}
		p.Weight = 8
	case strings.Contains(lower, "stone"):
		p.Collides = false
		p.Zone = []string{"open", "plateau"}
		p.Weight = 14
		p.On = []string{"ground_a", "plateau"}
		// подмена травяного камня на «земляной» под ground_b
		if strings.Contains(lower, "stone_grass") {
			cand := strings.Replace(base, "grass", "ground", 1) + ".png"
			if fileset[cand] {
				p.SwapOnGroundB = prefix + cand
			}
		}
	case strings.HasPrefix(lower, "ruin"):
		p.Collides = true
		p.Prefab = true
		p.Zone = []string{"open", "mid"}
		p.Weight = 4
	default:
		p.Collides = false
		p.Zone = []string{"open", "mid"}
	}
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
