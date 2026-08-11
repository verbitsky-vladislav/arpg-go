package engine

// Camera переводит мировые координаты в экранные. Держит центр обзора в мире
// и следует за целью (игроком), не выходя за границы мира.
type Camera struct {
	Pos          Vec2 // центр камеры в мировых координатах
	ViewW, ViewH float64
}

func NewCamera(viewW, viewH float64) *Camera {
	return &Camera{ViewW: viewW, ViewH: viewH}
}

// Follow центрирует камеру на цели, зажимая её в границы мира [0..worldW/H].
func (c *Camera) Follow(target Vec2, worldW, worldH float64) {
	c.Pos.X = Clamp(target.X, c.ViewW/2, worldW-c.ViewW/2)
	c.Pos.Y = Clamp(target.Y, c.ViewH/2, worldH-c.ViewH/2)
}

// WorldToScreen: мировая точка → пиксель на экране.
func (c *Camera) WorldToScreen(p Vec2) Vec2 {
	return Vec2{p.X - c.Pos.X + c.ViewW/2, p.Y - c.Pos.Y + c.ViewH/2}
}

// ScreenToWorld: пиксель на экране → мировая точка (для прицела мышью).
func (c *Camera) ScreenToWorld(p Vec2) Vec2 {
	return Vec2{p.X + c.Pos.X - c.ViewW/2, p.Y + c.Pos.Y - c.ViewH/2}
}
