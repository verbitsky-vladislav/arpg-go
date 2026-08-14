package worldgen

// mapformat.go — структуры выходного формата `map_format v1`.
// Спека worldgen.spec §3, §9.1 ссылается на несуществующий map-spec.md, поэтому
// формат зафиксирован здесь конкретно. См. docs/worldgen/map_format.md.

// MapV1 — карта целиком.
type MapV1 struct {
	Format   string     `json:"format"` // "map_format v1"
	Biome    string     `json:"biome"`
	Seed     int64      `json:"seed"`
	Width    int        `json:"width"`
	Height   int        `json:"height"`
	TileSize int        `json:"tile_size"`
	Sheets   []SheetRef `json:"sheets"` // упорядочены, index используется ссылками тайлов
	// WaterColors — палитра воды от мели к глубине; индекс = значение в
	// layers.liquid_shade минус 1. Сплошного водного тайла в наборах нет, воду
	// красит движок (autotiling.md §5).
	WaterColors [][]int    `json:"water_colors"`
	Layers      Layers     `json:"layers"`
	Props       []PropInst `json:"props"`
	Markers     []Marker   `json:"markers"`
	Nav         NavData    `json:"nav"`
}

// SheetRef — лист тайлсета в порядке firstgid (для совместимости с Tiled-нумерацией).
type SheetRef struct {
	Name     string `json:"name"`
	File     string `json:"file"`
	Columns  int    `json:"columns"`
	Count    int    `json:"tilecount"`
	Firstgid int    `json:"firstgid"`
}

// Layers — 14 канонических слоёв (§3). Плотные — []uint16 row-major (0 = пусто),
// каждый из одного листа. Разрежённые — списки клеток со своим листом.
//
// Порядок полей — порядок хранения; порядок отрисовки совпадает с ним всюду,
// кроме ground_decor: кустики травы рисуются ПОСЛЕ plateau, иначе макушка
// закрывала бы собой свои же кустики (docs/worldgen/map_format.md).
type Layers struct {
	Liquid        []uint16     `json:"liquid"`         // dense
	LiquidShade   []uint8      `json:"liquid_shade"`   // dense, индекс цвета в water_colors (1 = мель)
	LiquidDetail  []SparseTile `json:"liquid_detail"`  // sparse, anim
	Ground        []uint16     `json:"ground"`         // dense, grass-блок
	Mud           []uint16     `json:"mud"`            // dense, mud-блок (тропы)
	GroundSpots   []SparseTile `json:"ground_spots"`   // sparse
	GroundDecor   []SparseTile `json:"ground_decor"`   // sparse, кустики травы
	Coast         []SparseTile `json:"coast"`          // sparse, anim
	Bridges       []SparseTile `json:"bridges"`        // sparse, броды по камням через воду
	SurfaceLiquid []SparseTile `json:"surface_liquid"` // sparse
	PlateauShadow []SparseTile `json:"plateau_shadow"` // sparse
	Plateau       []uint16     `json:"plateau"`        // dense
	Cliff         []SparseTile `json:"cliff"`          // sparse
	Stairs        []SparseTile `json:"stairs"`         // sparse
	Hangers       []SparseTile `json:"hangers"`        // sparse
}

// SparseTile — одна клетка разрежённого слоя. Sheet — индекс в MapV1.Sheets.
// Rot — поворот тайла на 90° по часовой стрелке, 0..3 четверти. Поворот кратен
// прямому углу и потому обратим без ресемплинга: пиксельная сетка сохраняется.
// Нужен там, где у художника есть набор только одной ориентации (брод собран
// сверху вниз, а реку случается пересекать поперёк).
type SparseTile struct {
	X     int      `json:"x"`
	Y     int      `json:"y"`
	Sheet uint8    `json:"s"`
	Tile  uint16   `json:"t"`
	Rot   uint8    `json:"rot,omitempty"`
	Anim  *AnimRef `json:"anim,omitempty"`
}

// AnimRef — параметры анимации клетки (кадры лежат в атласе с шагом stride).
type AnimRef struct {
	Frames int `json:"frames"`
	Stride int `json:"stride"`
	MS     int `json:"ms"`
}

// PropInst — размещённый объект. Anchor — точка на полу (низ-центр), ключ сортировки.
type PropInst struct {
	ID       string `json:"id"`
	File     string `json:"file"`
	X        int    `json:"x"` // тайл верхнего-левого угла футпринта
	Y        int    `json:"y"`
	W        int    `json:"w"`
	H        int    `json:"h"`
	Anchor   [2]int `json:"anchor"`
	SortY    int    `json:"sort_y"`
	Collides bool   `json:"collides"`
	Body     int    `json:"body,omitempty"` // ширина тела у земли в пикселях
	// Frames/MS — покачивание на ветру: File указывает на вертикальную полосу
	// кадров высотой H тайлов каждый. Ноль кадров — обычный одиночный спрайт.
	Frames int `json:"frames,omitempty"`
	MS     int `json:"ms,omitempty"`
}

// Marker — точка интереса (spawn, exit, camera_bound).
type Marker struct {
	Kind string `json:"kind"`
	X    int    `json:"x"`
	Y    int    `json:"y"`
}

// NavData — сетка физики: что находится в каждой ПОД-клетке карты. Значения —
// physics.Cell (вода/земля/плато/лестница/стена), из них движок выводит и
// проходимость, и этаж, и замедление. Разрешение мельче тайла (Scale под-клеток
// на тайл), и сдвиг dual-grid уже учтён — см. buildNav и map_format.md.
type NavData struct {
	Width  int     `json:"w"`     // в под-клетках
	Height int     `json:"h"`     // в под-клетках
	Scale  int     `json:"scale"` // под-клеток на тайл
	Cells  []uint8 `json:"cells"`
}
