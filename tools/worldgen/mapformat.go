package main

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
	Layers   Layers     `json:"layers"`
	Props    []PropInst `json:"props"`
	Markers  []Marker   `json:"markers"`
	Nav      NavData    `json:"nav"`
}

// SheetRef — лист тайлсета в порядке firstgid (для совместимости с Tiled-нумерацией).
type SheetRef struct {
	Name     string `json:"name"`
	File     string `json:"file"`
	Columns  int    `json:"columns"`
	Count    int    `json:"tilecount"`
	Firstgid int    `json:"firstgid"`
}

// Layers — 12 канонических слоёв (§3). Плотные — []uint16 row-major (0 = пусто),
// каждый из одного листа. Разрежённые — списки клеток со своим листом.
type Layers struct {
	Liquid        []uint16     `json:"liquid"`         // dense
	LiquidDetail  []SparseTile `json:"liquid_detail"`  // sparse, anim
	GroundUnder   []uint16     `json:"ground_under"`   // dense, grass_overlay под блоком
	Ground        []uint16     `json:"ground"`         // dense, grass-блок
	MudUnder      []uint16     `json:"mud_under"`      // dense, mud_overlay под тропой
	Mud           []uint16     `json:"mud"`            // dense, mud-блок (тропы)
	GroundSpots   []SparseTile `json:"ground_spots"`   // sparse
	Coast         []SparseTile `json:"coast"`          // sparse, anim
	SurfaceLiquid []SparseTile `json:"surface_liquid"` // sparse
	PlateauShadow []SparseTile `json:"plateau_shadow"` // sparse
	Plateau       []uint16     `json:"plateau"`        // dense
	Cliff         []SparseTile `json:"cliff"`          // sparse
	Stairs        []SparseTile `json:"stairs"`         // sparse
	Hangers       []SparseTile `json:"hangers"`        // sparse
}

// SparseTile — одна клетка разрежённого слоя. Sheet — индекс в MapV1.Sheets.
type SparseTile struct {
	X     int      `json:"x"`
	Y     int      `json:"y"`
	Sheet uint8    `json:"s"`
	Tile  uint16   `json:"t"`
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
}

// Marker — точка интереса (spawn, exit, camera_bound).
type Marker struct {
	Kind string `json:"kind"`
	X    int    `json:"x"`
	Y    int    `json:"y"`
}

// NavData — плотная сетка стоимости прохода (0 = непроходимо).
type NavData struct {
	Width  int     `json:"w"`
	Height int     `json:"h"`
	Cost   []uint8 `json:"cost"`
}
