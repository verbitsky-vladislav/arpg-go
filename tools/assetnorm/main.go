// Command assetnorm normalizes raw CraftPix creature packs into a uniform,
// engine-friendly layout with a manifest.json next to each creature variant.
//
// Source layout (per pack):
//
//	<src>/PNG/<Variant>/With_shadow/<Name>_<Anim>_with_shadow.png
//	<src>/PNG/<Variant>/Parts/*.png
//
// Output layout (per variant):
//
//	<out>/<group>/<type>/<variant>/idle.png walk.png ... manifest.json parts/*.png
//
// Frame grid: creatures are 4-directional, so frameSize = sheetHeight/4
// (64 or 128 px), frameCount = sheetWidth/frameSize.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Mapping struct {
	FrameDefaults struct {
		Directions int `json:"directions"`
		FPS        int `json:"fps"`
	} `json:"frameDefaults"`
	Packs []Pack `json:"packs"`
}

type Pack struct {
	Src      string            `json:"src"`
	Group    string            `json:"group"`
	Type     string            `json:"type"`
	Category string            `json:"category"`
	Scheme   string            `json:"scheme"` // "rank" | "named"
	Names    map[string]string `json:"names"`
}

type Manifest struct {
	ID         string               `json:"id"`
	Name       string               `json:"name"`
	Type       string               `json:"type"`
	Category   string               `json:"category"`
	SourcePack string               `json:"source_pack"`
	Frame      Frame                `json:"frame"`
	Directions []string             `json:"directions"`
	Animations map[string]Animation `json:"animations"`
}

type Frame struct {
	W int `json:"w"`
	H int `json:"h"`
}

type Animation struct {
	File   string `json:"file"`
	Frames int    `json:"frames"`
	FPS    int    `json:"fps"`
	Loop   bool   `json:"loop"`
}

var dir4 = []string{"down", "up", "left", "right"}

// loopingAnim reports whether an animation should loop by default.
func loopingAnim(a string) bool {
	switch a {
	case "idle", "walk", "run", "flight":
		return true
	default:
		return false
	}
}

// classifyAnim maps a raw sheet filename to a canonical animation name.
// Keyword-based (not positional) because packs are inconsistent: some files
// are "Skeleton1_Idle_with_shadow.png", others "idle_with_shadow.png", plus
// combos like "Run_Attack" and stray trailing spaces/digits.
func classifyAnim(fname string) string {
	s := strings.ToLower(fname)
	s = strings.TrimSuffix(s, ".png")
	s = strings.ReplaceAll(s, "_with_shadow", "")
	s = strings.ReplaceAll(s, "with_shadow", "")
	has := func(sub string) bool { return strings.Contains(s, sub) }
	switch {
	case has("run") && has("attack"):
		return "run_attack"
	case has("walk") && has("attack"):
		return "walk_attack"
	case has("attack2"):
		return "attack2"
	case has("attack"):
		return "attack"
	case has("idle"):
		return "idle"
	case has("walk"):
		return "walk"
	case has("run"):
		return "run"
	case has("hurt"):
		return "hurt"
	case has("death") || has("die"):
		return "death"
	case has("flight") || has("fly"):
		return "flight"
	}
	return ""
}

func pngSize(path string) (w, h int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0, err
	}
	return cfg.Width, cfg.Height, nil
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

