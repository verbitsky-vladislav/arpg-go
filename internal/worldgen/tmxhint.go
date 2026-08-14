package worldgen

// tmxhint.go — ПОДСКАЗКА для авторинга манифеста. Читает пример карты .tmx
// (сделанную художником) и вытаскивает, какой локальный id тайла в атласе
// соответствует какой blob-маске для каждого слоя. Это не источник результата —
// результат генерируется с нуля из шума; .tmx лишь избавляет от ручного подбора
// индексов. Ключи blob-маски вычисляются той же normalizeBlob, что и в autotile.

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type tmxMap struct {
	XMLName  xml.Name     `xml:"map"`
	Width    int          `xml:"width,attr"`
	Height   int          `xml:"height,attr"`
	Tilesets []tmxTileset `xml:"tileset"`
	Layers   []tmxLayer   `xml:"layer"`
}

type tmxTileset struct {
	Firstgid  int    `xml:"firstgid,attr"`
	Name      string `xml:"name,attr"`
	Tilecount int    `xml:"tilecount,attr"`
	Columns   int    `xml:"columns,attr"`
	Source    string `xml:"source,attr"`
	Image     struct {
		Source string `xml:"source,attr"`
		Width  int    `xml:"width,attr"`
		Height int    `xml:"height,attr"`
	} `xml:"image"`
}

type tmxLayer struct {
	Name string  `xml:"name,attr"`
	Data tmxData `xml:"data"`
}

type tmxData struct {
	Encoding string     `xml:"encoding,attr"`
	Text     string     `xml:",chardata"`
	Chunks   []tmxChunk `xml:"chunk"`
}

type tmxChunk struct {
	X      int    `xml:"x,attr"`
	Y      int    `xml:"y,attr"`
	Width  int    `xml:"width,attr"`
	Height int    `xml:"height,attr"`
	Text   string `xml:",chardata"`
}

const gidMask = 0x1FFFFFFF // снять флаги отражения Tiled

// tmxCell — распакованная клетка слоя.
type tmxCell struct{ X, Y, Gid int }

// parseTMX читает и разбирает .tmx.
func parseTMX(path string) (*tmxMap, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m tmxMap
	if err := xml.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("разбор %s: %w", path, err)
	}
	return &m, nil
}

// tilesetName — имя листа (для source-based берём из имени файла .tsx).
func (t tmxTileset) tilesetName() string {
	if t.Name != "" {
		return t.Name
	}
	base := filepath.Base(t.Source)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// sheetForGid — имя листа и локальный id для gid по диапазонам firstgid.
func sheetForGid(sets []tmxTileset, gid int) (string, int) {
	best := -1
	for i, ts := range sets {
		if ts.Firstgid <= gid && (best == -1 || ts.Firstgid > sets[best].Firstgid) {
			best = i
		}
	}
	if best == -1 {
		return "", 0
	}
	return sets[best].tilesetName(), gid - sets[best].Firstgid
}

// cells распаковывает слой (и infinite-чанки, и обычный CSV) в список клеток.
func (l tmxLayer) cells() []tmxCell {
	var out []tmxCell
	push := func(baseX, baseY, w int, csv string) {
		nums := splitCSV(csv)
		for i, v := range nums {
			if v == 0 {
				continue
			}
			out = append(out, tmxCell{X: baseX + i%w, Y: baseY + i/w, Gid: v & gidMask})
		}
	}
	if len(l.Data.Chunks) > 0 {
		for _, ch := range l.Data.Chunks {
			push(ch.X, ch.Y, ch.Width, ch.Text)
		}
	} else {
		// обычный CSV: ширина неизвестна из data — берём из map, но здесь
		// используем только чанковый путь (Forest.tmx infinite). На всякий:
		push(0, 0, 1, l.Data.Text)
	}
	return out
}

// splitCSV разбирает CSV-текст слоя Tiled в срез int.
func splitCSV(s string) []int {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == ' ' || r == '\t'
	})
	out := make([]int, 0, len(fields))
	for _, f := range fields {
		if f == "" {
			continue
		}
		if v, ok := atoi(f); ok {
			out = append(out, v)
		}
	}
	return out
}

