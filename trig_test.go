package dusk

import (
	"math"
	"testing"
)

const (
	epsTrig = 1e-15
	epsMod  = 1e-10
)

func approxEqual(a, b, eps float64) bool {
	return math.Abs(a-b) < eps
}

func TestSinx(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		deg  float64
		want float64
	}{
		{"0°", 0, 0},
		{"90°", 90, 1},
		{"180°", 180, 0},
		{"270°", 270, -1},
		{"-90°", -90, -1},
		{"45°", 45, math.Sqrt(2) / 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := sinx(tc.deg)
			if !approxEqual(got, tc.want, epsTrig) {
				t.Errorf("sinx(%v) = %v, want %v", tc.deg, got, tc.want)
			}
		})
	}
}

func TestCosx(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		deg  float64
		want float64
	}{
		{"0°", 0, 1},
		{"90°", 90, 0},
		{"180°", 180, -1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := cosx(tc.deg)
			if !approxEqual(got, tc.want, epsTrig) {
				t.Errorf("cosx(%v) = %v, want %v", tc.deg, got, tc.want)
			}
		})
	}
}

func TestTanx(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		deg  float64
		want float64
	}{
		{"0°", 0, 0},
		{"45°", 45, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := tanx(tc.deg)
			if !approxEqual(got, tc.want, epsTrig) {
				t.Errorf("tanx(%v) = %v, want %v", tc.deg, got, tc.want)
			}
		})
	}
}

func TestAsinx(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		x    float64
		want float64
	}{
		{"0", 0, 0},
		{"1", 1, 90},
		{"-1", -1, -90},
		{"clamped above", math.Nextafter(1, 2), 90},
		{"clamped below", math.Nextafter(-1, -2), -90},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := asinx(tc.x)
			if math.IsNaN(got) {
				t.Fatalf("asinx(%v) = NaN, want %v", tc.x, tc.want)
			}

			if !approxEqual(got, tc.want, epsTrig) {
				t.Errorf("asinx(%v) = %v, want %v", tc.x, got, tc.want)
			}
		})
	}
}

func TestAcosx(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		x    float64
		want float64
	}{
		{"0", 0, 90},
		{"1", 1, 0},
		{"-1", -1, 180},
		{"clamped above", math.Nextafter(1, 2), 0},
		{"clamped below", math.Nextafter(-1, -2), 180},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := acosx(tc.x)
			if math.IsNaN(got) {
				t.Fatalf("acosx(%v) = NaN, want %v", tc.x, tc.want)
			}

			if !approxEqual(got, tc.want, epsTrig) {
				t.Errorf("acosx(%v) = %v, want %v", tc.x, got, tc.want)
			}
		})
	}
}

func TestAtan2x(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		y, x float64
		want float64
	}{
		{"1,1", 1, 1, 45},
		{"0,1", 0, 1, 0},
		{"1,0", 1, 0, 90},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := atan2x(tc.y, tc.x)
			if !approxEqual(got, tc.want, epsTrig) {
				t.Errorf("atan2x(%v, %v) = %v, want %v", tc.y, tc.x, got, tc.want)
			}
		})
	}
}

func TestSincosx(t *testing.T) {
	t.Parallel()

	s, c := sincosx(30)
	wantSin := 0.5
	wantCos := math.Sqrt(3) / 2

	if !approxEqual(s, wantSin, epsTrig) {
		t.Errorf("sincosx(30) sin = %v, want %v", s, wantSin)
	}

	if !approxEqual(c, wantCos, epsTrig) {
		t.Errorf("sincosx(30) cos = %v, want %v", c, wantCos)
	}
}

func TestMod360(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		x    float64
		want float64
	}{
		{"positive overflow", 370, 10},
		{"negative wrap", -10, 350},
		{"zero", 0, 0},
		{"exact period", 360, 0},
		{"large negative", -730, 350},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := mod360(tc.x)
			if !approxEqual(got, tc.want, epsMod) {
				t.Errorf("mod360(%v) = %v, want %v", tc.x, got, tc.want)
			}
		})
	}
}

func TestMod24(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		x    float64
		want float64
	}{
		{"positive overflow", 25, 1},
		{"negative wrap", -1, 23},
		{"zero", 0, 0},
		{"exact period", 24, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := mod24(tc.x)
			if !approxEqual(got, tc.want, epsMod) {
				t.Errorf("mod24(%v) = %v, want %v", tc.x, got, tc.want)
			}
		})
	}
}
