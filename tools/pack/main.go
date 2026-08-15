// Команда pack собирает каталог ресурсов для раздачи: в него едет только то,
// что игрок в этой сборке может увидеть.
//
// Правило простое и одно: игра ходит в один биом (internal/scene.GameBiome), а
// значит на диске раздачи нечего делать ни семнадцати остальным биомам, ни
// демонам с личами, которые водятся только в подземельях, ни домам деревни, к
// которым ещё нет кода. Полный каталог assets/ весит около 80 МБ, из них в
// забеге показывается меньше десятой части — остальное игрок скачивал бы зря.
//
//	go run ./tools/pack -out dist/assets
//	go run ./tools/pack -out dist/assets -biomes forest,swamp
//
// Выброшенные существа исчезают не только с диска, но и из таблиц: бестиарий
// показывает то, что лежит рядом, а не то, что когда-то было задумано. Поэтому
// pack переписывает species.json / enemies.json / bosses.json, а не только
// копирует папки со спрайтами.
//
// Собранное проверяется теми же загрузчиками, которыми пользуется игра
// (-check, включена по умолчанию): каталог, где ссылка ведёт в пустоту, — это
// сборка, которая упадёт у игрока, и лучше ей упасть здесь.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vladislav/game/internal/character"
	"github.com/vladislav/game/internal/item"
	"github.com/vladislav/game/internal/mob"
	"github.com/vladislav/game/internal/scene"
	"github.com/vladislav/game/internal/worldgen"
)

// Пути таблиц. Совпадают с теми, что зашиты в сценах: pack читает те же файлы,
// что и игра, и других мест не знает.
const (
	speciesFile  = "mobs/animals/species.json"
	enemiesFile  = "mobs/enemies/enemies.json"
	bossesFile   = "mobs/bosses/bosses.json"
	enemySpawn   = "mobs/enemies/spawn.json"
	musicFile    = "audio/music/music.json"
	animalsRoot  = "mobs/animals"
	enemiesRoot  = "mobs/enemies"
	bossesRoot   = "mobs/bosses"
	biomesRoot   = "biomes"
	locationsDir = "locations"
	// authoring — исходники разметки тайлсета (карты-примеры, .tmx, эталоны).
	// Ими пользуется художник и tools/worldgen, игра — никогда.
	authoringDir = "authoring"
)

func main() {
	src := flag.String("src", "assets", "каталог ресурсов проекта")
	out := flag.String("out", filepath.Join("dist", "assets"), "куда сложить сборку")
	biomes := flag.String("biomes", scene.GameBiome, "биомы сборки через запятую")
	check := flag.Bool("check", true, "проверить собранное загрузчиками игры")
	flag.Parse()

	keepBiomes := set(strings.Split(*biomes, ","))
	if len(keepBiomes) == 0 {
		fatal(fmt.Errorf("не задано ни одного биома"))
	}

	p := &packer{src: *src, out: *out, biomes: keepBiomes}
	if err := p.run(); err != nil {
		fatal(err)
	}
	if *check {
		if err := verify(*out, keepBiomes); err != nil {
			fatal(err)
		}
	}
	p.report()
}

type packer struct {
	src, out string
	biomes   map[string]bool

	species map[string]bool // виды животных, которые едут в сборку
	enemies map[string]bool // типы врагов
	bosses  map[string]bool // типы боссов

	srcBytes, outBytes int64
	total              int // сколько существ описано в проекте — для отчёта
	dropped            []string
}

func (p *packer) run() error {
	var err error
	if p.species, err = p.pickSpecies(); err != nil {
		return err
	}
	if p.enemies, err = p.pickEnemies(enemiesFile); err != nil {
		return err
	}
	if p.bosses, err = p.pickEnemies(bossesFile); err != nil {
		return err
	}
	if err := os.RemoveAll(p.out); err != nil {
		return err
	}
	if err := p.copyTree(); err != nil {
		return err
	}
	return p.rewriteTables()
}

// ── кого берём ────────────────────────────────────────────────────────────