// RunTMXHint печатает разбор слоёв и черновики blob-наборов, либо (в режиме
// emit) целый черновик manifest.json.
func RunTMXHint(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "укажите каталог биома с Tiled_files/*.tmx")
		os.Exit(2)
	}
	biomeDir := args[0]
	emit := len(args) >= 2 && args[1] == "emit"
	tmxPath := findTMX(biomeDir)
	if emit {
		emitDraftManifest(biomeDir, tmxPath)
		return
	}
	if tmxPath == "" {
		fatal(fmt.Errorf("не найден .tmx в %s/Tiled_files", biomeDir))
	}
	m, err := parseTMX(tmxPath)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("# %s (%dx%d), листов: %d, слоёв: %d\n", tmxPath, m.Width, m.Height, len(m.Tilesets), len(m.Layers))
	fmt.Println("\n## Листы (firstgid):")
	for _, ts := range m.Tilesets {
		fmt.Printf("  %-24s firstgid=%-6d cols=%-3d count=%-5d img=%s\n",
			ts.tilesetName(), ts.Firstgid, ts.Columns, ts.Tilecount, ts.Image.Source)
	}

	fmt.Println("\n## Слои (доминирующий лист, топ локальных id):")
	for _, l := range m.Layers {
		reportLayer(m, l)
	}

	// blob-наборы ключевых слоёв в JSON — для вставки в manifest.json
	fmt.Println("\n## blob-наборы (маска→id) для вставки в манифест:")
	for _, want := range []string{"water", "main_space", "elevated_space"} {
		for _, l := range m.Layers {
			if l.Name != want {
				continue
			}
			sheet, blob, fill := deriveBlob(m, l)
			js, _ := jsonMarshalIndent(blob)
			fmt.Printf("\n// слой %s → лист %s, fill=%d\n%s\n", l.Name, sheet, fill, js)
		}
	}
}

// buildSet — множество клеток, покрытых любым из перечисленных слоёв.
func (m *tmxMap) buildSet(layers ...string) map[[2]int]bool {
	set := map[[2]int]bool{}
	for _, ln := range layers {
		if l, ok := m.layerByName(ln); ok {
			for _, c := range l.cells() {
				set[[2]int{c.X, c.Y}] = true
			}
		}
	}
	return set
}

// maskAgainst — 8-соседняя маска: бит i выставлен, если сосед i лежит в ref.
// Порядок бит совпадает с nb8 и с generation-путём (autotile.mask8).
func maskAgainst(ref map[[2]int]bool, x, y int) uint8 {
	var mask uint8
	for i, d := range nb8 {
		if ref[[2]int{x + d[0], y + d[1]}] {
			mask |= 1 << uint(i)
		}
	}
	return mask
}

// deriveBlobRef снимает набор «нормализованная маска → id» со слоя layerName,
// но маска считается ОТНОСИТЕЛЬНО опорного множества ref (напр. вся суша), а не
// собственного заполнения слоя. Это делает семантику маски такой же, как в
// генерации (present=isLand / Plateau), и наборы разных слоёв становятся
// совместимыми. Возвращает лист, набор и число клеток внутри/снаружи ref.
func deriveBlobRef(m *tmxMap, layerName string, ref map[[2]int]bool) (sheet string, blob map[string]int, fill, inside, outside int) {
	l, ok := m.layerByName(layerName)
	if !ok {
		return "", map[string]int{}, 0, 0, 0
	}
	cs := l.cells()
	sheetCount := map[string]int{}
	for _, c := range cs {
		name, _ := sheetForGid(m.Tilesets, c.Gid)
		sheetCount[name]++
	}
	sheet = topKey(sheetCount)

	votes := map[string]map[int]int{}
	idHist := map[int]int{}
	for _, c := range cs {
		name, local := sheetForGid(m.Tilesets, c.Gid)
		if name != sheet {
			continue
		}
		if ref[[2]int{c.X, c.Y}] {
			inside++
		} else {
			outside++
		}
		idHist[local]++
		key := blobKey(maskAgainst(ref, c.X, c.Y))
		if votes[key] == nil {
			votes[key] = map[int]int{}
		}
		votes[key][local]++
	}
	blob = map[string]int{}
	for key, hist := range votes {
		blob[key] = topKey(hist)
	}
	return sheet, blob, topKey(idHist), inside, outside
}

