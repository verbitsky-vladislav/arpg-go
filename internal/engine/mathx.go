package engine

// Clamp зажимает v в диапазон [lo, hi].
func Clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Lerp — линейная интерполяция между a и b по t∈[0,1].
func Lerp(a, b, t float64) float64 { return a + (b-a)*t }
