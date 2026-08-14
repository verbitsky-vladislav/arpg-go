package character_test

import (
	"os"
	"testing"

	"github.com/vladislav/game/internal/assets"
	"github.com/vladislav/game/internal/character"
	"github.com/vladislav/game/internal/sprite"
)

const (
	assetsRoot   = "../../assets"
	characterDir = "character"
)

func catalog(t *testing.T) (*assets.Loader, *character.Catalog) {
	t.Helper()
	l := assets.NewLoader(os.DirFS(assetsRoot))
	cat, err := character.Load(l.FS(), characterDir+"/character.json")
	if err != nil {
		t.Fatal(err)
	}
	return l, cat
}

// packs — по паку на каждую пару «тело × лоадаут».
func packs(t *testing.T) (*character.Catalog, map[string]*sprite.Pack) {
	t.Helper()
	l, cat := catalog(t)
	ps, err := character.LoadPacks(l, characterDir, cat)
	if err != nil {
		t.Fatal(err)
	}
	return cat, ps
}

// TestCatalogValid — таблица персонажа внутренне связна.
func TestCatalogValid(t *testing.T) {
	_, cat := catalog(t)
	for _, p := range cat.Validate() {
		t.Error(p)
	}
}

// TestPacksLoad — у каждой пары есть спрайт-пак, он режется без ошибок, в нём
// все четыре направления и базовый набор клипов. Это проверка геометрии
// листов: несходящийся с манифестом PNG роняет sprite.Load.
func TestPacksLoad(t *testing.T) {
	cat, ps := packs(t)
	for _, bid := range cat.BodyIDs() {
		for _, lid := range cat.LoadoutIDs() {
			key := bid + "/" + lid
			p := ps[key]
			for d := range sprite.Dir(sprite.DirCount) {
				if p.Clip("idle", d) == nil {
					t.Errorf("%s: нет idle для направления %v", key, d)
				}
			}
			for _, n := range []string{"idle", "walk", "run", "hurt", "death"} {
				if !p.Has(n) {
					t.Errorf("%s: нет обязательного клипа %q", key, n)
				}
			}
		}
	}
}

// TestAnchors — у каждого пака посчитана точка опоры (tools/spriteanchor) и
// лежит она внутри кадра. Без неё персонаж будет стоять «в воздухе»: кадр
// 64×64, а ноги в нём на 47-м пикселе.
func TestAnchors(t *testing.T) {
	cat, ps := packs(t)
	for _, bid := range cat.BodyIDs() {
		for _, lid := range cat.LoadoutIDs() {
			key := bid + "/" + lid
			p := ps[key]
			if p.Anchor == nil || p.BBox == nil {
				t.Errorf("%s: манифест без anchor/bbox — прогнать tools/spriteanchor", key)
				continue
			}
			f := p.Foot()
			if f.X <= 0 || f.X >= p.Frame.W || f.Y <= 0 || f.Y > p.Frame.H {
				t.Errorf("%s: точка опоры %v вне кадра %dx%d", key, f, p.Frame.W, p.Frame.H)
			}
		}
	}
}

// TestHitAtMatchesClips — кадры попадания из character.json существуют в
// реальных клипах. Это единственная сверка данных с графикой: занизить hit_at
// безопасно, а завысить — значит бить кадром, которого в клипе нет.
func TestHitAtMatchesClips(t *testing.T) {
	cat, ps := packs(t)
	for _, lid := range cat.LoadoutIDs() {
		l := cat.Loadout(lid)
		for clipName, frame := range l.Attack.HitAt {
			for _, bid := range cat.BodyIDs() {
				key := bid + "/" + lid
				c := ps[key].Clip(clipName, sprite.Down)
				if c == nil {
					t.Errorf("%s: hit_at ссылается на клип %q, которого в паке нет", key, clipName)
					continue
				}
				if frame >= len(c.Frames) {
					t.Errorf("%s: hit_at[%q]=%d, а кадров в клипе %d",
						key, clipName, frame, len(c.Frames))
				}
			}
		}
	}
}

// TestOnMoveHasClips — лоадаут, объявивший удар на ходу, действительно умеет
// его показать. Иначе персонаж бил бы на бегу подменышем, чего данные не ждут.
func TestOnMoveHasClips(t *testing.T) {
	cat, ps := packs(t)
	for _, lid := range cat.LoadoutIDs() {
		if !cat.Loadout(lid).Attack.OnMove {
			continue
		}
		for _, bid := range cat.BodyIDs() {
			p := ps[bid+"/"+lid]
			if !p.Has("walk_attack") && !p.Has("run_attack") {
				t.Errorf("%s/%s: on_move=true, но в паке нет ни walk_attack, ни run_attack", bid, lid)
			}
		}
	}
}

// TestEightDirections — у каждой пары все восемь направлений нарисованы
// по-настоящему, а не подменены ближайшей стороной света.
//
// Проверка на подмену идёт по кадрам: у отката диагональ и боковое
// направление указывали бы на один и тот же кусок атласа. Без неё тест
// прошёл бы и на четырёхрядном паке — Clip там возвращает не nil, а
// запасной клип, и разницы по указателю не видно.
func TestEightDirections(t *testing.T) {
	cat, ps := packs(t)
	pairs := [][2]sprite.Dir{
		{sprite.DownRight, sprite.Right},
		{sprite.DownLeft, sprite.Left},
		{sprite.UpRight, sprite.Right},
		{sprite.UpLeft, sprite.Left},
	}
	for _, bid := range cat.BodyIDs() {
		for _, lid := range cat.LoadoutIDs() {
			key := bid + "/" + lid
			p := ps[key]
			for _, n := range p.Anims() {
				for _, pair := range pairs {
					diag, side := p.Clip(n, pair[0]), p.Clip(n, pair[1])
					if diag == nil {
						t.Errorf("%s/%s: нет клипа для %v", key, n, pair[0])
						continue
					}
					if len(diag.Frames) == 0 || len(side.Frames) == 0 {
						t.Errorf("%s/%s: пустой клип", key, n)
						continue
					}
					if diag.Frames[0] == side.Frames[0] {
						t.Errorf("%s/%s: %v подменено на %v — диагональ не нарисована",
							key, n, pair[0], pair[1])
					}
				}
			}
		}
	}
}