// habitat — общая часть записи зверя и врага: где водится.
type habitat struct {
	Art     string `json:"art"`
	Habitat struct {
		Biomes map[string]float64 `json:"biomes"`
	} `json:"habitat"`
}

// speciesEntry — плюс ссылки на других зверей. Взрослый тянет за собой
// детёныша и тех, с кем выходит: они появятся вместе с ним, значит поедут в
// сборку. Добыча (threat.prey) — не тянет: лиса охотится на кроликов, но
// кроликов в лесу нет, и везти их ради строчки в таблице незачем. Вместо
// этого список добычи потом чистится (prunePrey) — иначе таблица останется со
// ссылкой в пустоту, и Validate честно на неё пожалуется.
type speciesEntry struct {
	habitat
	GrowsInto string `json:"grows_into"`
	YoungForm string `json:"young_form"`
	Spawn     struct {
		With string `json:"with"`
	} `json:"spawn"`
	Threat struct {
		Prey []string `json:"prey"`
	} `json:"threat"`
}

func (p *packer) pickSpecies() (map[string]bool, error) {
	raw, err := p.table(speciesFile, "species")
	if err != nil {
		return nil, err
	}
	all := map[string]speciesEntry{}
	for id, b := range raw {
		var e speciesEntry
		if err := json.Unmarshal(b, &e); err != nil {
			return nil, fmt.Errorf("%s: %s: %w", speciesFile, id, err)
		}
		all[id] = e
	}
	keep := map[string]bool{}
	var walk func(string)
	walk = func(id string) {
		e, ok := all[id]
		if !ok || keep[id] {
			return
		}
		keep[id] = true
		for _, ref := range []string{e.GrowsInto, e.YoungForm, e.Spawn.With} {
			if ref != "" {
				walk(ref)
			}
		}
	}
	for id, e := range all {
		if p.lives(e.habitat) {
			walk(id)
		}
	}
	return keep, nil
}

func (p *packer) pickEnemies(file string) (map[string]bool, error) {
	raw, err := p.table(file, "types")
	if err != nil {
		return nil, err
	}
	all := map[string]habitat{}
	keep := map[string]bool{}
	for id, b := range raw {
		var e habitat
		if err := json.Unmarshal(b, &e); err != nil {
			return nil, fmt.Errorf("%s: %s: %w", file, id, err)
		}
		all[id] = e
		if p.lives(e) {
			keep[id] = true
		}
	}
	p.total += len(all)
	return keep, nil
}

// lives — водится ли существо хоть в одном биоме сборки.
func (p *packer) lives(h habitat) bool {
	for b, w := range h.Habitat.Biomes {
		if p.biomes[b] && w > 0 {
			return true
		}
	}
	return false
}

// table достаёт из файла таблицы вложенный объект (species/types) записями.
func (p *packer) table(file, key string) (map[string]json.RawMessage, error) {
	b, err := os.ReadFile(filepath.Join(p.src, filepath.FromSlash(file)))
	if err != nil {
		return nil, err
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	var inner map[string]json.RawMessage
	if err := json.Unmarshal(doc[key], &inner); err != nil {
		return nil, fmt.Errorf("%s: %s: %w", file, key, err)
	}
	return inner, nil
}

// ── копирование ───────────────────────────────────────────────────────────

// copyTree переносит ресурсы, спрашивая про каждый файл, нужен ли он.
func (p *packer) copyTree() error {
	root := os.DirFS(p.src)
	return fs.WalkDir(root, ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil || rel == "." {
			return err
		}
		if d.IsDir() {
			if p.keep(rel + "/") {
				return nil
			}
			p.dropped = append(p.dropped, rel)
			p.srcBytes += dirSize(filepath.Join(p.src, filepath.FromSlash(rel)))
			return fs.SkipDir
		}
		info, ierr := d.Info()
		if ierr != nil {
			return ierr
		}
		p.srcBytes += info.Size()
		if !p.keep(rel) {
			return nil
		}
		p.outBytes += info.Size()
		return copyFile(filepath.Join(p.src, filepath.FromSlash(rel)),
			filepath.Join(p.out, filepath.FromSlash(rel)))
	})
}

