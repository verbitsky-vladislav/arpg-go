package main

// export_json.go — запись карты в map_format v1 (JSON). Плотные слои пишутся
// плоскими массивами; при желании сжатие/RLE — будущая работа (см. map_format.md).

import (
	"encoding/json"
	"os"
)

func writeMapJSON(path string, mp *MapV1) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	return enc.Encode(mp)
}
