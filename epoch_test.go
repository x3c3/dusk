package dusk

import (
	"errors"
	"math"
	"testing"
	"time"
)

func TestJulianDate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		time    time.Time
		want    float64
		epsilon float64
	}{
		{
			name:    "Meeus p.62: 1992-04-12 00:00 UTC",
			time:    time.Date(1992, 4, 12, 0, 0, 0, 0, time.UTC),
			want:    2448724.5,
			epsilon: 0.001,
		},
		{
			name:    "J2000 epoch: 2000-01-01 12:00 UTC",
			time:    time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC),
			want:    2451545.0,
			epsilon: 0.001,
		},
		{
			name:    "Unix epoch: 1970-01-01 00:00 UTC",
			time:    time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC),
			want:    2440587.5,
			epsilon: 0.001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := julianDate(tt.time)
			if math.Abs(got-tt.want) > tt.epsilon {
				t.Errorf("julianDate() = %f, want %f (±%f)", got, tt.want, tt.epsilon)
			}
		})
	}
}

func TestValidJulianDateRange(t *testing.T) {
	t.Parallel()

	minValid := time.Unix(0, math.MinInt64).UTC()
	maxValid := time.Unix(0, math.MaxInt64).UTC()

	tests := []struct {
		name    string
		time    time.Time
		wantErr bool
	}{
		{"valid 2024", time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC), false},
		{"valid min boundary", minValid, false},
		{"valid max boundary", maxValid, false},
		{"too early 1600", time.Date(1600, 1, 1, 0, 0, 0, 0, time.UTC), true},
		{"too late 2300", time.Date(2300, 1, 1, 0, 0, 0, 0, time.UTC), true},
		{"just before min", minValid.Add(-time.Nanosecond), true},
		{"just after min", minValid.Add(time.Nanosecond), false},
		{"just before max", maxValid.Add(-time.Nanosecond), false},
		{"just after max", maxValid.Add(time.Nanosecond), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validJulianDateRange(tt.time)
			if tt.wantErr && !errors.Is(err, ErrDateOutOfRange) {
				t.Errorf("expected ErrDateOutOfRange, got %v", err)
			}

			if !tt.wantErr && err != nil {
				t.Errorf("expected nil, got %v", err)
			}
		})
	}
}

func TestGMST(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		time    time.Time
		want    float64
		epsilon float64
	}{
		{
			// Meeus p.88 example: 1987-04-10 00:00 UTC → θ₀ = 197.693195°
			name:    "Meeus p.88: 1987-04-10 00:00 UTC",
			time:    time.Date(1987, 4, 10, 0, 0, 0, 0, time.UTC),
			want:    197.693195,
			epsilon: 0.01,
		},
		{
			// Meeus p.88: 1987-04-10 19:21:00 UTC → θ = 128.7378734°
			name:    "Meeus p.88: 1987-04-10 19:21 UTC",
			time:    time.Date(1987, 4, 10, 19, 21, 0, 0, time.UTC),
			want:    128.7379,
			epsilon: 0.05,
		},
		{
			// J2000 epoch: 2000-01-01 12:00 UTC → θ₀ ≈ 280.46°
			name:    "J2000 epoch",
			time:    time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC),
			want:    280.46,
			epsilon: 0.1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := greenwichMeanSiderealTime(tt.time)
			if math.Abs(got-tt.want) > tt.epsilon {
				t.Errorf("greenwichMeanSiderealTime() = %f, want %f (±%f)", got, tt.want, tt.epsilon)
			}
		})
	}
}