// keep — единственное место, где решается судьба файла. Путь всегда со
// слэшами и относительно assets/; у каталога в конце слэш.
func (p *packer) keep(rel string) bool {
	parts := strings.Split(strings.TrimSuffix(rel, "/"), "/")
	switch parts[0] {
	case locationsDir:
		// Дома, таверны и кузницы: ассеты есть, кода под них нет.
		return false
	case biomesRoot:
		if len(parts) < 2 {
			return true
		}
		if !p.biomes[parts[1]] {
			return false
		}
		return len(parts) < 3 || parts[2] != authoringDir
	case "mobs":
		// Таблицы (species.json, enemies.json, spawn.json) лежат на той же
		// глубине, что и папки паков, но едут всегда: чистит их не копировщик,
		// а rewriteTables.
		if len(parts) < 3 || strings.HasSuffix(parts[2], ".json") {
			return true
		}
		switch strings.Join(parts[:2], "/") {
		case animalsRoot:
			return p.species[parts[2]]
		case enemiesRoot:
			return p.enemies[parts[2]]
		case bossesRoot:
			return p.bosses[parts[2]]
		}
		return true
	}
	return true
}

// ── таблицы ───────────────────────────────────────────────────────────────

// rewriteTables переписывает то, что после чистки перестало сходиться с
// диском: списки существ, настройки спавна, ссылающиеся на выброшенные типы, и
// список музыкальных тем.
func (p *packer) rewriteTables() error {
	if err := p.filterTable(speciesFile, "species", p.species); err != nil {
		return err
	}
	if err := p.filterTable(enemiesFile, "types", p.enemies); err != nil {
		return err
	}
	if err := p.filterTable(bossesFile, "types", p.bosses); err != nil {
		return err
	}
	if err := p.prunePrey(); err != nil {
		return err
	}
	if err := p.filterSpawn(); err != nil {
		return err
	}
	return p.filterMusic()
}

// filterTable оставляет в объекте key только записи из keep.
func (p *packer) filterTable(file, key string, keep map[string]bool) error {
	doc, err := p.readOut(file)
	if err != nil {
		return err
	}
	var inner map[string]json.RawMessage
	if err := json.Unmarshal(doc[key], &inner); err != nil {
		return fmt.Errorf("%s: %s: %w", file, key, err)
	}
	for id := range inner {
		if !keep[id] {
			delete(inner, id)
		}
	}
	b, err := json.MarshalIndent(inner, "", " ")
	if err != nil {
		return err
	}
	doc[key] = b
	return p.writeOut(file, doc)
}

// prunePrey вычёркивает из списков добычи виды, не доехавшие до сборки.
func (p *packer) prunePrey() error {
	doc, err := p.readOut(speciesFile)
	if err != nil {
		return err
	}
	var species map[string]json.RawMessage
	if err := json.Unmarshal(doc["species"], &species); err != nil {
		return fmt.Errorf("%s: species: %w", speciesFile, err)
	}
	for id, raw := range species {
		var entry map[string]json.RawMessage
		if err := json.Unmarshal(raw, &entry); err != nil {
			return fmt.Errorf("%s: %s: %w", speciesFile, id, err)
		}
		var threat map[string]json.RawMessage
		if err := json.Unmarshal(entry["threat"], &threat); err != nil {
			return fmt.Errorf("%s: %s.threat: %w", speciesFile, id, err)
		}
		var prey []string
		if err := json.Unmarshal(threat["prey"], &prey); err != nil {
			return fmt.Errorf("%s: %s.threat.prey: %w", speciesFile, id, err)
		}
		kept := []string{}
		for _, v := range prey {
			if p.species[v] {
				kept = append(kept, v)
			}
		}
		if len(kept) == len(prey) {
			continue
		}
		if threat["prey"], err = json.Marshal(kept); err != nil {
			return err
		}
		if entry["threat"], err = json.MarshalIndent(threat, "", " "); err != nil {
			return err
		}
		if species[id], err = json.MarshalIndent(entry, "", " "); err != nil {
			return err
		}
	}
	b, err := json.MarshalIndent(species, "", " ")
	if err != nil {
		return err
	}
	doc["species"] = b
	return p.writeOut(speciesFile, doc)
}

