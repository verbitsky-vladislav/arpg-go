package main

import (
	"log"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/core"
	"github.com/vladislav/game/internal/settings"
)

func main() {
	ebiten.SetWindowSize(config.WindowW, config.WindowH)
	ebiten.SetWindowTitle("Pixel ARPG")
	ebiten.SetCursorMode(ebiten.CursorModeHidden) // рисуем свой игровой курсор

	// Загружаем сохранённые настройки и применяем их (fullscreen/vsync/масштаб/
	// FPS). После создания окна — apply() трогает окно движка.
	settings.Init()

	if err := ebiten.RunGame(core.New()); err != nil {
		log.Fatal(err)
	}
}
