package dusk

import (
	"time"
)

// solarParams holds intermediate solar position values computed from a date
// and longitude. Used by SunriseSunset and twilight to avoid repeating the
// 6-step parameter sequence.
type solarParams struct {
	delta    float64 // solar declination (degrees)
	jTransit float64 // Julian date of solar transit (noon)
}

// computeSolarParams returns the solar declination and transit JD for a given
// date and observer longitude.
func computeSolarParams(date time.Time, lon float64) solarParams {
	J := meanSolarTime(date, lon)
	M := solarMeanAnomaly(J)
	C := solarEquationOfCenter(M)
	lambda := solarEclipticLongitude(M, C)
	T := julianCentury(date)
	delta := solarDeclination(lambda, T)
	jTransit := solarTransitJD(J, M, lambda)

	return solarParams{delta: delta, jTransit: jTransit}
}

// SunriseSunset computes sunrise, solar noon, and sunset for the given date
// and observer position. The observer must be constructed via [NewObserver].
// The date is converted to the observer's timezone to determine the local
// calendar day; the time-of-day is ignored.
// Output times are converted to the observer's timezone.
//
// The algorithm follows the NOAA solar calculator method (derived from Meeus,
// Astronomical Algorithms).
func SunriseSunset(date time.Time, obs Observer) (SunEvent, error) {
	if err := validObserver(obs); err != nil {
		return SunEvent{}, err
	}

	localDate := date.In(obs.loc)

	date = time.Date(localDate.Year(), localDate.Month(), localDate.Day(), 0, 0, 0, 0, time.UTC)
	if err := validJulianDateRange(date); err != nil {
		return SunEvent{}, err
	}

	sp := computeSolarParams(date, obs.lon)

	omega, err := solarHourAngle(sp.delta, 0, obs.lat)
	if err != nil {
		return SunEvent{}, err
	}

	Jrise := sp.jTransit - omega/360.0
	Jset := sp.jTransit + omega/360.0

	rise := universalTimeFromJD(Jrise).In(obs.loc)
	noon := universalTimeFromJD(sp.jTransit).In(obs.loc)
	set := universalTimeFromJD(Jset).In(obs.loc)

	return SunEvent{
		Rise:     rise,
		Noon:     noon,
		Set:      set,
		Duration: set.Sub(rise),
	}, nil
}

// solarPosition returns the equatorial coordinates (RA, Dec) of the Sun for
// a given instant, using the Meeus mean anomaly + equation of center method.
//
// Unlike SunriseSunset (which rounds J to an integer for the NOAA method),
// this function uses continuous Julian days for precise position at any instant.
func solarPosition(t time.Time) equatorial {
	JD := julianDate(t)
	J := JD - j2000

	M := solarMeanAnomaly(J)
	C := solarEquationOfCenter(M)
	lambda := solarEclipticLongitude(M, C)

	// The Sun lies on the ecliptic (latitude = 0).
	return eclipticToEquatorial(t, lambda, 0)
}

// solarMeanAnomaly returns the Sun's mean anomaly in degrees.
// J is the number of days since J2000.0.
func solarMeanAnomaly(J float64) float64 {
	return mod360(357.5291092 + 0.98560028*J)
}

// solarMeanAnomalyFromCentury returns the Sun's mean anomaly in degrees.
// T is Julian centuries since J2000.0.
func solarMeanAnomalyFromCentury(T float64) float64 {
	return mod360(357.5291092 + 35999.0503*T)
}

// solarEquationOfCenter returns the equation of center in degrees for a given
// solar mean anomaly M (in degrees).
func solarEquationOfCenter(M float64) float64 {
	return 1.9148*sinx(M) + 0.0200*sinx(2*M) + 0.0003*sinx(3*M)
}

// solarEclipticLongitude returns the Sun's ecliptic longitude in degrees.
func solarEclipticLongitude(M, C float64) float64 {
	return mod360(M + C + 180 + 102.9372)
}

// solarDeclination returns the Sun's declination in degrees from its ecliptic
// longitude and Julian century T since J2000.0.
func solarDeclination(lambda, T float64) float64 {
	eps := meanObliquity(T)

	return asinx(sinx(lambda) * sinx(eps))
}

