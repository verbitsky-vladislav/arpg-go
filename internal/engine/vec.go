// Package engine — низкоуровневые движковые утилиты: векторы, математика,
// камера. Зависит только от ebiten, но не от игровой логики.
package engine

import "math"

// Vec2 — 2D-вектор: позиции, скорости, направления.
type Vec2 struct{ X, Y float64 }

func (a Vec2) Add(b Vec2) Vec2      { return Vec2{a.X + b.X, a.Y + b.Y} }
func (a Vec2) Sub(b Vec2) Vec2      { return Vec2{a.X - b.X, a.Y - b.Y} }
func (a Vec2) Scale(s float64) Vec2 { return Vec2{a.X * s, a.Y * s} }
func (a Vec2) Len() float64         { return math.Hypot(a.X, a.Y) }
func (a Vec2) Angle() float64       { return math.Atan2(a.Y, a.X) }
func (a Vec2) Dot(b Vec2) float64   { return a.X*b.X + a.Y*b.Y }

// Normalized — вектор длины 1 (направление). Для нулевого возвращает нулевой.
func (a Vec2) Normalized() Vec2 {
	l := a.Len()
	if l == 0 {
		return Vec2{}
	}
	return Vec2{a.X / l, a.Y / l}
}

// Dist — расстояние между точками.
func Dist(a, b Vec2) float64 { return a.Sub(b).Len() }
