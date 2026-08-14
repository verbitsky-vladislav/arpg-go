package main

// Разбор .tmx: единственный источник правды про анимации в этих паках.
//
// Художник кладёт кадры блоками в один лист, а Tiled хранит анимацию как
// последовательность tileid с длительностями. Шаг между кадрами (stride в
// тайлах) и говорит, как лист сложен:
//
//	stride кратен columns  -> кадры лежат друг под другом, кадр = полоса
//	                          высотой (stride/columns) тайлов во всю ширину
//	иначе                  -> кадры лежат в ряд, кадр = stride тайлов шириной
//
// Имя листа берём из <image source=...>, а не из name= тайлсета: у CraftPix
// имена тайлсетов дублируются (Fish2.png лежит в тайлсете с name="Fish1").

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type tmxMap struct {
	Tilesets []tmxTileset `xml:"tileset"`
}

type tmxTileset struct {
	Name      string `xml:"name,attr"`
	TileW     int    `xml:"tilewidth,attr"`
	TileH     int    `xml:"tileheight,attr"`
	Columns   int    `xml:"columns,attr"`
	TileCount int    `xml:"tilecount,attr"`
	Image     struct {
		Source string `xml:"source,attr"`
		W      int    `xml:"width,attr"`
		H      int    `xml:"height,attr"`
	} `xml:"image"`
	Tiles []struct {
		ID        int `xml:"id,attr"`
		Animation struct {
			Frames []struct {
				TileID   int `xml:"tileid,attr"`
				Duration int `xml:"duration,attr"`
			} `xml:"frame"`
		} `xml:"animation"`
	} `xml:"tile"`
}

type animSpec struct {
	frames   int
	frameW   int
	frameH   int
	ms       int
	sequence []int
	objects  int // сколько тайлов листа анимировано этим же ритмом
}

// pickAnim выбирает первый вариант кадровки, который сходится с размером
// листа. Кандидатов больше одного, когда часть тайлов объекта пустует не во
// всех кадрах: у них меньше уникальных tileid, и раскладка выходит другая.
func pickAnim(cands []*animSpec, w, h int) *animSpec {
	for _, c := range cands {
		if c.check(w, h) {
			return c
		}
	}
	return nil
}

func (a *animSpec) check(w, h int) bool {
	if a == nil || a.frames < 2 || a.frameW <= 0 || a.frameH <= 0 {
		return false
	}
	if a.frameW == w {
		return a.frames*a.frameH == h
	}
	if a.frameH == h {
		return a.frames*a.frameW == w
	}
	return false
}

func (a *animSpec) manifest(defMS int) *Anim {
	ms := a.ms
	if ms <= 0 {
		ms = defMS
	}
	out := &Anim{Frames: a.frames, Frame: Size{a.frameW, a.frameH}, MS: ms}
	// последовательность пишем только если она не совпадает с 0,1,2,…
	plain := len(a.sequence) == a.frames
	for i, v := range a.sequence {
		if i >= a.frames || v != i {
			plain = false
			break
		}
	}
	if !plain {
		out.Sequence = a.sequence
	}
	return out
}

// tmxAnims: имя листа (без расширения, в нижнем регистре) -> кандидаты на
// раскладку кадров, от самого массового к редкому.
func tmxAnims(dir string) (map[string][]*animSpec, error) {
	out := map[string][]*animSpec{}
	ents, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	for _, e := range ents {
		if strings.ToLower(filepath.Ext(e.Name())) != ".tmx" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var m tmxMap
		if err := xml.Unmarshal(raw, &m); err != nil {
			return nil, err
		}
		for _, ts := range m.Tilesets {
			if ts.Image.Source == "" || ts.Columns == 0 {
				continue
			}
			cands := tilesetAnims(ts)
			if len(cands) == 0 {
				continue
			}
			key := strings.ToLower(strings.TrimSuffix(filepath.Base(ts.Image.Source), filepath.Ext(ts.Image.Source)))
			out[key] = append(out[key], cands...)
		}
	}
	return out, nil
}

func tilesetAnims(ts tmxTileset) []*animSpec {
	type variant struct {
		stride, frames, ms int
		seq                []int
		count              int
	}
	byShape := map[[3]int]*variant{}
	for _, t := range ts.Tiles {
		fr := t.Animation.Frames
		if len(fr) < 2 {
			continue
		}
		// уникальные tileid в порядке возрастания = блоки кадров на листе
		seen := map[int]bool{}
		var ids []int
		for _, f := range fr {
			if !seen[f.TileID] {
				seen[f.TileID] = true
				ids = append(ids, f.TileID)
			}
		}
		if len(ids) < 2 {
			continue
		}
		sort.Ints(ids)
		stride := 0
		for i := 1; i < len(ids); i++ {
			stride = gcd(stride, ids[i]-ids[i-1])
		}
		if stride <= 0 {
			continue
		}
		// порядок проигрывания в индексах кадров
		idx := map[int]int{}
		for i, id := range ids {
			idx[id] = i
		}
		seq := make([]int, 0, len(fr))
		for _, f := range fr {
			seq = append(seq, idx[f.TileID])
		}
		key := [3]int{stride, len(ids), fr[0].Duration}
		v := byShape[key]
		if v == nil {
			v = &variant{stride: stride, frames: len(ids), ms: fr[0].Duration, seq: seq}
			byShape[key] = v
		}
		v.count++
	}
	// Порядок строгий: сначала самая массовая раскладка (на листе с десятком
	// объектов ритм у всех один), при равенстве — та, где кадров больше.
	// Без сортировки победитель зависел бы от обхода map.
	vs := make([]*variant, 0, len(byShape))
	for _, v := range byShape {
		vs = append(vs, v)
	}
	sort.Slice(vs, func(i, j int) bool {
		a, b := vs[i], vs[j]
		if a.count != b.count {
			return a.count > b.count
		}
		if a.frames != b.frames {
			return a.frames > b.frames
		}
		return a.stride < b.stride
	})
	out := make([]*animSpec, 0, len(vs))
	for _, v := range vs {
		spec := &animSpec{frames: v.frames, ms: v.ms, sequence: v.seq, objects: v.count}
		if v.stride%ts.Columns == 0 {
			spec.frameW = ts.Image.W
			spec.frameH = (v.stride / ts.Columns) * ts.TileH
		} else {
			spec.frameW = v.stride * ts.TileW
			spec.frameH = ts.Image.H
		}
		out = append(out, spec)
	}
	return out
}

func gcd(a, b int) int {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	for b != 0 {
		a, b = b, a%b
	}
	return a
}
