package main

import (
	"embed"
	"io/fs"
	"log"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/vladislav/game/internal/assets"
	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/core"
)

// assetsFS вшивает каталог ассетов в бинарник — игра самодостаточна, не зависит
// от текущего каталога запуска. all: включает и файлы со служебными префиксами.
//
//go:embed all:assets
var assetsFS embed.FS

func main() {
	// Обрезаем префикс "assets", чтобы пути внутри были вида "sprites/heroes/...".
	sub, err := fs.Sub(assetsFS, "assets")
	if err != nil {
		log.Fatal(err)
	}
	loader := assets.NewLoader(sub)

	ebiten.SetWindowSize(config.WindowW, config.WindowH)
	ebiten.SetWindowTitle("Pixel ARPG")

	if err := ebiten.RunGame(core.New(loader)); err != nil {
		log.Fatal(err)
	}
}