// filterSpawn чистит настройки заселения от выброшенных типов и биомов:
// поправка на демонов в сборке без демонов — это не безобидный мусор, а
// расхождение таблицы с миром, из которого потом рождаются вопросы.
func (p *packer) filterSpawn() error {
	doc, err := p.readOut(enemySpawn)
	if err != nil {
		return err
	}
	if err := pruneNested(doc, "limits", "per_type_overrides", func(k string) bool { return p.enemies[k] }); err != nil {
		return err
	}
	if err := pruneNested(doc, "tiers", "biome_shift", func(k string) bool { return p.biomes[k] }); err != nil {
		return err
	}
	return p.writeOut(enemySpawn, doc)
}

// filterMusic оставляет темы биомов, которые едут в сборку. Тема, названная не
// по биому (меню, титры), остаётся: она не про биом и выброшена быть не может.
func (p *packer) filterMusic() error {
	doc, err := p.readOut(musicFile)
	if err != nil {
		return err
	}
	var tracks map[string]struct {
		File string  `json:"file"`
		Gain float64 `json:"gain"`
	}
	if err := json.Unmarshal(doc["tracks"], &tracks); err != nil {
		return fmt.Errorf("%s: %w", musicFile, err)
	}
	all, err := p.biomeDirs()
	if err != nil {
		return err
	}
	for id, t := range tracks {
		if !all[id] || p.biomes[id] {
			continue
		}
		delete(tracks, id)
		_ = os.Remove(filepath.Join(p.out, "audio", "music", filepath.FromSlash(t.File)))
	}
	b, err := json.MarshalIndent(tracks, "", " ")
	if err != nil {
		return err
	}
	doc["tracks"] = b
	return p.writeOut(musicFile, doc)
}

// biomeDirs — все биомы, какие есть в проекте (не только сборочные).
func (p *packer) biomeDirs() (map[string]bool, error) {
	ents, err := os.ReadDir(filepath.Join(p.src, biomesRoot))
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, e := range ents {
		if e.IsDir() {
			out[e.Name()] = true
		}
	}
	return out, nil
}

