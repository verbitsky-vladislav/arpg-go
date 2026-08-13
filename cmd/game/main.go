// Команда game — точка входа: создаёт окно, готовит загрузчик ресурсов и
// запускает игру с нужной стартовой сцены.
//
//	go run ./cmd/game          — стартовое меню
//	go run ./cmd/game -zoo     — сцена-зоопарк: все виды животных сеткой
//
// Ресурсы читаются с диска (os.DirFS), а не вшиты в бинарник: пока контент
// правится каждый день, важнее менять PNG без пересборки. Загрузчик работает
// поверх fs.FS, поэтому переход на embed.FS — это одна строка здесь.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/vladislav/game/internal/assets"
	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/core"
	"github.com/vladislav/game/internal/mob"
	"github.com/vladislav/game/internal/scene"
	"github.com/vladislav/game/internal/settings"
)

func main() {
	zoo := flag.Bool("zoo", false, "запустить сцену-зоопарк (все животные сеткой)")
	root := flag.String("assets", "assets", "каталог с ресурсами")
	flag.Parse()

	ebiten.SetWindowTitle("Pixel ARPG")
	ebiten.SetWindowSize(config.WindowW, config.WindowH)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetCursorMode(ebiten.CursorModeHidden) // курсор рисуем сами (ui.DrawCursor)
	settings.Init()

	loader := assets.NewLoader(os.DirFS(*root))

	var start scene.Scene = scene.NewMenu()
	if *zoo {
		s, err := newZoo(loader)
		if err != nil {
			log.Fatal(err)
		}
		start = s
	}

	if err := ebiten.RunGame(core.New(start)); err != nil {
		log.Fatal(err)
	}
}

// newZoo собирает сцену-зоопарк и заодно проверяет таблицу видов: расхождения
// в данных лучше увидеть в консоли при старте, чем ловить их потом в поведении.
func newZoo(l *assets.Loader) (scene.Scene, error) {
	cat, err := mob.LoadSpecies(l.FS(), "mobs/animals/species.json")
	if err != nil {
		return nil, err
	}
	for _, p := range cat.Validate() {
		fmt.Fprintln(os.Stderr, "species.json:", p)
	}
	return scene.NewZoo(l, cat, "mobs/animals"), nil
}
