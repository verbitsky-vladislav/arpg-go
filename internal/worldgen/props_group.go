package worldgen

// props_group.go — группы пропсов и правила их размещения.
//
// Группировать по картинке нельзя: у дерева спрайт 128 px, а ствол у земли 22-28,
// у глыбы спрайт 64 при опоре 15 и теле 49. Поэтому группа задаёт СМЫСЛ (за что
// игрок цепляется, где объекту место), а размеры берутся замером из PNG.
//
// Правила, по которым группы и разведены:
//   - на воде пропсов не бывает, но проверяется ТЕЛО, а не спрайт: дерево имеет
//     право стоять у самой воды, свесив крону над ней, как и рисовал художник;
//   - тропа — дорога, и чистой должна быть не только её клетка, но и вид: крона
//     шириной 8 тайлов накрывает дорожку, даже когда ствол стоит рядом, поэтому
//     крупное отходит от тропы на половину своего спрайта;
//   - у кромки плато нельзя ничего: там обрыв, физика и без пропсов тесная;
//   - крупное непроходимо, мелочь (камушки, грибы, пучки травы) — сквозная.

// PropGroup — правила одной группы.
type PropGroup struct {
	Name     string
	Collides bool     // непроходим
	OnTrail  bool     // допустим на тропе
	On       []string // роли поверхности
	Weight   int      // вес внутри своей группы
	// Cover — доля пригодных клеток по ЗОНАМ влажности. Зона, которой нет в
	// карте, для группы закрыта. Так лес густеет во влажных местах и редеет в
	// сухих, а не обрывается по границе зоны: раньше плотность была одна на
	// группу, и в сухой половине острова деревьев не было вовсе.
	Cover map[string]float64
	// EdgeClear — сколько клеток отступать от кромки плато и от тела обрыва.
	EdgeClear int
}

// propGroups — порядок важен: тяжёлые группы сеются первыми и занимают место,
// мелочь потом втискивается в остатки.
var propGroups = []PropGroup{{
	Name: "ruin", Collides: true, OnTrail: false, On: []string{"ground_a"},
	Weight: 4, EdgeClear: 2,
	Cover: map[string]float64{"open": 0.006, "mid": 0.006},
}, {
	Name: "trunk", Collides: true, OnTrail: false, On: []string{"ground_a", "plateau"},
	Weight: 30, EdgeClear: 1,
	Cover: map[string]float64{"dense": 0.90, "mid": 0.60, "open": 0.22, "plateau": 0.30},
}, {
	Name: "boulder", Collides: true, OnTrail: false, On: []string{"ground_a", "plateau"},
	Weight: 14, EdgeClear: 1,
	Cover: map[string]float64{"open": 0.10, "mid": 0.05, "plateau": 0.10},
}, {
	Name: "rubble", Collides: true, OnTrail: false, On: []string{"ground_a", "plateau"},
	Weight: 18, EdgeClear: 1,
	Cover: map[string]float64{"open": 0.10, "mid": 0.08, "dense": 0.06, "plateau": 0.10},
}, {
	Name: "bush", Collides: false, OnTrail: false, On: []string{"ground_a", "plateau"},
	Weight: 24, EdgeClear: 1,
	Cover: map[string]float64{"dense": 0.35, "mid": 0.35, "open": 0.30, "plateau": 0.30},
}, {
	Name: "litter", Collides: false, OnTrail: true, On: []string{"ground_a", "plateau"},
	Weight: 20, EdgeClear: 0,
	Cover: map[string]float64{"dense": 0.03, "mid": 0.03, "open": 0.03, "plateau": 0.03},
}}

// propGroupByName — правила группы по имени.
func propGroupByName(name string) (PropGroup, bool) {
	for _, g := range propGroups {
		if g.Name == name {
			return g, true
		}
	}
	return PropGroup{}, false
}

// zones — зоны группы в устойчивом порядке: map перебирается случайно, а карта
// обязана зависеть только от сида.
func (g PropGroup) zones() []string {
	out := make([]string, 0, len(g.Cover))
	for _, z := range []string{"dense", "mid", "open", "plateau"} {
		if _, ok := g.Cover[z]; ok {
			out = append(out, z)
		}
	}
	return out
}

// propDensity — общий множитель плотности объектов поверх долей Cover. Отдельная
// ручка, чтобы разредить или сгустить карту целиком, не трогая соотношение групп
// между собой.
const propDensity = 0.75

// Порог «мелочи»: объект с телом у земли уже этого игрок не ощущает, и делать
// его препятствием — только злить. Замер по каталогу: галька 9-13 px, пучки
// травы 13, грибы 13-24, кучка камней 18, куст 25-35, ствол 24-61, руина 32-88.
const propLitterBody = 20

// propStairClear — свободная полоса вокруг лестницы в клетках. Лестница —
// единственная связь этажей, и запереть её пропсом нельзя.
const propStairClear = 2

// propPassGap — зазор вокруг непроходимого объекта в пикселях, чтобы между
// двумя соседними осталась щель шире игрока (радиус тела героя 7-8 px).
const propPassGap = 14

// Ремонт стены из объектов: сколько проходов и на сколько клеток от отрезанного
// куска вырубать. Две клетки — ширина просеки, которой хватает телу героя.
const (
	propRepairPasses = 3
	propRepairReach  = 2
)

// propDustBody — совсем мелочь: галька и пучки травы. Она одна ставится без
// зазора, всё остальное держит соседей на клетке.
const propDustBody = 14
