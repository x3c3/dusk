package dusk

import (
	"time"
)

const lunarMonthDays = 29.53059

// lunarHorizonDepression accounts for atmospheric refraction (~0.566°) and
// the Moon's mean semidiameter (~0.25°) when detecting moonrise/moonset.
const lunarHorizonDepression = 0.833

// ---------------------------------------------------------------------------
// Exported functions
// ---------------------------------------------------------------------------

// lunarEclipticPosition returns the geocentric ecliptic longitude, latitude
// (degrees), and distance (km) of the Moon for the given instant.
//
// The algorithm is from Meeus, Astronomical Algorithms, Chapter 47.
func lunarEclipticPosition(t time.Time) ecliptic {
	T := julianCentury(t)

	D := lunarMeanElongation(T)
	Lp := lunarMeanLongitude(T)
	M := solarMeanAnomalyFromCentury(T)
	Mp := lunarMeanAnomaly(T)
	F := lunarArgumentOfLatitude(T)

	A1 := mod360(119.75 + 131.849*T)
	A2 := mod360(53.09 + 479264.29*T)
	A3 := mod360(313.45 + 481266.484*T)

	E := 1 - 0.002516*T - 0.0000074*T*T
	E2 := E * E

	// Additive corrections in units of 0.000001 degrees (Meeus p. 338).
	Sl := 3958*sinx(A1) + 1962*sinx(Lp-F) + 318*sinx(A2)
	Sr := 0.0
	Sb := -2235*sinx(Lp) + 382*sinx(A3) + 175*sinx(A1-F) + 175*sinx(A1+F) + 127*sinx(Lp-Mp) - 115*sinx(Lp+Mp)

	for i := range tableLongDist {
		r := &tableLongDist[i]
		arg := D*r.D + M*r.M + Mp*r.Mʹ + F*r.F
		sa, ca := sincosx(arg)

		switch r.M {
		case 0:
			Sl += r.Σl * sa
			Sr += r.Σr * ca
		case 1, -1:
			Sl += r.Σl * sa * E
			Sr += r.Σr * ca * E
		case 2, -2:
			Sl += r.Σl * sa * E2
			Sr += r.Σr * ca * E2
		}
	}

	for i := range tableLat {
		r := &tableLat[i]
		arg := D*r.D + M*r.M + Mp*r.Mʹ + F*r.F
		sb := sinx(arg)

		switch r.M {
		case 0:
			Sb += r.Σb * sb
		case 1, -1:
			Sb += r.Σb * sb * E
		case 2, -2:
			Sb += r.Σb * sb * E2
		}
	}

	return ecliptic{
		lon:  mod360(Lp + Sl/1e6),
		lat:  Sb / 1e6,
		dist: 385000.56 + Sr/1000,
	}
}

// LunarPhase returns the lunar phase at the given instant.
//
// Unlike SunriseSunset and MoonriseMoonset which use only the calendar date,
// LunarPhase uses the exact time — the phase changes continuously. The result
// depends on the UTC instant; timezone does not affect the calculation.
//
// The phase angle uses the Meeus approach: solar ecliptic longitude from the
// mean-anomaly method, lunar ecliptic position from Chapter 47 tables.
//
// An error is returned if the date is out of the valid Julian date range.
func LunarPhase(date time.Time) (LunarPhaseInfo, error) {
	err := validJulianDateRange(date)
	if err != nil {
		return LunarPhaseInfo{}, err
	}

	ec := lunarEclipticPosition(date)

	J := julianDate(date) - j2000
	Msol := solarMeanAnomaly(J)
	C := solarEquationOfCenter(Msol)
	sunLon := solarEclipticLongitude(Msol, C)

	T := julianCentury(date)
	Mp := lunarMeanAnomaly(T)

	// elongation (0-360°, waxing = 0-180, waning = 180-360)
	d := acosx(cosx(ec.lon-sunLon) * cosx(ec.lat))
	if mod360(ec.lon-sunLon) > 180 {
		d = 360 - d
	}

	// phase angle (Meeus p. 346)
	PA := 180 - d - 0.1468*((1-0.0549*sinx(Mp))/(1-0.0167*sinx(Msol)))*sinx(d)

	K := 100 * (1 + cosx(PA)) / 2

	days := d / 360 * lunarMonthDays

	return LunarPhaseInfo{
		Illumination: K,
		Elongation:   d,
		Angle:        PA,
		DaysApprox:   days,
		Waxing:       d < 180,
		Name:         lunarPhaseName(d),
	}, nil
}

