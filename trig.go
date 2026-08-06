package dusk

import "math"

const (
	degToRad = math.Pi / 180.0
	radToDeg = 180.0 / math.Pi
)

// clamp restricts x to [-1, 1] before passing it to asin/acos. This prevents
// NaN from floating-point rounding in trig chains. Note: it also silently
// clamps genuinely wrong values (e.g., a miscalculated 1.3 → 1), which could
// mask upstream bugs. Correctness is validated by test coverage against Meeus
// and USNO reference data rather than runtime detection.
func clamp(x float64) float64 { return math.Max(-1, math.Min(1, x)) }

func sinx(deg float64) float64    { return math.Sin(deg * degToRad) }
func cosx(deg float64) float64    { return math.Cos(deg * degToRad) }
func tanx(deg float64) float64    { return math.Tan(deg * degToRad) }
func asinx(x float64) float64     { return radToDeg * math.Asin(clamp(x)) }
func acosx(x float64) float64     { return radToDeg * math.Acos(clamp(x)) }
func atan2x(y, x float64) float64 { return radToDeg * math.Atan2(y, x) }

func sincosx(deg float64) (float64, float64) {
	return math.Sincos(deg * degToRad)
}

func mod360(x float64) float64 {
	x = math.Mod(x, 360)
	if x < 0 {
		x += 360
	}

	return x
}

func mod24(x float64) float64 {
	x = math.Mod(x, 24)
	if x < 0 {
		x += 24
	}

	return x
}