func pruneNested(doc map[string]json.RawMessage, outer, inner string, keep func(string) bool) error {
	raw, ok := doc[outer]
	if !ok {
		return nil
	}
	var sect map[string]json.RawMessage
	if err := json.Unmarshal(raw, &sect); err != nil {
		return fmt.Errorf("%s: %w", outer, err)
	}
	sub, ok := sect[inner]
	if !ok {
		return nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(sub, &m); err != nil {
		return fmt.Errorf("%s.%s: %w", outer, inner, err)
	}
	for k := range m {
		if !keep(k) {
			delete(m, k)
		}
	}
	b, err := json.MarshalIndent(m, "", " ")
	if err != nil {
		return err
	}
	sect[inner] = b
	if b, err = json.MarshalIndent(sect, "", " "); err != nil {
		return err
	}
	doc[outer] = b
	return nil
}

func (p *packer) readOut(file string) (map[string]json.RawMessage, error) {
	b, err := os.ReadFile(filepath.Join(p.out, filepath.FromSlash(file)))
	if err != nil {
		return nil, err
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	return doc, nil
}

func (p *packer) writeOut(file string, doc map[string]json.RawMessage) error {
	b, err := json.MarshalIndent(doc, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(p.out, filepath.FromSlash(file)), append(b, '\n'), 0o644)
}

// ── проверка ──────────────────────────────────────────────────────────────

// verify прогоняет собранное теми же загрузчиками, что и игра, и проверяет,
// что у каждого оставшегося существа на диске лежат все кадры.
func verify(out string, biomes map[string]bool) error {
	fsys := os.DirFS(out)
	for b := range biomes {
		m, err := worldgen.LoadManifest(filepath.Join(out, biomesRoot, b))
		if err != nil {
			return fmt.Errorf("биом %s: %w", b, err)
		}
		if _, err := worldgen.NewAtlasSet(m); err != nil {
			return fmt.Errorf("биом %s: атласы: %w", b, err)
		}
	}

	sp, err := mob.LoadSpecies(fsys, speciesFile)
	if err != nil {
		return err
	}
	if probs := sp.Validate(); len(probs) > 0 {
		return fmt.Errorf("%s: %s", speciesFile, strings.Join(probs, "; "))
	}
	for _, id := range sp.IDs() {
		if err := packOK(fsys, path.Join(animalsRoot, sp.Get(id).Art)); err != nil {
			return err
		}
	}

	for file, root := range map[string]string{enemiesFile: enemiesRoot, bossesFile: bossesRoot} {
		cat, err := mob.LoadEnemies(fsys, file)
		if err != nil {
			return err
		}
		if probs := cat.Validate(); len(probs) > 0 {
			return fmt.Errorf("%s: %s", file, strings.Join(probs, "; "))
		}
		for _, tid := range cat.TypeIDs() {
			t := cat.Types[tid]
			for _, tier := range t.TierIDs() {
				if err := packOK(fsys, path.Join(root, t.Art, tier)); err != nil {
					return err
				}
			}
		}
	}

	if _, err := mob.LoadBehavior(fsys, "mobs/enemies/behavior.json"); err != nil {
		return err
	}
	if _, err := mob.LoadEnemySpawn(fsys, enemySpawn); err != nil {
		return err
	}
	if _, err := mob.LoadSpawnConfig(fsys, "mobs/animals/spawn.json"); err != nil {
		return err
	}
	cat, err := character.Load(fsys, "character/character.json")
	if err != nil {
		return err
	}
	if probs := cat.Validate(); len(probs) > 0 {
		return fmt.Errorf("character.json: %s", strings.Join(probs, "; "))
	}
	for _, bid := range cat.BodyIDs() {
		for _, lid := range cat.LoadoutIDs() {
			if err := packOK(fsys, path.Join("character", cat.Body(bid).Art, cat.Loadout(lid).Art)); err != nil {
				return err
			}
		}
	}
	if _, err := item.Load(fsys, "items/items.json"); err != nil {
		return err
	}
	return nil
}

// packOK — у пака есть манифест и все файлы, которые он обещает.
func packOK(fsys fs.FS, dir string) error {
	b, err := fs.ReadFile(fsys, path.Join(dir, "manifest.json"))
	if err != nil {
		return fmt.Errorf("пак %s: %w", dir, err)
	}
	var m struct {
		Animations map[string]struct {
			File string `json:"file"`
		} `json:"animations"`
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return fmt.Errorf("пак %s: %w", dir, err)
	}
	for name, a := range m.Animations {
		if _, err := fs.Stat(fsys, path.Join(dir, a.File)); err != nil {
			return fmt.Errorf("пак %s: клип %s: %w", dir, name, err)
		}
	}
	return nil
}

// ── отчёт и мелочи ────────────────────────────────────────────────────────

func (p *packer) report() {
	sort.Strings(p.dropped)
	fmt.Printf("биомы:  %s\n", strings.Join(keys(p.biomes), ", "))
	fmt.Printf("звери:  %d\n", len(p.species))
	fmt.Printf("враги:  %d типов, боссов %d\n", len(p.enemies), len(p.bosses))
	fmt.Printf("размер: %.1f МБ из %.1f МБ (%.0f%%)\n",
		mb(p.outBytes), mb(p.srcBytes), 100*float64(p.outBytes)/float64(max(p.srcBytes, 1)))
	fmt.Printf("выброшено каталогов: %d\n", len(p.dropped))
}

func mb(b int64) float64 { return float64(b) / (1 << 20) }

func dirSize(dir string) int64 {
	var n int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			n += info.Size()
		}
		return nil
	})
	return n
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func set(ss []string) map[string]bool {
	out := map[string]bool{}
	for _, s := range ss {
		if s = strings.TrimSpace(s); s != "" {
			out[s] = true
		}
	}
	return out
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "pack:", err)
	os.Exit(1)
}
