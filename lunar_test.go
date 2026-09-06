package dusk

import (
	"math"
	"testing"
	"time"
)

func TestLunarEclipticPosition(t *testing.T) {
	t.Parallel()

	// Meeus p. 342: 1992-04-12 00:00 UTC
	dt := time.Date(1992, 4, 12, 0, 0, 0, 0, time.UTC)
	ec := lunarEclipticPosition(dt)

	tests := []struct {
		name    string
		got     float64
		want    float64
		epsilon float64
	}{
		{"longitude", ec.lon, 133.162655, 0.001},
		{"latitude", ec.lat, -3.229126, 0.001},
		{"distance", ec.dist, 368409.7, 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if math.Abs(tt.got-tt.want) > tt.epsilon {
				t.Errorf("%s = %f, want %f (±%f)", tt.name, tt.got, tt.want, tt.epsilon)
			}
		})
	}
}

func TestLunarPosition(t *testing.T) {
	t.Parallel()

	// Meeus p. 342: 1992-04-12 00:00 UTC.
	// Expected equatorial coordinates (nutation-corrected): RA ~134.7°, Dec ~13.8°.
	dt := time.Date(1992, 4, 12, 0, 0, 0, 0, time.UTC)
	eq := lunarPosition(dt)

	if math.Abs(eq.ra-134.7) > 0.5 {
		t.Errorf("RA = %.4f, want ~134.7°", eq.ra)
	}

	if math.Abs(eq.dec-13.8) > 0.5 {
		t.Errorf("Dec = %.4f, want ~13.8°", eq.dec)
	}
}

func TestLunarPhase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		date       time.Time
		wantLow    float64 // illumination lower bound
		wantHigh   float64 // illumination upper bound
		wantName   string  // expected phase name (empty to skip)
		wantWaxing bool
		elongLow   float64 // elongation lower bound (degrees)
		elongHigh  float64 // elongation upper bound (degrees)
	}{
		{
			name:       "near new moon 2024-01-11",
			date:       time.Date(2024, 1, 11, 12, 0, 0, 0, time.UTC),
			wantLow:    0,
			wantHigh:   5,
			wantName:   "New Moon",
			wantWaxing: true,
			elongLow:   0,
			elongHigh:  15,
		},
		{
			name:       "near first quarter 2024-01-18",
			date:       time.Date(2024, 1, 18, 3, 0, 0, 0, time.UTC),
			wantLow:    40,
			wantHigh:   60,
			wantName:   "First Quarter",
			wantWaxing: true,
			elongLow:   80,
			elongHigh:  100,
		},
		{
			name:       "near full moon 2024-01-25",
			date:       time.Date(2024, 1, 25, 18, 0, 0, 0, time.UTC),
			wantLow:    95,
			wantHigh:   100,
			wantName:   "Full Moon",
			wantWaxing: false, // exact full moon was 17:54 UTC; by 18:00 elongation > 180°
			elongLow:   175,
			elongHigh:  195,
		},
		{
			name:       "waxing crescent 2024-01-14",
			date:       time.Date(2024, 1, 14, 12, 0, 0, 0, time.UTC),
			wantLow:    5,
			wantHigh:   25,
			wantName:   "Waxing Crescent",
			wantWaxing: true,
			elongLow:   30,
			elongHigh:  55,
		},
		{
			name:       "waxing gibbous 2024-01-21",
			date:       time.Date(2024, 1, 21, 12, 0, 0, 0, time.UTC),
			wantLow:    70,
			wantHigh:   90,
			wantName:   "Waxing Gibbous",
			wantWaxing: true,
			elongLow:   120,
			elongHigh:  145,
		},
		{
			name:       "waning gibbous 2024-01-28",
			date:       time.Date(2024, 1, 28, 12, 0, 0, 0, time.UTC),
			wantLow:    70,
			wantHigh:   95,
			wantName:   "Waning Gibbous",
			wantWaxing: false,
			elongLow:   200,
			elongHigh:  225,
		},
		{
			name:       "near last quarter 2024-02-02",
			date:       time.Date(2024, 2, 2, 23, 0, 0, 0, time.UTC),
			wantLow:    40,
			wantHigh:   60,
			wantName:   "Last Quarter",
			wantWaxing: false,
			elongLow:   260,
			elongHigh:  280,
		},
		{
			name:       "waning crescent 2024-02-06",
			date:       time.Date(2024, 2, 6, 12, 0, 0, 0, time.UTC),
			wantLow:    5,
			wantHigh:   30,
			wantName:   "Waning Crescent",
			wantWaxing: false,
			elongLow:   300,
			elongHigh:  325,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p, err := LunarPhase(tt.date)
			if err != nil {
				t.Fatalf("LunarPhase() returned error: %v", err)
			}

			if p.Illumination < tt.wantLow || p.Illumination > tt.wantHigh {
				t.Errorf("illumination = %.2f%%, want [%.0f, %.0f]",
					p.Illumination, tt.wantLow, tt.wantHigh)
			}

			if tt.wantName != "" && p.Name != tt.wantName {
				t.Errorf("name = %q, want %q", p.Name, tt.wantName)
			}

			if p.Waxing != tt.wantWaxing {
				t.Errorf("Waxing = %v, want %v", p.Waxing, tt.wantWaxing)
			}

			if p.Elongation < tt.elongLow || p.Elongation > tt.elongHigh {
				t.Errorf("Elongation = %.1f°, want [%.0f, %.0f]",
					p.Elongation, tt.elongLow, tt.elongHigh)
			}
		})
	}
}