// solarHourAngle returns the hour angle in degrees for the Sun at the given
// declination, observer latitude, and depression angle (degrees below the
// geometric horizon, positive downward). For standard sunrise/sunset, pass
// depression = 0.
//
// For sunrise/sunset (depression=0), includes a -0.83 degree correction for
// atmospheric refraction and solar semidiameter. For twilight, uses the
// depression angle directly per IAU/USNO convention.
//
// Returns ErrCircumpolar when the Sun never sets (midnight sun) or
// ErrNeverRises when the Sun never rises (polar night) at this latitude
// and depression angle.
func solarHourAngle(delta, depression, lat float64) (float64, error) {
	var h0 float64
	if depression == 0 {
		h0 = -0.83
	} else {
		h0 = -depression
	}

	num := sinx(h0) - sinx(lat)*sinx(delta)
	den := cosx(lat) * cosx(delta)

	cosHA := num / den
	if cosHA < -1 {
		return 0, ErrCircumpolar
	}

	if cosHA > 1 {
		return 0, ErrNeverRises
	}

	return acosx(cosHA), nil
}

// solarTransitJD returns the Julian date of solar transit (solar noon).
func solarTransitJD(J, M, lambda float64) float64 {
	return j2000 + J + 0.0053*sinx(M) - 0.0069*sinx(2*lambda)
}

// ---------------------------------------------------------------------------
// Twilight (moved from twilight.go)
// ---------------------------------------------------------------------------

// CivilTwilight computes the evening civil twilight period (Sun 6 degrees below the
// horizon) for the given date and observer position. Dusk is tonight's civil
// dusk; Dawn is tomorrow morning's civil dawn.
func CivilTwilight(date time.Time, obs Observer) (TwilightEvent, error) {
	return twilight(date, obs, 6)
}

// NauticalTwilight computes the evening nautical twilight period (Sun 12
// degrees below the horizon) for the given date and observer position.
func NauticalTwilight(date time.Time, obs Observer) (TwilightEvent, error) {
	return twilight(date, obs, 12)
}

// AstronomicalTwilight computes the evening astronomical twilight period (Sun
// 18 degrees below the horizon) for the given date and observer position.
func AstronomicalTwilight(date time.Time, obs Observer) (TwilightEvent, error) {
	return twilight(date, obs, 18)
}

// twilight computes the twilight period for a given depression angle (positive
// degrees below the geometric horizon). Only the calendar date is used; the
// time-of-day is ignored. The returned Dusk is today's "set" at the depression
// angle (evening boundary) and Dawn is tomorrow's "rise" at the depression
// angle (morning boundary).
//
// Because the calculation spans two calendar days (tonight and tomorrow morning),
// an error is returned if twilight cannot be computed for either day. Near polar
// transition dates (latitudes ~65–70°), today's dusk may exist but tomorrow's
// dawn may not (or vice versa). In that case the entire call returns an error;
// callers needing partial results should compute each boundary separately using
// the appropriate depression angle and [SunriseSunset]-style hour-angle logic.
func twilight(date time.Time, obs Observer, depression float64) (TwilightEvent, error) {
	if err := validObserver(obs); err != nil {
		return TwilightEvent{}, err
	}

	localDate := date.In(obs.loc)

	date = time.Date(localDate.Year(), localDate.Month(), localDate.Day(), 0, 0, 0, 0, time.UTC)
	if err := validJulianDateRange(date); err != nil {
		return TwilightEvent{}, err
	}

	// Evening twilight: sunset at the given depression angle for today.
	sp := computeSolarParams(date, obs.lon)

	omega, err := solarHourAngle(sp.delta, depression, obs.lat)
	if err != nil {
		return TwilightEvent{}, err
	}

	dusk := universalTimeFromJD(sp.jTransit + omega/360).In(obs.loc)

	// Tomorrow's "rise" at this depression = twilight dawn.
	tomorrow := date.AddDate(0, 0, 1)
	if err := validJulianDateRange(tomorrow); err != nil {
		return TwilightEvent{}, err
	}

	sp2 := computeSolarParams(tomorrow, obs.lon)

	omega2, err2 := solarHourAngle(sp2.delta, depression, obs.lat)
	if err2 != nil {
		return TwilightEvent{}, err2
	}

	dawn := universalTimeFromJD(sp2.jTransit - omega2/360).In(obs.loc)

	return TwilightEvent{
		Dusk:          dusk,
		Dawn:          dawn,
		NightDuration: dawn.Sub(dusk),
	}, nil
}