// listVariants returns variant subdirs under <src>/PNG that contain a
// With_shadow (case-insensitive) folder, sorted by name.
func listVariants(src string) ([]string, error) {
	pngDir := filepath.Join(src, "PNG")
	entries, err := os.ReadDir(pngDir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if findWithShadow(filepath.Join(pngDir, e.Name())) != "" {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

// findWithShadow returns the path of the With_shadow dir (any case) or "".
func findWithShadow(variantDir string) string {
	entries, err := os.ReadDir(variantDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() && strings.EqualFold(e.Name(), "with_shadow") {
			return filepath.Join(variantDir, e.Name())
		}
	}
	return ""
}

func variantName(p Pack, raw string, idx int) string {
	if p.Scheme == "named" {
		if n, ok := p.Names[raw]; ok {
			return n
		}
		return strings.ToLower(raw)
	}
	return fmt.Sprintf("t%d", idx+1) // rank
}

func processVariant(assetsRoot, outRoot string, m *Mapping, p Pack, rawVariant, name string) error {
	src := filepath.Join(assetsRoot, p.Src)
	vDir := filepath.Join(src, "PNG", rawVariant)
	wsDir := findWithShadow(vDir)
	dst := filepath.Join(outRoot, p.Group, p.Type, name)

	sheets, err := os.ReadDir(wsDir)
	if err != nil {
		return err
	}
	anims := map[string]Animation{}
	frame := Frame{}
	dirs := m.FrameDefaults.Directions
	for _, sh := range sheets {
		if sh.IsDir() || !strings.HasSuffix(strings.ToLower(sh.Name()), ".png") {
			continue
		}
		anim := classifyAnim(sh.Name())
		if anim == "" {
			fmt.Printf("  WARN skip unclassified: %s/%s\n", rawVariant, sh.Name())
			continue
		}
		if _, dup := anims[anim]; dup {
			fmt.Printf("  WARN dup anim %q (%s); keeping first\n", anim, sh.Name())
			continue
		}
		srcSheet := filepath.Join(wsDir, sh.Name())
		w, h, err := pngSize(srcSheet)
		if err != nil {
			return fmt.Errorf("size %s: %w", srcSheet, err)
		}
		fw := h / dirs // 4-directional: frame is square, size = height/dirs
		if fw == 0 || w%fw != 0 || h%fw != 0 {
			fmt.Printf("  WARN odd grid %s: %dx%d fw=%d\n", sh.Name(), w, h, fw)
		}
		if frame.W == 0 {
			frame = Frame{W: fw, H: fw}
		}
		if err := copyFile(srcSheet, filepath.Join(dst, anim+".png")); err != nil {
			return err
		}
		frames := 0
		if fw > 0 {
			frames = w / fw
		}
		anims[anim] = Animation{File: anim + ".png", Frames: frames, FPS: m.FrameDefaults.FPS, Loop: loopingAnim(anim)}
	}

	// copy layered parts (kept for recolor / equipment)
	if partsDir := findSibling(vDir, "parts"); partsDir != "" {
		parts, _ := os.ReadDir(partsDir)
		for _, pt := range parts {
			if pt.IsDir() || !strings.HasSuffix(strings.ToLower(pt.Name()), ".png") {
				continue
			}
			_ = copyFile(filepath.Join(partsDir, pt.Name()), filepath.Join(dst, "parts", strings.ToLower(pt.Name())))
		}
	}

	man := Manifest{
		ID: p.Type + "_" + name, Name: name, Type: p.Type, Category: p.Category,
		SourcePack: filepath.Base(p.Src), Frame: frame, Directions: dir4[:dirs], Animations: anims,
	}
	buf, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dst, "manifest.json"), append(buf, '\n'), 0o644)
}

func findSibling(dir, name string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() && strings.EqualFold(e.Name(), name) {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}

func main() {
	assetsRoot := flag.String("assets", "assets", "path to assets root")
	mapPath := flag.String("mapping", "tools/assetnorm/mapping.json", "path to mapping.json")
	outRoot := flag.String("out", "", "output root (staging dir)")
	flag.Parse()
	if *outRoot == "" {
		fmt.Fprintln(os.Stderr, "-out is required")
		os.Exit(2)
	}

	raw, err := os.ReadFile(*mapPath)
	must(err)
	var m Mapping
	must(json.Unmarshal(raw, &m))

	total := 0
	for _, p := range m.Packs {
		variants, err := listVariants(filepath.Join(*assetsRoot, p.Src))
		if err != nil {
			fmt.Printf("ERR %s: %v\n", p.Src, err)
			continue
		}
		for i, rv := range variants {
			name := variantName(p, rv, i)
			if err := processVariant(*assetsRoot, *outRoot, &m, p, rv, name); err != nil {
				fmt.Printf("ERR %s/%s: %v\n", p.Src, rv, err)
				continue
			}
			fmt.Printf("ok %s/%s -> %s/%s/%s\n", filepath.Base(p.Src), rv, p.Group, p.Type, name)
			total++
		}
	}
	fmt.Printf("\ndone: %d variants normalized into %s\n", total, *outRoot)
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