func TestMoonriseMoonset_AboveHorizon(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		lat, lon     float64
		loc          string
		date         time.Time
		wantAbove    bool
		wantRiseZero bool
		wantSetZero  bool
	}{
		{
			name:         "NYC normal day — Moon rises and sets",
			lat:          40.7128,
			lon:          -74.006,
			loc:          "America/New_York",
			date:         time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			wantAbove:    false,
			wantRiseZero: false,
			wantSetZero:  false,
		},
		{
			name:         "high arctic winter — Moon above horizon all day",
			lat:          78,
			lon:          16,
			loc:          "UTC",
			date:         time.Date(2024, 12, 18, 0, 0, 0, 0, time.UTC),
			wantAbove:    true,
			wantRiseZero: true,
			wantSetZero:  true,
		},
		{
			name:         "high arctic summer — Moon below horizon all day",
			lat:          78,
			lon:          16,
			loc:          "UTC",
			date:         time.Date(2024, 6, 22, 0, 0, 0, 0, time.UTC),
			wantAbove:    false,
			wantRiseZero: true,
			wantSetZero:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var loc *time.Location
			if tt.loc == "UTC" {
				loc = time.UTC
			} else {
				var err error

				loc, err = time.LoadLocation(tt.loc)
				if err != nil {
					t.Fatal(err)
				}
			}

			obs := mustObserver(t, tt.lat, tt.lon, loc)

			evt, err := MoonriseMoonset(tt.date, obs)
			if err != nil {
				t.Fatal(err)
			}

			if evt.AboveHorizon != tt.wantAbove {
				t.Errorf("AboveHorizon = %v, want %v", evt.AboveHorizon, tt.wantAbove)
			}

			if evt.Rise.IsZero() != tt.wantRiseZero {
				t.Errorf("Rise.IsZero() = %v, want %v", evt.Rise.IsZero(), tt.wantRiseZero)
			}

			if evt.Set.IsZero() != tt.wantSetZero {
				t.Errorf("Set.IsZero() = %v, want %v", evt.Set.IsZero(), tt.wantSetZero)
			}
		})
	}
}

func TestMoonriseMoonset(t *testing.T) {
	t.Parallel()

	// NYC 2024-01-15
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}

	date := time.Date(2024, 1, 15, 0, 0, 0, 0, loc)
	obs := mustObserver(t, 40.7128, -74.0060, loc)

	evt, err := MoonriseMoonset(date, obs)
	if err != nil {
		t.Fatal(err)
	}

	if evt.Rise.IsZero() {
		t.Error("expected non-zero rise time")
	}

	if evt.Set.IsZero() {
		t.Error("expected non-zero set time")
	}
	// Regression reference: algorithm-computed values for NYC 2024-01-15.
	// Note: simplified Meeus approach can differ from USNO by up to ~1.5h for the Moon.
	tolerance := 5 * time.Minute
	wantRise := time.Date(2024, 1, 15, 10, 6, 0, 0, loc)
	wantSet := time.Date(2024, 1, 15, 22, 13, 0, 0, loc)

	if diff := evt.Rise.Sub(wantRise); diff < -tolerance || diff > tolerance {
		t.Errorf("Rise = %v, want %v (±%v, diff=%v)", evt.Rise.Format("15:04"), wantRise.Format("15:04"), tolerance, diff)
	}

	if diff := evt.Set.Sub(wantSet); diff < -tolerance || diff > tolerance {
		t.Errorf("Set = %v, want %v (±%v, diff=%v)", evt.Set.Format("15:04"), wantSet.Format("15:04"), tolerance, diff)
	}

	t.Logf("rise=%v set=%v", evt.Rise, evt.Set)
}

func TestMoonriseMoonset_SouthernHemisphere(t *testing.T) {
	t.Parallel()

	// Sydney, Australia on 2024-01-15.
	loc, err := time.LoadLocation("Australia/Sydney")
	if err != nil {
		t.Fatal(err)
	}

	date := time.Date(2024, 1, 15, 0, 0, 0, 0, loc)
	obs := mustObserver(t, -33.87, 151.21, loc)

	evt, err := MoonriseMoonset(date, obs)
	if err != nil {
		t.Fatal(err)
	}

	if evt.Rise.IsZero() {
		t.Error("expected non-zero rise time for Sydney")
	}

	if evt.Set.IsZero() {
		t.Error("expected non-zero set time for Sydney")
	}
	// Regression reference: algorithm-computed values for Sydney 2024-01-15.
	// These are library-derived, not USNO; used to detect regressions.
	tolerance := 20 * time.Minute
	wantRise := time.Date(2024, 1, 15, 9, 50, 0, 0, loc)
	wantSet := time.Date(2024, 1, 15, 23, 0, 0, 0, loc)

	if diff := evt.Rise.Sub(wantRise); diff < -tolerance || diff > tolerance {
		t.Errorf("Rise = %v, want %v (±%v, diff=%v)", evt.Rise.Format("15:04"), wantRise.Format("15:04"), tolerance, diff)
	}

	if diff := evt.Set.Sub(wantSet); diff < -tolerance || diff > tolerance {
		t.Errorf("Set = %v, want %v (±%v, diff=%v)", evt.Set.Format("15:04"), wantSet.Format("15:04"), tolerance, diff)
	}

	t.Logf("Sydney moonrise=%v moonset=%v", evt.Rise, evt.Set)
}