func TestLocalSiderealTime(t *testing.T) {
	t.Parallel()

	// Meeus p.88: 1987-04-10 00:00 UTC → GMST = 197.693195° = 13.1795h.
	// At longitude 0°, LST = GMST.
	tests := []struct {
		name      string
		time      time.Time
		longitude float64
		want      float64
		epsilon   float64
	}{
		{
			name:      "Meeus p.88: Greenwich (lon=0)",
			time:      time.Date(1987, 4, 10, 0, 0, 0, 0, time.UTC),
			longitude: 0.0,
			want:      197.693195 / 15.0, // 13.1795h
			epsilon:   0.01,
		},
		{
			name:      "Meeus p.88: 15°E (lon=15)",
			time:      time.Date(1987, 4, 10, 0, 0, 0, 0, time.UTC),
			longitude: 15.0,
			want:      mod24(197.693195/15.0 + 1.0), // +1 hour
			epsilon:   0.01,
		},
		{
			name:      "Meeus p.88: 75°W (lon=-75)",
			time:      time.Date(1987, 4, 10, 0, 0, 0, 0, time.UTC),
			longitude: -75.0,
			want:      mod24(197.693195/15.0 - 5.0), // -5 hours
			epsilon:   0.01,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := localSiderealTime(tt.time, tt.longitude)
			if math.Abs(got-tt.want) > tt.epsilon {
				t.Errorf("localSiderealTime() = %f, want %f (±%f)", got, tt.want, tt.epsilon)
			}
		})
	}

	// Verify that longitude shifts LST by the expected amount.
	t.Run("longitude offset shifts LST", func(t *testing.T) {
		t.Parallel()

		dt := time.Date(1987, 4, 10, 0, 0, 0, 0, time.UTC)
		lst0 := localSiderealTime(dt, 0.0)
		lst15 := localSiderealTime(dt, 15.0)

		diff := lst15 - lst0
		if diff < 0 {
			diff += 24
		}

		if math.Abs(diff-1.0) > 0.001 {
			t.Errorf("15° longitude should shift LST by 1 hour, got shift = %f", diff)
		}
	})
}

func TestMeanObliquity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		T       float64
		want    float64
		epsilon float64
	}{
		{
			name:    "J2000 (T=0) obliquity ~23.4393°",
			T:       0.0,
			want:    23.4393,
			epsilon: 0.001,
		},
		{
			name:    "J1900 (T=-1) obliquity ~23.4523°",
			T:       -1.0,
			want:    23.4523,
			epsilon: 0.001,
		},
		{
			name:    "J2100 (T=1) obliquity ~23.4263°",
			T:       1.0,
			want:    23.4263,
			epsilon: 0.001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := meanObliquity(tt.T)
			if math.Abs(got-tt.want) > tt.epsilon {
				t.Errorf("meanObliquity() = %f, want %f (±%f)", got, tt.want, tt.epsilon)
			}
		})
	}
}

func TestJulianCentury(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		time    time.Time
		want    float64
		epsilon float64
	}{
		{
			name:    "J2000 epoch → T = 0.0",
			time:    time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC),
			want:    0.0,
			epsilon: 1e-6,
		},
		{
			// 2100-01-01 12:00 UTC is 36524 days after J2000 in Julian Date terms,
			// so T = 36524 / 36525 ≈ 0.9999726 (slightly less than 1 Julian century).
			name:    "J2100 → T ≈ 0.9999726",
			time:    time.Date(2100, 1, 1, 12, 0, 0, 0, time.UTC),
			want:    36524.0 / 36525.0,
			epsilon: 0.001,
		},
		{
			// 1900-01-01 12:00 UTC is 36524 days before J2000 in Julian Date terms,
			// so T = -36524 / 36525 ≈ -0.9999726 (slightly more than -1 Julian century).
			name:    "J1900 → T ≈ -0.9999726",
			time:    time.Date(1900, 1, 1, 12, 0, 0, 0, time.UTC),
			want:    -36524.0 / 36525.0,
			epsilon: 0.001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := julianCentury(tt.time)
			if math.Abs(got-tt.want) > tt.epsilon {
				t.Errorf("julianCentury() = %f, want %f (±%f)", got, tt.want, tt.epsilon)
			}
		})
	}
}

