// Команда worldgen — командная строка генератора карт (internal/worldgen).
// Сам пайплайн живёт в библиотеке, потому что им же генерирует карты игра;
// здесь только разбор подкоманды.
//
//	go run ./tools/worldgen <biome-dir> [-seeds 6] [-size 256] [-out dir]
//	go run ./tools/worldgen audit
//	go run ./tools/worldgen index <biome-dir> <sheet>
//	go run ./tools/worldgen tmxhint <biome-dir>
package main

import (
	"os"

	"github.com/vladislav/game/internal/worldgen"
)

func main() {
	if len(os.Args) < 2 {
		worldgen.Usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "audit":
		worldgen.RunAudit(os.Args[2:])
	case "index":
		worldgen.RunIndex(os.Args[2:])
	case "tmxhint":
		worldgen.RunTMXHint(os.Args[2:])
	case "props":
		worldgen.RunProps(os.Args[2:])
	case "plateautest":
		worldgen.RunPlateauTest(os.Args[2:])
	case "ref":
		worldgen.RunRef(os.Args[2:])
	case "tiles":
		worldgen.RunTilesDump(os.Args[2:])
	case "tmx":
		worldgen.RunTMXExport(os.Args[2:])
	case "level":
		worldgen.RunLevelDump(os.Args[2:])
	case "-h", "--help", "help":
		worldgen.Usage()
	default:
		worldgen.RunGenerate(os.Args[1:])
	}
}