// layerByName ищет слой по имени.
func (m *tmxMap) layerByName(name string) (tmxLayer, bool) {
	for _, l := range m.Layers {
		if l.Name == name {
			return l, true
		}
	}
	return tmxLayer{}, false
}

// emitDraftManifest строит и печатает черновик manifest.json из примера .tmx.
// Плотные роли (liquid/ground_a/plateau) выводятся с полными blob-наборами;
// cliff/transitions/stairs/props — заготовки под ручную доводку (M4/M5).
func emitDraftManifest(biomeDir, tmxPath string) {
	if tmxPath == "" {
		fatal(fmt.Errorf("не найден .tmx в %s/Tiled_files", biomeDir))
	}
	m, err := parseTMX(tmxPath)
	if err != nil {
		fatal(err)
	}

	// геометрия листов (для source-based Water_detilazation2 берём известные).
	prefix := tilesPrefix(biomeDir)
	sheets := map[string]Sheet{}
	for _, ts := range m.Tilesets {
		name := ts.tilesetName()
		cols, count := ts.Columns, ts.Tilecount
		img := filepath.Base(ts.Image.Source) // путь в .tmx может быть относительным
		if cols == 0 {                        // source-based (.tsx) — дублирует основную воду
			cols, count, img = 37, 2886, "water_detilazation.png"
		}
		sheets[name] = Sheet{File: prefix + img, Columns: cols, Count: count}
	}

	// опорные множества форм: вся суша и отдельно плато. Маски всех наборов
	// считаются относительно них — так семантика совпадает с генерацией.
	landSet := m.buildSet("main_space", "main_space2", "elevated_space")
	platSet := m.buildSet("elevated_space")

	// liquid — вода целиком под всем (фон), просто ровный тайл.
	_, liquidBlob, liquidFill, _, _ := deriveBlobRef(m, "water", m.buildSet("water"))
	liquid := Terrain{Sheet: "Water_detilazation", Fill: liquidFill, Blob: liquidBlob}

	// ground_a — ровная трава (в исходнике main_space плоский), без blob-краёв.
	_, _, groundFill, _, _ := deriveBlobRef(m, "main_space", landSet)
	groundA := Terrain{Sheet: "Water_coasts", Fill: groundFill}

	// plateau — верхний уровень; blob относительно формы плато (даёт грани).
	platSheet, platBlob, platFill, _, _ := deriveBlobRef(m, "elevated_space", platSet)
	plateau := Terrain{Sheet: platSheet, Fill: platFill, Blob: platBlob}

	// coast — отдельный слой из авторского `ground` (береговая линия Water_coasts),
	// маска относительно суши. Ставится на клетки воды у берега.
	coastSheet, coastSet, _, cin, cout := deriveBlobRef(m, "ground", landSet)
	fmt.Fprintf(os.Stderr, "coast: лист=%s клеток внутри суши=%d снаружи=%d\n", coastSheet, cin, cout)

	// shadow — тень плато (Ground_grass), маска относительно формы плато.
	shSheet, shSet, shFill, _, _ := deriveBlobRef(m, "shadow", platSet)

	// cliff — грани обрыва берём из blob плато (нижние тайлы elevated_space).
	cliff := Terrain{Sheet: platSheet, Fill: platFill, Edges: map[string]int{}}

	out := Manifest{
		ID:          filepath.Base(biomeDir),
		TileSize:    16,
		RenderScale: 2,
		EdgeMode:    "water",
		Sheets:      sheets,
		Terrains: map[string]Terrain{
			"liquid":   liquid,
			"ground_a": groundA,
			"ground_b": groundA, // доводится позже (второй тип покрытия)
			"plateau":  plateau,
			"cliff":    cliff,
			"shadow":   {Sheet: shSheet, Fill: shFill, Blob: shSet},
		},
		Transitions: map[string]Transition{
			"liquid->ground_a": {Sheet: coastSheet, Set: coastSet},
		},
		Stairs: Stairs{Sheet: "stairs_grass", Width: 2},
		Hangers: Hangers{
			Sheet:    "lianas",
			Variants: []Stamp{{W: 1, H: 1, Cells: []StampCell{{Tile: 113}}}},
			On:       "cliff_south",
		},
		Props:   scanProps(biomeDir),
		Surface: map[string][]string{"liquid": {"water_lilis"}, "shallow": {"reeds"}},
		Spots:   spotSetFrom(m, "spots1", "spots2", "spots3"),
	}
	js, _ := jsonMarshalIndent(out)
	fmt.Println(js)
}

