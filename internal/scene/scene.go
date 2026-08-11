// Package scene — экраны игры (меню, игра, пауза...) и переходы между ними.
// Каждый экран сам решает, каким стать следующим кадром.
package scene

import "github.com/hajimehoshi/ebiten/v2"

// Scene — один экран. Update возвращает сцену следующего кадра
// (обычно себя же; другую — при переходе).
type Scene interface {
	Update() (Scene, error)
	Draw(screen *ebiten.Image)
}
