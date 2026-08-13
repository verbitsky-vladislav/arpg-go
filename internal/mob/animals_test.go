package mob_test

import (
	"os"
	"testing"

	"github.com/vladislav/game/internal/assets"
	"github.com/vladislav/game/internal/mob"
	"github.com/vladislav/game/internal/sprite"
)

const (
	assetsRoot = "../../assets"
	animalsDir = "mobs/animals"
)

func catalog(t *testing.T) (*assets.Loader, *mob.Catalog) {
	t.Helper()
	l := assets.NewLoader(os.DirFS(assetsRoot))
	cat, err := mob.LoadSpecies(l.FS(), animalsDir+"/species.json")
	if err != nil {
		t.Fatal(err)
	}
	return l, cat
}

// TestSpeciesValid — таблица видов внутренне связна: ссылки ведут на известные
// виды, обязательные числа заполнены.
func TestSpeciesValid(t *testing.T) {
	_, cat := catalog(t)
	for _, p := range cat.Validate() {
		t.Error(p)
	}
}

// TestPacksLoad — у каждого вида есть спрайт-пак, он режется без ошибок, и в
// нём есть все четыре направления. Это проверка геометрии листов: несходящийся
// с манифестом PNG роняет sprite.Load.
func TestPacksLoad(t *testing.T) {
	l, cat := catalog(t)
	for _, id := range cat.IDs() {
		sp := cat.Get(id)
		p, err := sprite.Load(l, animalsDir+"/"+sp.Art)
		if err != nil {
			t.Errorf("%s: %v", id, err)
			continue
		}
		if len(p.Anims()) == 0 {
			t.Errorf("%s: в паке нет анимаций", id)
		}
		for _, name := range p.Anims() {
			for d := range sprite.Dir(sprite.DirCount) {
				if p.Clip(name, d) == nil {
					t.Errorf("%s/%s: нет направления %s", id, name, d)
				}
			}
		}
	}
}

// TestSpeciesMatchesArt — сверка намерений вида с тем, что реально нарезано.
// Расхождения тут ожидаемы (клипов attack почти нет), поэтому это не провал
// теста, а список, который движок обязан закрыть фолбэками, — см. docs/mobs/species.md §4.
func TestSpeciesMatchesArt(t *testing.T) {
	l, cat := catalog(t)
	for _, id := range cat.IDs() {
		sp := cat.Get(id)
		p, err := sprite.Load(l, animalsDir+"/"+sp.Art)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if sp.Threat.Attacks && !p.Has("attack") {
			t.Logf("фолбэк: %s нападает, но клипа attack нет", id)
		}
		if sp.Locomotion.Water && !p.Has("swim") {
			t.Logf("фолбэк: %s водоплавающее, но клипа swim нет", id)
		}
		if sp.Locomotion.Air && !p.Has("flight") {
			t.Logf("фолбэк: %s летающее, но клипа flight нет", id)
		}
		if !p.Has("idle") {
			t.Logf("фолбэк: у %s нет idle", id)
		}
		if sp.Stats.Speed.Run > 0 && !p.Has("run") {
			t.Logf("фолбэк: %s бегает, но клипа run нет", id)
		}
	}
}
