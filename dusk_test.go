package dusk

import (
	"math"
	"testing"
	"time"
)

// mustObserver constructs an Observer, failing the test on invalid input.
func mustObserver(t *testing.T, lat, lon float64, loc *time.Location) Observer {
	t.Helper()

	obs, err := NewObserver(lat, lon, loc)
	if err != nil {
		t.Fatalf("mustObserver(%v, %v): %v", lat, lon, err)
	}

	return obs
}

func TestNewObserver(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		lat     float64
		lon     float64
		loc     *time.Location
		wantErr bool
	}{
		{"valid NYC", 40.7128, -74.006, time.UTC, false},
		{"valid south pole", -90, 0, time.UTC, false},
		{"valid date line", 0, 180, time.UTC, false},
		{"valid negative lon", 0, -180, time.UTC, false},
		{"nil location", 40.7128, -74.006, nil, true},
		{"lat too high", 91, 0, time.UTC, true},
		{"lat too low", -91, 0, time.UTC, true},
		{"lon too high", 0, 181, time.UTC, true},
		{"lon too low", 0, -181, time.UTC, true},
		{"NaN lat", math.NaN(), 0, time.UTC, true},
		{"NaN lon", 0, math.NaN(), time.UTC, true},
		{"Inf lat", math.Inf(1), 0, time.UTC, true},
		{"Inf lon", 0, math.Inf(-1), time.UTC, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			obs, err := NewObserver(tt.lat, tt.lon, tt.loc)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			_ = obs
		})
	}
}

func TestZeroObserverReturnsError(t *testing.T) {
	t.Parallel()

	var zero Observer

	date := time.Date(2024, 3, 20, 0, 0, 0, 0, time.UTC)

	_, err := SunriseSunset(date, zero)
	if err == nil {
		t.Error("SunriseSunset: expected error for zero Observer")
	}

	_, err = CivilTwilight(date, zero)
	if err == nil {
		t.Error("CivilTwilight: expected error for zero Observer")
	}

	_, err = MoonriseMoonset(date, zero)
	if err == nil {
		t.Error("MoonriseMoonset: expected error for zero Observer")
	}
}
