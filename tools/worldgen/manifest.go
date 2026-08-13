package main

// manifest.go — структуры manifest.json (роли → тайлы) и его загрузка.
// Генератор не знает слов «трава/лес», он знает роли; конкретные тайлы под роли
// даёт манифест биома. См. docs/worldgen/manifest.schema.md и worldgen.spec §2.

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Manifest — контракт одного биома.
type Manifest struct {
	ID          string                `json:"id"`
	TileSize    int                   `json:"tile_size"`
	RenderScale int                   `json:"render_scale"`
	EdgeMode    string                `json:"edge_mode"`   // "water" | "wall"
	WaterColor  []int                 `json:"water_color"` // RGB заливки воды (в Water_coasts нет сплошного водного тайла)
	WaterColors [][]int               `json:"water_colors"`
	Sheets      map[string]Sheet      `json:"sheets"`
	Terrains    map[string]Terrain    `json:"terrains"`
	Transitions map[string]Transition `json:"transitions"`
	Stairs      Stairs                `json:"stairs"`
	Hangers     Hangers               `json:"hangers"`
	Props       []Prop                `json:"props"`
	Surface     map[string][]string   `json:"surface"`
	Spots       SpotSet               `json:"spots"`

	dir string // каталог манифеста (для резолва путей к арту), не сериализуется
}

// SpotSet — набор напольных декалей (пятна травы/земли) для разбавления фона.
type SpotSet struct {
	Sheet    string `json:"sheet"`
	Variants []int  `json:"variants"`
}

// Sheet — метаданные атласа: файл + геометрия сетки. Anim задаётся, если ВЕСЬ
// лист анимирован (у воды кадры лежат вертикальными блоками с шагом stride
// тайлов): любой тайл этого листа уезжает в вывод со ссылкой на анимацию.
type Sheet struct {
	File    string `json:"file"`
	Columns int    `json:"columns"`
	Count   int    `json:"tilecount"`
	Anim    *Anim  `json:"anim,omitempty"`
}

// waterPalette — цвета воды от мели к глубине. Источник — water_colors; если его
// нет, палитра вырождается в один water_color (поведение до полос глубины).
func (m *Manifest) waterPalette() [][3]uint8 {
	rgb := func(v []int) ([3]uint8, bool) {
		if len(v) != 3 {
			return [3]uint8{}, false
		}
		return [3]uint8{uint8(v[0]), uint8(v[1]), uint8(v[2])}, true
	}
	var pal [][3]uint8
	for _, c := range m.WaterColors {
		if v, ok := rgb(c); ok {
			pal = append(pal, v)
		}
	}
	if len(pal) > 0 {
		return pal
	}
	if v, ok := rgb(m.WaterColor); ok {
		return [][3]uint8{v}
	}
	return [][3]uint8{{53, 87, 120}}
}

// waterPaletteRGB — та же палитра в виде, в котором она уезжает в map_format.
func (m *Manifest) waterPaletteRGB() [][]int {
	pal := m.waterPalette()
	out := make([][]int, 0, len(pal))
	for _, c := range pal {
		out = append(out, []int{int(c[0]), int(c[1]), int(c[2])})
	}
	return out
}

// Terrain — площадная роль. Blob: маска-строка "0".."46" → локальный id тайла.
// Edges (для cliff): 4-битная маска "0".."15" → id. Fill — запасной ровный тайл.
// Corner: угловая схема, ключ "NW,NE,SE,SW" (1/0) → id. Wangset: имя углового
// набора в сиблинг .tsx листа; если задано, Corner подгружается из разметки при
// загрузке манифеста (источник истины — ручная разметка в Tiled).
type Terrain struct {
	Sheet   string           `json:"sheet"`
	Fill    int              `json:"fill"`
	Blob    map[string]int   `json:"blob,omitempty"`
	Edges   map[string]int   `json:"edges,omitempty"`
	Corner  map[string][]int `json:"corner,omitempty"` // ключ→ВСЕ варианты тайла
	Wangset string           `json:"wangset,omitempty"`
	Anim    *Anim            `json:"anim,omitempty"`
}

// Transition — переход между ролями. Set: 4-битная маска "0".."15" → id.
type Transition struct {
	Sheet string         `json:"sheet"`
	Set   map[string]int `json:"set"`
	Anim  *Anim          `json:"anim,omitempty"`
}

// Anim — анимация (кадры в атласе с вертикальным шагом stride).
type Anim struct {
	Frames int `json:"frames"`
	Stride int `json:"stride"`
	MS     int `json:"ms"`
}