// spotSetFrom собирает частые тайлы напольных пятен из авторских слоёв spots*.
func spotSetFrom(m *tmxMap, layers ...string) SpotSet {
	hist := map[int]int{}
	sheet := ""
	for _, ln := range layers {
		l, ok := m.layerByName(ln)
		if !ok {
			continue
		}
		for _, c := range l.cells() {
			name, local := sheetForGid(m.Tilesets, c.Gid)
			if sheet == "" {
				sheet = name
			}
			if name == sheet {
				hist[local]++
			}
		}
	}
	// топ-8 самых частых пятен
	type kv struct{ id, n int }
	arr := make([]kv, 0, len(hist))
	for id, n := range hist {
		arr = append(arr, kv{id, n})
	}
	sort.Slice(arr, func(i, j int) bool {
		if arr[i].n != arr[j].n {
			return arr[i].n > arr[j].n
		}
		return arr[i].id < arr[j].id
	})
	var vars []int
	for i := 0; i < len(arr) && i < 8; i++ {
		vars = append(vars, arr[i].id)
	}
	return SpotSet{Sheet: sheet, Variants: vars}
}

// deriveBlob возвращает доминирующий лист, набор blob-маска→id (голосованием) и
// самый частый (fill) id для слоя.
func deriveBlob(m *tmxMap, l tmxLayer) (sheet string, blob map[string]int, fill int) {
	cs := l.cells()
	if len(cs) == 0 {
		return "", map[string]int{}, 0
	}
	sheetCount := map[string]int{}
	for _, c := range cs {
		name, _ := sheetForGid(m.Tilesets, c.Gid)
		sheetCount[name]++
	}
	sheet = topKey(sheetCount)

	minX, minY, maxX, maxY := bounds(cs)
	w, h := maxX-minX+1, maxY-minY+1
	grid := make([]int, w*h)
	for _, c := range cs {
		grid[(c.Y-minY)*w+(c.X-minX)] = c.Gid
	}
	votes := map[string]map[int]int{}
	idHist := map[int]int{}
	for _, c := range cs {
		name, local := sheetForGid(m.Tilesets, c.Gid)
		if name != sheet {
			continue
		}
		idHist[local]++
		var mask uint8
		for i, d := range nb8 {
			nx, ny := c.X-minX+d[0], c.Y-minY+d[1]
			if nx < 0 || ny < 0 || nx >= w || ny >= h || grid[ny*w+nx] == 0 {
				continue
			}
			mask |= 1 << uint(i)
		}
		key := blobKey(mask)
		if votes[key] == nil {
			votes[key] = map[int]int{}
		}
		votes[key][local]++
	}
	blob = map[string]int{}
	for key, hist := range votes {
		blob[key] = topKey(hist)
	}
	return sheet, blob, topKey(idHist)
}

