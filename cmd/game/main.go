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
	"path/filepath"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/vladislav/game/internal/assets"
	"github.com/vladislav/game/internal/audio"
	"github.com/vladislav/game/internal/config"
	"github.com/vladislav/game/internal/core"
	"github.com/vladislav/game/internal/scene"
	"github.com/vladislav/game/internal/settings"
	"github.com/vladislav/game/internal/ui"
)

func main() {
	root := flag.String("assets", "", "каталог с ресурсами (по умолчанию — рядом с игрой)")
	flag.Parse()
	if *root == "" {
		*root = findAssets()
	}

	ebiten.SetWindowTitle("Pixel ARPG")
	ebiten.SetWindowSize(config.WindowW, config.WindowH)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetCursorMode(ebiten.CursorModeHidden) // курсор рисуем сами (ui.DrawCursor)
	// Закрытие окна разбирает игра, а не движок: перед выходом забег
	// сохраняется (см. core.Game.Update и scene.Closer).
	ebiten.SetWindowClosingHandled(true)
	settings.Init()

	loader := assets.NewLoader(os.DirFS(*root))

	// Полосы состояния — арт, а не код, и их отсутствие не повод не запускаться:
	// HUD в этом случае рисует запасную прямоугольную полосу.
	if err := ui.InitBars(loader, "ui/bars"); err != nil {
		log.Println("полосы здоровья:", err)
	}
	// Окна интерфейса (сундук, сумка) — тоже арт с ручной разметкой.
	if err := ui.InitPanels(loader, "ui/rpg_basic"); err != nil {
		log.Println("окна интерфейса:", err)
	}
	// Звук — один банк на процесс, и он тоже не обязателен: без звуковой карты
	// (и в отладочных прогонах) игра идёт молча, а не падает.
	if err := audio.Init(loader.FS(), "audio"); err != nil {
		log.Println("звук:", err)
	}
	s := settings.Get()
	audio.SetVolume(s.Volume, s.SFXVolume, s.MusicVolume)

	// Экраны за пунктами меню подключаются здесь: сцены знают друг о друге
	// ровно столько, сколько скажет точка входа.
	menu := scene.NewMenu()
	// Игра начинается не с героя, а с персонажа: сначала слот в книге
	// сохранений (их три), и уже он решает — продолжить забег или завести
	// нового героя с выбором тела и имени.
	menu.OnStart = func() scene.Scene { return scene.NewProfiles(loader, menu) }
	menu.OnSettings = func() scene.Scene { return scene.NewSettings(menu) }
	menu.OnBestiary = func() scene.Scene { return scene.NewBestiary(loader, menu) }

	if err := ebiten.RunGame(core.New(menu)); err != nil {
		log.Fatal(err)
	}
}

// findAssets ищет каталог ресурсов: сначала в текущем каталоге (так игра
// запускается из исходников), потом рядом с бинарником (так она запускается из
// раздачи — распакованной папки, по которой кликнули из проводника).
//
// Второе не прихоть: запущенная двойным кликом программа получает рабочим
// каталогом что угодно — домашний каталог, корень, — и «assets» относительно
// него не существует. Игра при этом лежит ровно рядом со своими ресурсами.
func findAssets() string {
	const name = "assets"
	if st, err := os.Stat(name); err == nil && st.IsDir() {
		return name
	}
	exe, err := os.Executable()
	if err != nil {
		return name
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return name
	}
	near := filepath.Join(filepath.Dir(exe), name)
	if st, err := os.Stat(near); err == nil && st.IsDir() {
		return near
	}
	return name
}