func TestEclipticToEquatorial(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		dt      time.Time
		lon     float64
		lat     float64
		wantRA  float64
		wantDec float64
		eps     float64
	}{
		{
			name:    "Meeus p.342: Moon 1992-04-12",
			dt:      time.Date(1992, 4, 12, 0, 0, 0, 0, time.UTC),
			lon:     133.162655,
			lat:     -3.229126,
			wantRA:  134.7,
			wantDec: 13.8,
			eps:     0.15,
		},
		{
			name:    "ecliptic lon=90 lat=0 (solstice geometry)",
			dt:      time.Date(2024, 6, 20, 12, 0, 0, 0, time.UTC),
			lon:     90.0,
			lat:     0.0,
			wantRA:  90.0,
			wantDec: 23.44,
			eps:     0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			eq := eclipticToEquatorial(tt.dt, tt.lon, tt.lat)
			if math.Abs(eq.ra-tt.wantRA) > tt.eps {
				t.Errorf("RA = %.4f, want ~%.1f (within %.2f°)", eq.ra, tt.wantRA, tt.eps)
			}

			if math.Abs(eq.dec-tt.wantDec) > tt.eps {
				t.Errorf("Dec = %.4f, want ~%.2f (within %.2f°)", eq.dec, tt.wantDec, tt.eps)
			}
		})
	}
}

func TestEquatorialToHorizontal(t *testing.T) {
	t.Parallel()

	// Sirius observed from NYC on 2024-01-16 02:00 UTC (~9pm EST).
	// Sirius transits ~00:40 local in mid-January; at 9pm it is well up in the SE.
	dt := time.Date(2024, 1, 16, 2, 0, 0, 0, time.UTC)
	obs := mustObserver(t, 40.7128, -74.006, time.UTC)
	sirius := equatorial{ra: 101.287, dec: -16.716}

	h := equatorialToHorizontal(dt, obs, sirius)

	// Algorithm-computed reference: Sirius from NYC at 2024-01-16 02:00 UTC.
	// Alt ~26°, Az ~147° (SE sky, still rising toward transit).
	const eps = 1.0
	if math.Abs(h.alt-26.06) > eps {
		t.Errorf("alt = %.4f, want ~26.06° (within %.0f°)", h.alt, eps)
	}

	if math.Abs(h.az-147.49) > eps {
		t.Errorf("az = %.4f, want ~147.49° (within %.0f°)", h.az, eps)
	}
}

func TestEquatorialToHorizontal_Pole(t *testing.T) {
	t.Parallel()

	// Observer at the North Pole — cosAltCosLat guard triggers, azimuth defaults to 0.
	dt := time.Date(2024, 1, 16, 12, 0, 0, 0, time.UTC)
	obs := mustObserver(t, 90.0, 0, time.UTC)
	star := equatorial{ra: 0, dec: 45}

	h := equatorialToHorizontal(dt, obs, star)
	if h.az != 0 {
		t.Errorf("az = %.4f, want 0 at pole (guard branch)", h.az)
	}
	// Altitude should equal declination at the pole.
	if math.Abs(h.alt-45.0) > 0.5 {
		t.Errorf("alt = %.4f, want ~45° at North Pole for Dec=45°", h.alt)
	}
}

func TestHourAngle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		ra     float64
		lst    float64
		wantHA float64
	}{
		{"object on meridian", 90, 6, 0},
		{"RA=90 LST=0h", 90, 0, 270},
		{"RA=0 LST=6h", 0, 6, 90},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ha := hourAngle(tt.ra, tt.lst)
			if math.Abs(ha-tt.wantHA) > 1e-9 {
				t.Errorf("hourAngle(%v, %v) = %v, want %v", tt.ra, tt.lst, ha, tt.wantHA)
			}
		})
	}
}

func TestNutationInLongitude(t *testing.T) {
	t.Parallel()

	// Meeus p. 148, Example 22.a: 1987-04-10 0h TT
	// Δψ ≈ -3.788" = -0.001052°
	dt := time.Date(1987, 4, 10, 0, 0, 0, 0, time.UTC)
	T := julianCentury(dt)
	L := solarMeanLongitude(T)
	l := lunarMeanLongitude(T)
	omega := lunarAscendingNode(T)

	dpsi := nutationInLongitude(L, l, omega)

	// Meeus gives Δψ = -3.788" ≈ -0.001052°. This simplified formula
	// (4-term) agrees to ~1" (~0.0003°).
	const want = -0.001052
	if math.Abs(dpsi-want) > 0.0004 {
		t.Errorf("nutationInLongitude = %.6f°, want ~%.6f° (within 0.0004°)", dpsi, want)
	}
}