// Stairs — лестницы. Материал описан тремя угловыми наборами .tsx: ряд у
// макушки, повторяемое тело и ряд у подножия. Лестница нужной высоты набирается
// из них, тело при необходимости повторяется; материал всегда один на лестницу.
//
// Осторожно с именами наборов художника: они названы по ходу ПОДЪЁМА, то есть
// «start» лежит внизу у подножия, а «end» — наверху у макушки. В манифесте роли
// названы по месту на карте (top/bottom), чтобы не путать порядок укладки.
// Раскладка сверена с эталоном elevated_test.tsx.tmx, где лестница врезана в
// центр южного обрыва.
type Stairs struct {
	Sheet     string                   `json:"sheet"`
	Width     int                      `json:"width"`
	Materials map[string]StairMaterial `json:"materials"`

	kits map[string][][]int // материал → ряды сверху вниз, заполняется при загрузке
}

// StairMaterial — угловые наборы одного материала лестницы, по месту на карте.
type StairMaterial struct {
	Top    string `json:"top"`    // ряд, примыкающий к макушке плато
	Block  string `json:"block"`  // тело, повторяется по высоте обрыва
	Bottom string `json:"bottom"` // ряд у подножия
}

// stairMaterials — имена материалов в устойчивом порядке (map сам по себе
// перебирается случайно, а выбор материала обязан зависеть только от сида).
func (s *Stairs) stairMaterials() []string {
	names := make([]string, 0, len(s.kits))
	for n := range s.kits {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// loadStairs разбирает наборы лестниц из .tsx листа Stairs.Sheet: тайлы набора
// режутся на ряды по Width в порядке разметки (сверху вниз, слева направо).
func (m *Manifest) loadStairs() error {
	st := &m.Stairs
	if st.Sheet == "" || len(st.Materials) == 0 {
		return nil
	}
	sh, ok := m.Sheets[st.Sheet]
	if !ok {
		return fmt.Errorf("stairs: нет листа %s", st.Sheet)
	}
	tsxPath := filepath.Join(m.dir, strings.TrimSuffix(sh.File, filepath.Ext(sh.File))+".tsx")
	sets, err := parseWangCorners(tsxPath)
	if err != nil {
		return fmt.Errorf("stairs: %w", err)
	}
	st.kits = map[string][][]int{}
	for name, mat := range st.Materials {
		var rows [][]int
		for _, set := range []string{mat.Top, mat.Block, mat.Bottom} {
			tbl, ok := sets[set]
			if !ok {
				return fmt.Errorf("stairs %s: в %s нет набора %q",
					name, filepath.Base(tsxPath), set)
			}
			ids := tbl["1,1,1,1"]
			if len(ids) < st.Width {
				return fmt.Errorf("stairs %s: в наборе %q %d тайлов при ширине %d",
					name, set, len(ids), st.Width)
			}
			rows = append(rows, ids[:st.Width])
		}
		st.kits[name] = rows
	}
	return nil
}

// Hangers — свесы (лианы) на южной грани обрыва.
type Hangers struct {
	Sheet    string  `json:"sheet"`
	Variants [][]int `json:"variants"`
	On       string  `json:"on"`
}

// Prop — объект. Footprint/Anchor опциональны: если 0, меряются из PNG.
type Prop struct {
	ID            string   `json:"id"`
	File          string   `json:"file"`
	Footprint     [2]int   `json:"footprint,omitempty"`
	Anchor        [2]int   `json:"anchor,omitempty"`
	Collides      bool     `json:"collides"`
	On            []string `json:"on"`
	Zone          []string `json:"zone,omitempty"`
	Weight        int      `json:"weight"`
	SwapOnGroundB string   `json:"swap_on_ground_b,omitempty"`
	Prefab        bool     `json:"prefab,omitempty"`
}

// LoadManifest читает manifest.json из каталога биома.
func LoadManifest(biomeDir string) (*Manifest, error) {
	path := filepath.Join(biomeDir, "manifest.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("манифест: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("манифест %s: %w", path, err)
	}
	m.dir = biomeDir
	if m.TileSize == 0 {
		m.TileSize = 16
	}
	if m.RenderScale == 0 {
		m.RenderScale = 2
	}
	if err := m.loadWangsets(); err != nil {
		return nil, err
	}
	if err := m.loadStairs(); err != nil {
		return nil, err
	}
	return &m, nil
}

// loadWangsets заполняет Terrain.Corner из угловых наборов .tsx для ролей,
// у которых задан Wangset. Разметка сделана вручную в Tiled рядом с листом
// (Ground_grass.png → Ground_grass.tsx) и служит источником истины.
func (m *Manifest) loadWangsets() error {
	cache := map[string]map[string]map[string][]int{} // tsxPath → wangset → corner-таблица(варианты)
	for role, t := range m.Terrains {
		if t.Wangset == "" {
			continue
		}
		sh, ok := m.Sheets[t.Sheet]
		if !ok {
			return fmt.Errorf("роль %s: нет листа %s для wangset %s", role, t.Sheet, t.Wangset)
		}
		tsxPath := filepath.Join(m.dir, strings.TrimSuffix(sh.File, filepath.Ext(sh.File))+".tsx")
		sets, ok := cache[tsxPath]
		if !ok {
			var err error
			sets, err = parseWangCorners(tsxPath)
			if err != nil {
				return fmt.Errorf("роль %s: %w", role, err)
			}
			cache[tsxPath] = sets
		}
		tbl, ok := sets[t.Wangset]
		if !ok {
			return fmt.Errorf("роль %s: в %s нет wangset %q", role, filepath.Base(tsxPath), t.Wangset)
		}
		// Разметка .tsx — источник истины, но ключи, которых художник не
		// нарисовал, можно дописать в манифесте (поле corner). Без этого
		// генератор молча подставлял ближайший по Хеммингу тайл, и на стыке
		// вставал чужой кусок. Ключ из манифеста имеет приоритет.
		merged := make(map[string][]int, len(tbl)+len(t.Corner))
		for k, ids := range tbl {
			merged[k] = ids
		}
		for k, ids := range t.Corner {
			if len(ids) > 0 {
				merged[k] = ids
			}
		}
		t.Corner = merged
		m.Terrains[role] = t
	}
	return nil
}

// tsx-структуры для чтения угловых наборов.
type tsxRoot struct {
	Wangsets struct {
		Wangset []struct {
			Name     string `xml:"name,attr"`
			Type     string `xml:"type,attr"`
			Wangtile []struct {
				TileID int    `xml:"tileid,attr"`
				WangID string `xml:"wangid,attr"`
			} `xml:"wangtile"`
		} `xml:"wangset"`
	} `xml:"wangsets"`
}

// parseWangCorners читает все corner-наборы из .tsx в таблицы
// "NW,NE,SE,SW" → ВСЕ варианты локального id тайла. wangid — 8 значений
// [top,NE,right,SE,bottom,SW,left,NW]; для corner-схемы значимы углы
// индексов 7(NW),1(NE),3(SE),5(SW). Все тайлы комбинации копятся как варианты
// (генератор выбирает один псевдослучайно по позиции — так поверхность не плоская).
func parseWangCorners(path string) (map[string]map[string][]int, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("wangsets %s: %w", path, err)
	}
	var root tsxRoot
	if err := xml.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("wangsets %s: %w", path, err)
	}
	out := map[string]map[string][]int{}
	for _, ws := range root.Wangsets.Wangset {
		if ws.Type != "corner" {
			continue
		}
		tbl := map[string][]int{}
		for _, wt := range ws.Wangtile {
			v := strings.Split(wt.WangID, ",")
			if len(v) != 8 {
				continue
			}
			nz := func(i int) bool { return strings.TrimSpace(v[i]) != "0" }
			key := cornerKeyStr([4]bool{nz(7), nz(1), nz(3), nz(5)}) // NW NE SE SW
			tbl[key] = append(tbl[key], wt.TileID)
		}
		out[ws.Name] = tbl
	}
	return out, nil
}

// requiredRoles — обязательные роли манифеста (валидатор E11). Единый словарь
// поверхностей для всех биомов: поверхность нижней земли и её затенённый вариант
// под возвышенностью. Для edge_mode=wall жидкая роль называется solid.
func (m *Manifest) requiredRoles() []string {
	// Вода — сплошной цвет (water_color), тайла воды в наборах нет; обязательны
	// только наземные роли-поверхности.
	return []string{"ground", "ground_shadow"}
}

// validateRoles проверяет, что обязательные роли присутствуют (E11).
// Возвращает список пробелов.
func (m *Manifest) validateRoles() []string {
	var gaps []string
	for _, r := range m.requiredRoles() {
		if _, ok := m.Terrains[r]; !ok {
			gaps = append(gaps, "нет роли terrains."+r)
		}
	}
	return gaps
}
