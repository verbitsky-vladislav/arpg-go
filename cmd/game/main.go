// Команда game — точка входа: создаёт окно, готовит загрузчик ресурсов,
// собирает стартовое меню и запускает игру.
//
//	go run ./cmd/game
//	go run ./cmd/game -assets assets
//
// Ресурсы читаются с диска (os.DirFS), а не вшиты в бинарник: пока контент
// правится каждый день, важнее менять PNG без пересборки. Загрузчик работает
// поверх fs.FS, поэтому переход на embed.FS — это одна строка здесь.
package main

import (
	"flag"
	"log"
	"os"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/vladislav/game/internal/assets"
	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/core"
	"github.com/vladislav/game/internal/scene"
	"github.com/vladislav/game/internal/settings"
)

func main() {
	root := flag.String("assets", "assets", "каталог с ресурсами")
	flag.Parse()

	ebiten.SetWindowTitle("Pixel ARPG")
	ebiten.SetWindowSize(config.WindowW, config.WindowH)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetCursorMode(ebiten.CursorModeHidden) // курсор рисуем сами (ui.DrawCursor)
	settings.Init()

	loader := assets.NewLoader(os.DirFS(*root))

	// Экраны за пунктами меню подключаются здесь: сцены знают друг о друге
	// ровно столько, сколько скажет точка входа.
	menu := scene.NewMenu()
	menu.OnBestiary = func() scene.Scene { return scene.NewBestiary(loader, menu) }

	if err := ebiten.RunGame(core.New(menu)); err != nil {
		log.Fatal(err)
	}
}
