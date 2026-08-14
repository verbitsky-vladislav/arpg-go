package worldgen

// props_emit.go — подкоманда `worldgen props <biome-dir>`: пересобрать секцию
// props манифеста по каталогу props/. Правится только ключ "props", остальные
// ключи переписываются исходными байтами — ручная разметка ролей не страдает.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// RunProps сканирует props/ биома и вписывает результат в его manifest.json.
func RunProps(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "worldgen props <biome-dir>")
		os.Exit(2)
	}
	biomeDir := args[0]
	props := scanProps(biomeDir)
	if len(props) == 0 {
		fmt.Fprintf(os.Stderr, "props: в %s нет спрайтов объектов\n", biomeDir)
		os.Exit(1)
	}
	path := filepath.Join(biomeDir, "manifest.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "props: %v\n", err)
		os.Exit(1)
	}
	out, err := replaceJSONKey(raw, "props", props)
	if err != nil {
		fmt.Fprintf(os.Stderr, "props: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "props: %v\n", err)
		os.Exit(1)
	}

	// сводка по типам — видно, что каталог разобран так, как ожидалось
	byKind := map[string]int{}
	swaps := 0
	for _, p := range props {
		byKind[propKind(p)]++
		if p.SwapOnGroundB != "" {
			swaps++
		}
	}
	fmt.Fprintf(os.Stderr, "%s: %d объектов (%v), замен под ground_b %d\n",
		path, len(props), byKind, swaps)
}

// propKind — обратная классификация для сводки (по выставленным полям).
func propKind(p Prop) string {
	switch {
	case p.Prefab:
		return "ruin"
	case p.Collides:
		return "tree"
	case len(p.On) > 1:
		return "stone"
	case len(p.Zone) == 1 && p.Zone[0] == "dense":
		return "mushroom"
	default:
		return "bush"
	}
}

// replaceJSONKey заменяет значение ключа верхнего уровня (или дописывает ключ в
// конец), сохраняя порядок и исходное форматирование остальных ключей.
func replaceJSONKey(raw []byte, key string, val any) ([]byte, error) {
	type pair struct {
		key string
		val json.RawMessage
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil, fmt.Errorf("манифест: ожидался объект верхнего уровня")
	}
	var pairs []pair
	for {
		t, err := dec.Token()
		if err == io.EOF || t == json.Delim('}') {
			break
		}
		if err != nil {
			return nil, err
		}
		k, ok := t.(string)
		if !ok {
			return nil, fmt.Errorf("манифест: ожидалось имя ключа, получено %v", t)
		}
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, err
		}
		pairs = append(pairs, pair{k, v})
	}

	// отступ манифеста — один пробел на уровень (как во всех биомах)
	body, err := json.MarshalIndent(val, " ", " ")
	if err != nil {
		return nil, err
	}
	replaced := false
	for i := range pairs {
		if pairs[i].key == key {
			pairs[i].val = body
			replaced = true
		}
	}
	if !replaced {
		pairs = append(pairs, pair{key, body})
	}

	var buf bytes.Buffer
	buf.WriteString("{\n")
	for i, p := range pairs {
		fmt.Fprintf(&buf, " %q: %s", p.key, p.val)
		if i < len(pairs)-1 {
			buf.WriteByte(',')
		}
		buf.WriteByte('\n')
	}
	buf.WriteString("}\n")
	return buf.Bytes(), nil
}
