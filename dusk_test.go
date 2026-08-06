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
	var zero Observer

	date := time.Date(2024, 3, 20, 0, 0, 0, 0, time.UTC)

	if _, err := SunriseSunset(date, zero); err == nil {
		t.Error("SunriseSunset: expected error for zero Observer")
	}

	if _, err := CivilTwilight(date, zero); err == nil {
		t.Error("CivilTwilight: expected error for zero Observer")
	}

	if _, err := MoonriseMoonset(date, zero); err == nil {
		t.Error("MoonriseMoonset: expected error for zero Observer")
	}
}

func TestSunEventString(t *testing.T) {
	loc := time.FixedZone("TEST", -5*3600)
	s := SunEvent{
		Rise:     time.Date(2024, 1, 15, 12, 1, 0, 0, time.UTC).In(loc),
		Noon:     time.Date(2024, 1, 15, 17, 30, 0, 0, time.UTC).In(loc),
		Set:      time.Date(2024, 1, 15, 22, 59, 0, 0, time.UTC).In(loc),
		Duration: 10*time.Hour + 58*time.Minute,
	}

	want := "Rise=07:01 Noon=12:30 Set=17:59 Duration=10h58m0s"
	if got := s.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSunEventString_Zero(t *testing.T) {
	s := SunEvent{}

	want := "Rise=--:-- Noon=--:-- Set=--:-- Duration=0s"
	if got := s.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMoonEventString(t *testing.T) {
	loc := time.FixedZone("TEST", 0)
	m := MoonEvent{
		Rise: time.Date(2024, 1, 15, 8, 15, 0, 0, loc),
		Set:  time.Date(2024, 1, 15, 20, 30, 0, 0, loc),
	}

	want := "Rise=08:15 Set=20:30 AboveHorizon=false"
	if got := m.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMoonEventString_AboveHorizon(t *testing.T) {
	loc := time.FixedZone("TEST", 0)
	m := MoonEvent{
		Rise:         time.Date(2024, 1, 15, 8, 15, 0, 0, loc),
		Set:          time.Date(2024, 1, 15, 20, 30, 0, 0, loc),
		AboveHorizon: true,
	}

	want := "Rise=08:15 Set=20:30 AboveHorizon=true"
	if got := m.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMoonEventString_Zero(t *testing.T) {
	m := MoonEvent{}

	want := "Rise=--:-- Set=--:-- AboveHorizon=false"
	if got := m.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLunarPhaseInfoString(t *testing.T) {
	l := LunarPhaseInfo{
		Illumination: 75.3,
		DaysApprox:   11.2,
		Name:         "Waxing Gibbous",
	}

	want := "Waxing Gibbous 75.3% (day 11.2)"
	if got := l.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLunarPhaseInfoString_Zero(t *testing.T) {
	l := LunarPhaseInfo{}

	want := "0.0% (day 0.0)"
	if got := l.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTwilightEventString(t *testing.T) {
	loc := time.FixedZone("TEST", 0)
	tw := TwilightEvent{
		Dusk:          time.Date(2024, 1, 15, 18, 30, 0, 0, loc),
		Dawn:          time.Date(2024, 1, 16, 6, 15, 0, 0, loc),
		NightDuration: 11*time.Hour + 45*time.Minute,
	}

	want := "Dusk=18:30 Dawn=06:15 NightDuration=11h45m0s"
	if got := tw.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTwilightEventString_Zero(t *testing.T) {
	tw := TwilightEvent{}

	want := "Dusk=--:-- Dawn=--:-- NightDuration=0s"
	if got := tw.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