// reportLayer печатает по слою: разбивку по листам, самый частый (fill) id и
// черновик blob-набора для доминирующего листа.
func reportLayer(m *tmxMap, l tmxLayer) {
	cs := l.cells()
	if len(cs) == 0 {
		fmt.Printf("  %-22s (пусто)\n", l.Name)
		return
	}
	// разбивка по листам и гистограмма локальных id доминирующего листа
	sheetCount := map[string]int{}
	for _, c := range cs {
		name, _ := sheetForGid(m.Tilesets, c.Gid)
		sheetCount[name]++
	}
	domSheet := topKey(sheetCount)

	// сетка gid по границам слоя для расчёта blob-масок
	minX, minY, maxX, maxY := bounds(cs)
	w, h := maxX-minX+1, maxY-minY+1
	grid := make([]int, w*h)
	for _, c := range cs {
		grid[(c.Y-minY)*w+(c.X-minX)] = c.Gid
	}

	idHist := map[int]int{}
	blob := map[string]map[int]int{} // ключ маски → (localID → голоса)
	for _, c := range cs {
		name, local := sheetForGid(m.Tilesets, c.Gid)
		if name != domSheet {
			continue
		}
		idHist[local]++
		// blob-маска: сосед «заполнен», если в этом слое там непусто
		var mask uint8
		for i, d := range nb8 {
			nx, ny := c.X-minX+d[0], c.Y-minY+d[1]
			if nx < 0 || ny < 0 || nx >= w || ny >= h || grid[ny*w+nx] == 0 {
				continue
			}
			mask |= 1 << uint(i)
		}
		key := blobKey(mask)
		if blob[key] == nil {
			blob[key] = map[int]int{}
		}
		blob[key][local]++
	}

	fmt.Printf("  %-22s лист=%-20s клеток=%-5d fill=%d масок=%d\n",
		l.Name, domSheet, len(cs), topKey(idHist), len(blob))
}

// findTMX ищет первый .tmx: сначала в authoring/ (новая структура), затем в
// Tiled_files/ (старая) и в корне биома.
func findTMX(biomeDir string) string {
	for _, sub := range []string{"authoring", "Tiled_files", "."} {
		dir := filepath.Join(biomeDir, sub)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if strings.EqualFold(filepath.Ext(e.Name()), ".tmx") {
				return filepath.Join(dir, e.Name())
			}
		}
	}
	return ""
}

// tilesPrefix — где лежат листы-атласы: tiles/ (новая структура) или
// Tiled_files/ (старая). Используется в путях sheet.File манифеста.
func tilesPrefix(biomeDir string) string {
	if fi, err := os.Stat(filepath.Join(biomeDir, "tiles")); err == nil && fi.IsDir() {
		return "tiles/"
	}
	return "Tiled_files/"
}

func bounds(cs []tmxCell) (minX, minY, maxX, maxY int) {
	minX, minY = 1<<30, 1<<30
	maxX, maxY = -(1 << 30), -(1 << 30)
	for _, c := range cs {
		if c.X < minX {
			minX = c.X
		}
		if c.Y < minY {
			minY = c.Y
		}
		if c.X > maxX {
			maxX = c.X
		}
		if c.Y > maxY {
			maxY = c.Y
		}
	}
	return
}

// topKey — ключ с наибольшим значением (детерминированно: при равенстве меньший id/имя).
func topKey[K int | string](m map[K]int) K {
	type kv struct {
		k K
		v int
	}
	arr := make([]kv, 0, len(m))
	for k, v := range m {
		arr = append(arr, kv{k, v})
	}
	sort.Slice(arr, func(i, j int) bool {
		if arr[i].v != arr[j].v {
			return arr[i].v > arr[j].v
		}
		return fmt.Sprint(arr[i].k) < fmt.Sprint(arr[j].k)
	})
	var zero K
	if len(arr) == 0 {
		return zero
	}
	return arr[0].k
}