// lunarPosition returns the equatorial coordinates (RA, Dec) of the Moon for
// a given instant, using the Meeus ecliptic position converted to equatorial.
func lunarPosition(t time.Time) equatorial {
	ec := lunarEclipticPosition(t)

	return eclipticToEquatorial(t, ec.lon, ec.lat)
}

// MoonriseMoonset computes the moonrise and moonset times for the given date
// at the specified observer position and timezone.
// The date is converted to the observer's timezone to determine the local
// calendar day, then the function scans that local day (midnight to midnight)
// for rise/set events. This means the same time.Time can produce different
// results for observers in different timezones.
//
// The algorithm scans minute-by-minute through the day to detect altitude
// sign changes. This is slow by design (~1440 ecliptic-position evaluations).
// A single call takes approximately 1-2 ms on modern hardware (Apple M-series
// or equivalent; see BenchmarkMoonriseMoonset). Callers computing
// moonrise/moonset for many dates (e.g., a 30-day calendar ≈ 30-60 ms)
// should expect proportional cost and may benefit from caching or
// parallelization.
//
// The one-minute resolution means events shorter than one minute may not be
// detected. At polar or near-polar latitudes, the Moon can graze the horizon
// briefly enough to fall within a single scan step.
//
// An error is returned if the date is out of the valid Julian date range.
func MoonriseMoonset(date time.Time, obs Observer) (MoonEvent, error) {
	err := validObserver(obs)
	if err != nil {
		return MoonEvent{}, err
	}

	err = validJulianDateRange(date)
	if err != nil {
		return MoonEvent{}, err
	}

	localDate := date.In(obs.loc)
	// Construct in local time then convert to UTC so DST is handled:
	// spring-forward days are 23h, fall-back days are 25h.
	d := time.Date(localDate.Year(), localDate.Month(), localDate.Day(), 0, 0, 0, 0, obs.loc).UTC()
	nextMidnight := time.Date(localDate.Year(), localDate.Month(), localDate.Day()+1, 0, 0, 0, 0, obs.loc).UTC()

	err = validJulianDateRange(d)
	if err != nil {
		return MoonEvent{}, err
	}

	err = validJulianDateRange(nextMidnight)
	if err != nil {
		return MoonEvent{}, err
	}

	scanMinutes := int(nextMidnight.Sub(d).Minutes())

	var rise, set time.Time

	prevAlt := equatorialToHorizontal(d, obs, lunarPosition(d)).alt
	aboveAtStart := prevAlt > -lunarHorizonDepression

	for i := 1; i <= scanMinutes; i++ {
		cur := d.Add(time.Duration(i) * time.Minute)

		hz := equatorialToHorizontal(cur, obs, lunarPosition(cur))

		if rise.IsZero() && hz.alt > -lunarHorizonDepression && prevAlt <= -lunarHorizonDepression {
			rise = cur.In(obs.loc)
		}

		if set.IsZero() && hz.alt < -lunarHorizonDepression && prevAlt >= -lunarHorizonDepression {
			set = cur.In(obs.loc)
		}

		if !rise.IsZero() && !set.IsZero() {
			break
		}

		prevAlt = hz.alt
	}

	return MoonEvent{
		Rise:         rise,
		Set:          set,
		AboveHorizon: aboveAtStart,
	}, nil
}

// ---------------------------------------------------------------------------
// Unexported helpers (Meeus Chapter 47)
// ---------------------------------------------------------------------------

// lunarMeanElongation returns the Moon's mean elongation in degrees.
//
// T is Julian centuries since J2000.0.
// See Meeus eq. 47.2 p. 338.
func lunarMeanElongation(T float64) float64 {
	return mod360(297.8501921 + 445267.1114034*T - 0.0018819*T*T + T*T*T/545868 - T*T*T*T/113065000)
}

