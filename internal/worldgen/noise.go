package worldgen

// noise.go — детерминированный шум без внешних зависимостей.
// Value-noise на целочисленном хеше + fBm (сумма октав) + domain warp
// (смещение координат вторым полем шума). Всё воспроизводимо по seed:
// одинаковый seed → одинаковая карта.

import "math"

// hash2 — быстрый целочисленный хеш координат решётки с подмешанным seed.
// Возвращает float64 в [0,1). Основан на wang/splitmix-подобном перемешивании.
func hash2(ix, iy int, seed uint64) float64 {
	h := seed
	h ^= uint64(int64(ix)) * 0x9E3779B97F4A7C15
	h ^= uint64(int64(iy)) * 0xC2B2AE3D27D4EB4F
	h ^= h >> 29
	h *= 0xBF58476D1CE4E5B9
	h ^= h >> 27
	h *= 0x94D049BB133111EB
	h ^= h >> 31
	// старшие 53 бита → мантисса float64 в [0,1)
	return float64(h>>11) / float64(uint64(1)<<53)
}

// smootherstep — квинтическая интерполяция 6t^5-15t^4+10t^3 (плавные производные).
func smootherstep(t float64) float64 {
	return t * t * t * (t*(t*6-15) + 10)
}

func lerp(a, b, t float64) float64 { return a + (b-a)*t }

// valueNoise2 — одно октавное value-noise поле в точке (x,y). Диапазон [0,1].
func valueNoise2(x, y float64, seed uint64) float64 {
	x0 := int(math.Floor(x))
	y0 := int(math.Floor(y))
	fx := smootherstep(x - float64(x0))
	fy := smootherstep(y - float64(y0))

	v00 := hash2(x0, y0, seed)
	v10 := hash2(x0+1, y0, seed)
	v01 := hash2(x0, y0+1, seed)
	v11 := hash2(x0+1, y0+1, seed)

	top := lerp(v00, v10, fx)
	bot := lerp(v01, v11, fx)
	return lerp(top, bot, fy)
}

// fbmParams — параметры фрактального суммирования (worldgen.spec §3, §5).
type fbmParams struct {
	Octaves    int
	Freq       float64 // базовая частота (тайлов на период)
	Lacunarity float64 // рост частоты по октавам
	Gain       float64 // спад амплитуды по октавам
}

func defaultFBM() fbmParams {
	return fbmParams{Octaves: 5, Freq: 1.0 / 48.0, Lacunarity: 2.0, Gain: 0.5}
}

// fbm — сумма октав value-noise, нормированная в [0,1].
func fbm(x, y float64, seed uint64, p fbmParams) float64 {
	var sum, amp, norm float64
	freq := p.Freq
	amp = 1
	for i := 0; i < p.Octaves; i++ {
		// разный seed на октаву, чтобы слои не коррелировали
		s := seed + uint64(i)*0x100000001B3
		sum += amp * valueNoise2(x*freq, y*freq, s)
		norm += amp
		amp *= p.Gain
		freq *= p.Lacunarity
	}
	if norm == 0 {
		return 0
	}
	return sum / norm
}

// fbmWarped — fBm с domain warp: координаты сэмплирования смещаются двумя
// дополнительными полями шума. Даёт органические, «извилистые» береговые линии
// вместо круглых пятен (Quílez domain warp).
func fbmWarped(x, y float64, seed uint64, p fbmParams, warp float64) float64 {
	if warp == 0 {
		return fbm(x, y, seed, p)
	}
	// два независимых поля для смещения по X и Y
	qx := fbm(x, y, seed^0xA511E9B3, p)
	qy := fbm(x+31.4, y+17.2, seed^0x1D2C6F5B, p)
	return fbm(x+warp*(qx-0.5)*2, y+warp*(qy-0.5)*2, seed, p)
}