// lunarMeanAnomaly returns the Moon's mean anomaly in degrees.
//
// T is Julian centuries since J2000.0.
// See Meeus eq. 47.4 p. 338.
func lunarMeanAnomaly(T float64) float64 {
	return mod360(134.9633964 + 477198.8675055*T + 0.0087414*T*T + T*T*T/69699 - T*T*T*T/14712000)
}

// lunarArgumentOfLatitude returns the Moon's argument of latitude in degrees.
//
// T is Julian centuries since J2000.0.
// See Meeus eq. 47.5 p. 338.
func lunarArgumentOfLatitude(T float64) float64 {
	return mod360(93.272095 + 483202.0175233*T - 0.0036539*T*T + T*T*T/3526000 - T*T*T*T/863310000)
}

// lunarPhaseName returns the common name for the lunar phase based on the
// elongation angle in degrees.
func lunarPhaseName(age float64) string {
	age = mod360(age)

	switch {
	case age < 22.5 || age >= 337.5:
		return "New Moon"
	case age < 67.5:
		return "Waxing Crescent"
	case age < 112.5:
		return "First Quarter"
	case age < 157.5:
		return "Waxing Gibbous"
	case age < 202.5:
		return "Full Moon"
	case age < 247.5:
		return "Waning Gibbous"
	case age < 292.5:
		return "Last Quarter"
	default:
		return "Waning Crescent"
	}
}

// ---------------------------------------------------------------------------
// Meeus Table 47.A/B coefficients (moved from lunar_tables.go)
// ---------------------------------------------------------------------------

// Meeus Table 47.A — Periodic terms for the longitude (Σl) and distance (Σr)
// of the Moon.
//
// See Meeus, Astronomical Algorithms, p. 339.
type lunarLongDistCoeff struct{ D, M, Mʹ, F, Σl, Σr float64 }

// Meeus Table 47.B — Periodic terms for the latitude (Σb) of the Moon.
//
// See Meeus, Astronomical Algorithms, p. 341.
type lunarLatCoeff struct{ D, M, Mʹ, F, Σb float64 }

var tableLongDist = [...]lunarLongDistCoeff{
	{0, 0, 1, 0, 6288774, -20905355},
	{2, 0, -1, 0, 1274027, -3699111},
	{2, 0, 0, 0, 658314, -2955968},
	{0, 0, 2, 0, 213618, -569925},

	{0, 1, 0, 0, -185116, 48888},
	{0, 0, 0, 2, -114332, -3149},
	{2, 0, -2, 0, 58793, 246158},
	{2, -1, -1, 0, 57066, -152138},

	{2, 0, 1, 0, 53322, -170733},
	{2, -1, 0, 0, 45758, -204586},
	{0, 1, -1, 0, -40923, -129620},
	{1, 0, 0, 0, -34720, 108743},

	{0, 1, 1, 0, -30383, 104755},
	{2, 0, 0, -2, 15327, 10321},
	{0, 0, 1, 2, -12528, 0},
	{0, 0, 1, -2, 10980, 79661},

	{4, 0, -1, 0, 10675, -34782},
	{0, 0, 3, 0, 10034, -23210},
	{4, 0, -2, 0, 8548, -21636},
	{2, 1, -1, 0, -7888, 24208},

	{2, 1, 0, 0, -6766, 30824},
	{1, 0, -1, 0, -5163, -8379},
	{1, 1, 0, 0, 4987, -16675},
	{2, -1, 1, 0, 4036, -12831},

	{2, 0, 2, 0, 3994, -10445},
	{4, 0, 0, 0, 3861, -11650},
	{2, 0, -3, 0, 3665, 14403},
	{0, 1, -2, 0, -2689, -7003},

	{2, 0, -1, 2, -2602, 0},
	{2, -1, -2, 0, 2390, 10056},
	{1, 0, 1, 0, -2348, 6322},
	{2, -2, 0, 0, 2236, -9884},

	{0, 1, 2, 0, -2120, 5751},
	{0, 2, 0, 0, -2069, 0},
	{2, -2, -1, 0, 2048, -4950},
	{2, 0, 1, -2, -1773, 4130},

	{2, 0, 0, 2, -1595, 0},
	{4, -1, -1, 0, 1215, -3958},
	{0, 0, 2, 2, -1110, 0},
	{3, 0, -1, 0, -892, 3258},

	{2, 1, 1, 0, -810, 2616},
	{4, -1, -2, 0, 759, -1897},
	{0, 2, -1, 0, -713, -2117},
	{2, 2, -1, 0, -700, 2354},

	{2, 1, -2, 0, 691, 0},
	{2, -1, 0, -2, 596, 0},
	{4, 0, 1, 0, 549, -1423},
	{0, 0, 4, 0, 537, -1117},

	{4, -1, 0, 0, 520, -1571},
	{1, 0, -2, 0, -487, -1739},
	{2, 1, 0, -2, -399, 0},
	{0, 0, 2, -2, -381, -4421},

	{1, 1, 1, 0, 351, 0},
	{3, 0, -2, 0, -340, 0},
	{4, 0, -3, 0, 330, 0},
	{2, -1, 2, 0, 327, 0},

	{0, 2, 1, 0, -323, 1165},
	{1, 1, -1, 0, 299, 0},
	{2, 0, 3, 0, 294, 0},
	{2, 0, -1, -2, 0, 8752},
}

var tableLat = [...]lunarLatCoeff{
	{0, 0, 0, 1, 5128122},
	{0, 0, 1, 1, 280602},
	{0, 0, 1, -1, 277693},
	{2, 0, 0, -1, 173237},

	{2, 0, -1, 1, 55413},
	{2, 0, -1, -1, 46271},
	{2, 0, 0, 1, 32573},
	{0, 0, 2, 1, 17198},

	{2, 0, 1, -1, 9266},
	{0, 0, 2, -1, 8822},
	{2, -1, 0, -1, 8216},
	{2, 0, -2, -1, 4324},

	{2, 0, 1, 1, 4200},
	{2, 1, 0, -1, -3359},
	{2, -1, -1, 1, 2463},
	{2, -1, 0, 1, 2211},

	{2, -1, -1, -1, 2065},
	{0, 1, -1, -1, -1870},
	{4, 0, -1, -1, 1828},
	{0, 1, 0, 1, -1794},

	{0, 0, 0, 3, -1749},
	{0, 1, -1, 1, -1565},
	{1, 0, 0, 1, -1491},
	{0, 1, 1, 1, -1475},

	{0, 1, 1, -1, -1410},
	{0, 1, 0, -1, -1344},
	{1, 0, 0, -1, -1335},
	{0, 0, 3, 1, 1107},

	{4, 0, 0, -1, 1021},
	{4, 0, -1, 1, 833},

	{0, 0, 1, -3, 777},
	{4, 0, -2, 1, 671},
	{2, 0, 0, -3, 607},
	{2, 0, 2, -1, 596},

	{2, -1, 1, -1, 491},
	{2, 0, -2, 1, -451},
	{0, 0, 3, -1, 439},
	{2, 0, 2, 1, 422},

	{2, 0, -3, -1, 421},
	{2, 1, -1, 1, -366},
	{2, 1, 0, 1, -351},
	{4, 0, 0, 1, 331},

	{2, -1, 1, 1, 315},
	{2, -2, 0, -1, 302},
	{0, 0, 1, 3, -283},
	{2, 1, 1, -1, -229},

	{1, 1, 0, -1, 223},
	{1, 1, 0, 1, 223},
	{0, 1, -2, -1, -220},
	{2, 1, -1, -1, -220},

	{1, 0, 1, 1, -185},
	{2, -1, -2, -1, 181},
	{0, 1, 2, 1, -177},
	{4, 0, -2, -1, 176},

	{4, -1, -1, -1, 166},
	{1, 0, 1, -1, -164},
	{4, 0, 1, -1, 132},
	{1, 0, -1, -1, -119},

	{4, -1, 0, -1, 115},
	{2, -2, 0, 1, 107},
}
