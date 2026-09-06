# Dusk Walkthrough

*2026-09-06T13:24:59Z by Showboat 0.6.1*
<!-- showboat-id: 25364f3c-47a8-4fa5-b022-09cf7d286d97 -->

## Overview

`dusk` is a Go library that answers questions about the sky at a given place
on a given day: when the sun rises and sets, when each band of twilight
begins and ends, when the moon rises and sets, and what phase it is in.

Three things shape the whole codebase:

- **Zero dependencies.** Nothing outside the standard library. The Meeus
  coefficient tables are transcribed into the source rather than fetched.
- **One package at the root**, with a reference CLI under `cmd/dusk`. There
  is no internal package structure to navigate — file boundaries are by
  domain, not by layer.
- **Degrees everywhere.** Meeus's *Astronomical Algorithms* is written in
  degrees, so the code is too, and a thin wrapper layer keeps `math`'s
  radians out of sight.

There are two entry surfaces. The library exports five calculation functions
and a constructor; `cmd/dusk` is the reference consumer that calls every one
of them.

```bash
wc -l *.go cmd/dusk/*.go | grep -v _test | grep -v '^ *[0-9]* total'
```

```output
     211 dusk.go
     228 epoch.go
     441 lunar.go
     244 solar.go
      44 trig.go
     215 cmd/dusk/main.go
     327 cmd/dusk/render.go
     314 cmd/dusk/report.go
```

## Architecture

Files divide by **domain**, not by layer, and depend on each other in one
direction only:

| File | Holds | Depends on |
| --- | --- | --- |
| `trig.go` | Degree-based trig wrappers, `clamp`, angle normalization | `math` |
| `epoch.go` | Julian dates, sidereal time, nutation, coordinate conversion | `trig` |
| `dusk.go` | Package doc, `Observer`, result types, sentinel errors | — |
| `solar.go` | `SunriseSunset`, the three twilights, solar helpers | `trig`, `epoch`, `dusk` |
| `lunar.go` | `MoonriseMoonset`, `LunarPhase`, Meeus Table 47.A/B | `trig`, `epoch`, `dusk` |
| `cmd/dusk/` | The reference CLI | the public API only |

`solar.go` and `lunar.go` never call each other. Everything they share lives
below them in `epoch.go` and `trig.go`.

Here is the entire public surface — six sentinel errors, one constructor,
five calculations, four result types:

```bash
go doc . | sed -n '21,40p'
```

```output
const ErrCircumpolar = stringError("dusk: object is circumpolar (always above the horizon)")
const ErrDateOutOfRange = stringError("dusk: date outside valid range (1677-09-21 to 2262-04-11)")
const ErrInvalidCoord = stringError("dusk: latitude must be in [-90, 90] and longitude in [-180, 180]")
const ErrNeverRises = stringError("dusk: object never rises at this latitude")
const ErrNilLocation = stringError("dusk: location must not be nil")
const ErrNonFiniteCoord = stringError("dusk: coordinates must be finite (NaN and Inf are not allowed)")
type LunarPhaseInfo struct{ ... }
    func LunarPhase(date time.Time) (LunarPhaseInfo, error)
type MoonEvent struct{ ... }
    func MoonriseMoonset(date time.Time, obs Observer) (MoonEvent, error)
type Observer struct{ ... }
    func NewObserver(lat, lon float64, loc *time.Location) (Observer, error)
type SunEvent struct{ ... }
    func SunriseSunset(date time.Time, obs Observer) (SunEvent, error)
type TwilightEvent struct{ ... }
    func AstronomicalTwilight(date time.Time, obs Observer) (TwilightEvent, error)
    func CivilTwilight(date time.Time, obs Observer) (TwilightEvent, error)
    func NauticalTwilight(date time.Time, obs Observer) (TwilightEvent, error)
```

Note what is *not* there: no configuration, no options struct, no interfaces.
Every function takes a value and returns a value.

---

## 1. The foundation: degrees, not radians

Meeus's formulas are written in degrees. Rather than sprinkle conversions
through the astronomy, `trig.go` wraps `math` once and the rest of the
codebase never sees a radian.

```bash
sed -n '1,22p' trig.go
```

```output
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
```

`clamp` is the only one of these with behavior of its own, and it is doing
real work: a long chain of floating-point trig can produce `1.0000000000000002`
where the mathematics guarantees at most `1`, and `math.Asin` of that is `NaN`.
Clamping trades a silent `NaN` for a silently-corrected value — a deliberate
trade the comment argues for explicitly.

Two normalizers close the file. Every angle in the codebase passes through one
of them:

```bash
sed -n '28,44p' trig.go
```

```output
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
```

---

## 2. Time: the Julian date, and the range that bounds everything

Astronomy counts time in Julian days — a continuous day count that ignores
calendars entirely. `epoch.go` converts to and from it.

The conversion is three lines, but the doc comment is the important part:

```bash
sed -n '8,44p' epoch.go
```

```output
const (
	j1970 = 2440587.5
	j2000 = 2451545.0
)

// julianDate returns the Julian date for a given time, i.e., the continuous
// count of days and fractions of day since the beginning of the Julian period.
//
// Uses UnixNano internally, which limits the valid range to the int64
// nanosecond bounds (approximately 1677-09-21 to 2262-04-11). UnixNano's
// result is undefined outside that range: it wraps to an arbitrary value
// rather than to zero or any recognizable sentinel, so dates outside it
// silently produce incorrect results.
// Use [validJulianDateRange] to check before calling.
func julianDate(t time.Time) float64 {
	ms := t.UTC().UnixNano() / 1e6

	return float64(ms)/86400000.0 + j1970
}

// ErrDateOutOfRange is returned when a date falls outside the valid range
// for Julian date calculations (the int64 nanosecond bounds, approximately
// 1677-09-21 to 2262-04-11).
const ErrDateOutOfRange = stringError("dusk: date outside valid range (1677-09-21 to 2262-04-11)")

// julianDateMin and julianDateMax are the bounds of the int64 UnixNano range.
var (
	julianDateMin = time.Unix(0, math.MinInt64).UTC()
	julianDateMax = time.Unix(0, math.MaxInt64).UTC()
)

// validJulianDateRange reports whether t falls within the valid range for
// [julianDate]. Returns nil if valid, [ErrDateOutOfRange] otherwise.
func validJulianDateRange(t time.Time) error {
	if t.Before(julianDateMin) || t.After(julianDateMax) {
		return ErrDateOutOfRange
	}
```

Because `julianDate` reads `UnixNano`, the library inherits int64's range:
roughly 1677 to 2262. **Every public entry point calls
`validJulianDateRange` before doing anything else** — that is the one
invariant keeping `julianDate` from silently returning nonsense.

The comment is precise about *how* it fails — `UnixNano` wraps to an
arbitrary value rather than to zero or any recognizable sentinel — because an
earlier version claimed it returned 0, and a maintainer who believed that
might reasonably skip a range check on a new call path.

### Sidereal time

To know where the sky is pointing you need sidereal time, not clock time.
The implementation carries a warning worth reading twice:

```bash
sed -n '46,76p' epoch.go
```

```output
	return nil
}

// greenwichMeanSiderealTime returns the mean sidereal time at Greenwich in
// degrees for the given instant.
//
// See Meeus, Astronomical Algorithms, eq. 12.4 p. 88.
func greenwichMeanSiderealTime(t time.Time) float64 {
	// T is computed from midnight UTC, not from t. This matches Meeus's
	// formulation: the polynomial terms use 0h UT for the date, while the
	// linear term (360.985… × (JD − J2000)) uses the full Julian date to
	// account for the fractional day. Do not "simplify" by passing t here.
	d := datetimeZeroHour(t)
	T := julianCentury(d)
	JD := julianDate(t)

	theta := 280.46061837 +
		360.98564736629*(JD-j2000) +
		0.000387933*T*T -
		T*T*T/38710000.0

	return mod360(theta)
}

// localSiderealTime returns the local sidereal time in hours for a given
// instant and observer longitude (east positive, west negative, in degrees).
func localSiderealTime(t time.Time, longitude float64) float64 {
	gst := greenwichMeanSiderealTime(t) // degrees
	lst := gst + longitude              // degrees

	return mod24(lst / 15.0)
```

That comment ("Do not 'simplify' by passing t here") marks a genuine trap:
the polynomial terms and the linear term take their time from different
places, and collapsing them looks like a cleanup but is a bug.

### From the ecliptic to the horizon

Positions arrive in ecliptic coordinates and have to reach altitude and
azimuth. Two conversions do it. The first applies full nutation — both the
wobble in longitude and the wobble in obliquity:

```bash
sed -n '159,182p' epoch.go
```

```output
// ---------------------------------------------------------------------------

// eclipticToEquatorial converts ecliptic coordinates (longitude, latitude in
// degrees) to equatorial coordinates using nutation-corrected obliquity and
// nutation in longitude.
//
// See Meeus, Astronomical Algorithms, eq. 13.3 & 13.4 p. 93.
func eclipticToEquatorial(t time.Time, lon, lat float64) equatorial {
	T := julianCentury(t)

	L := solarMeanLongitude(T)
	l := lunarMeanLongitude(T)
	omega := lunarAscendingNode(T)

	dpsi := nutationInLongitude(L, l, omega)
	lon += dpsi

	eps := meanObliquity(T) + nutationInObliquity(L, l, omega)

	ra := atan2x(sinx(lon)*cosx(eps)-tanx(lat)*sinx(eps), cosx(lon))
	dec := asinx(sinx(lat)*cosx(eps) + cosx(lat)*sinx(eps)*sinx(lon))

	return equatorial{
		ra:  mod360(ra),
```

The second turns equatorial coordinates into what an observer actually sees,
and has to guard a division that vanishes at the poles and at the zenith:

```bash
sed -n '184,226p' epoch.go
```

```output
	}
}

// equatorialToHorizontal converts equatorial coordinates to horizontal
// (altitude/azimuth) for the given observer position and time.
//
// See Meeus, Astronomical Algorithms, eq. 13.5 & 13.6 p. 93.
func equatorialToHorizontal(t time.Time, obs Observer, eq equatorial) horizontal {
	lst := localSiderealTime(t, obs.lon)
	ha := hourAngle(eq.ra, lst)

	alt := asinx(sinx(eq.dec)*sinx(obs.lat) + cosx(eq.dec)*cosx(obs.lat)*cosx(ha))

	cosAltCosLat := cosx(alt) * cosx(obs.lat)

	var az float64
	// Guard against division by zero at the poles (lat ±90) or zenith (alt 90).
	if math.Abs(cosAltCosLat) < 1e-10 {
		az = 0
	} else {
		az = acosx((sinx(eq.dec) - sinx(alt)*sinx(obs.lat)) / cosAltCosLat)
	}

	// acos gives 0..180; if sin(ha) > 0, object is west, so az = 360 - az
	if sinx(ha) > 0 {
		az = 360 - az
	}

	return horizontal{
		alt: alt,
		az:  az,
	}
}

// hourAngle computes the hour angle in degrees.
//
// Parameters use mixed units:
//   - ra: right ascension in degrees (0-360)
//   - lst: local sidereal time in hours (0-24)
//
// The conversion lst*15 is applied internally, so callers must not
// pre-convert LST to degrees.
func hourAngle(ra, lst float64) float64 {
```

`hourAngle` closing the file is a wart the codebase admits to: it takes right
ascension in **degrees** and local sidereal time in **hours**, and does the
`×15` internally. The doc comment shouts about it because the alternative —
converting at every call site — would stop the surrounding code from looking
like the equations in the book.

---

## 3. The Observer, and the shape of an answer

`dusk.go` holds no astronomy. It defines who is asking and what an answer
looks like.

Errors first. They are declared as **constants**, which ordinary
`errors.New` cannot do:

```bash
sed -n '27,51p' dusk.go
```

```output
// stringError is an immutable error type used for sentinel errors.
// Unlike errors.New, these can be declared as constants.
type stringError string

func (e stringError) Error() string { return string(e) }

// ErrCircumpolar is returned when a celestial object is circumpolar
// (always above the horizon) at the given latitude.
const ErrCircumpolar = stringError("dusk: object is circumpolar (always above the horizon)")

// ErrNeverRises is returned when a celestial object never rises above
// the horizon at the given latitude.
const ErrNeverRises = stringError("dusk: object never rises at this latitude")

// ErrNilLocation is returned when a nil *time.Location is passed to
// [NewObserver].
const ErrNilLocation = stringError("dusk: location must not be nil")

// ErrNonFiniteCoord is returned when NaN or Inf coordinates are passed
// to [NewObserver].
const ErrNonFiniteCoord = stringError("dusk: coordinates must be finite (NaN and Inf are not allowed)")

// ErrInvalidCoord is returned when latitude or longitude are outside
// the valid range in [NewObserver].
const ErrInvalidCoord = stringError("dusk: latitude must be in [-90, 90] and longitude in [-180, 180]")
```

Two of these six are not failures at all. `ErrCircumpolar` and
`ErrNeverRises` describe **geometry** — the midnight sun and the polar night.
A caller above the Arctic Circle will meet them on a perfectly ordinary day,
and the CLI treats them as results and exits 0.

`Observer` validates once, at construction, and then cannot be changed:

```bash
sed -n '62,98p' dusk.go
```

```output

// Observer represents a geographic position on Earth used as the viewpoint
// for all astronomical calculations.
type Observer struct {
	lat float64
	lon float64
	loc *time.Location
}

// NewObserver constructs an Observer after validating all inputs.
// lat must be in [-90, 90], lon in [-180, 180], and loc must not be nil.
// NaN and infinite values are rejected.
func NewObserver(lat, lon float64, loc *time.Location) (Observer, error) {
	if loc == nil {
		return Observer{}, ErrNilLocation
	}

	if math.IsNaN(lat) || math.IsInf(lat, 0) || math.IsNaN(lon) || math.IsInf(lon, 0) {
		return Observer{}, ErrNonFiniteCoord
	}

	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return Observer{}, ErrInvalidCoord
	}

	return Observer{lat: lat, lon: lon, loc: loc}, nil
}

// Lat returns the observer's latitude in degrees.
func (o Observer) Lat() float64 { return o.lat }

// Lon returns the observer's longitude in degrees (east positive, west negative).
func (o Observer) Lon() float64 { return o.lon }

// Location returns the observer's timezone.
func (o Observer) Location() *time.Location { return o.loc }

```

The fields are unexported and there are no setters, so a value that exists is
a value that passed validation. The cost is `validObserver`, which every
public entry point calls to catch a zero-value `Observer{}` that never went
through the constructor.

The result types are plain data, and they use **two different ways of saying
"nothing happened"**:

```bash
sed -n '109,146p' dusk.go
```

```output
// SunEvent holds the times of sunrise, solar noon, sunset, and the duration
// of daylight for a single day.
type SunEvent struct {
	Rise     time.Time
	Noon     time.Time
	Set      time.Time
	Duration time.Duration
}

// MoonEvent holds the rise and set times for the Moon on a given day, along
// with the duration between rise and set.
type MoonEvent struct {
	Rise         time.Time // zero value if the Moon does not rise
	Set          time.Time // zero value if the Moon does not set
	AboveHorizon bool      // true if Moon was above the horizon at start of day
}

// TwilightEvent holds the dusk and dawn times of a twilight period.
// Dusk is tonight's boundary (sun passes below the depression angle).
// Dawn is tomorrow morning's boundary (sun passes above the depression angle).
// To get this morning's dawn, call with yesterday's date.
type TwilightEvent struct {
	Dusk          time.Time     // evening boundary (today)
	Dawn          time.Time     // morning boundary (tomorrow)
	NightDuration time.Duration // time from Dusk to Dawn (overnight darkness)
}

// LunarPhaseInfo describes the Moon's current phase.
type LunarPhaseInfo struct {
	Illumination float64 // percentage 0-100
	Elongation   float64 // degrees 0-360
	Angle        float64 // phase angle in degrees (may be negative per Meeus formula)
	DaysApprox   float64 // rough days into lunation (linear estimate from elongation)
	Waxing       bool    // true from New Moon to Full Moon (elongation 0-180)
	Name         string  // "New Moon", "Waxing Crescent", etc.
}

// equatorial represents right ascension and declination in degrees.
```

A **zero `time.Time`** means "this did not happen today" — the Moon rises but
does not set before midnight. A **sentinel error** means "the geometry forbids
this here" — the sun never clears the horizon at all. `MoonEvent` uses the
first, `SunEvent` the second, and mixing them up is the easiest mistake a
caller can make.

Note also `TwilightEvent`: `Dusk` is tonight's, but `Dawn` is *tomorrow
morning's*. That asymmetry drives a real complication in the CLI, below.

---

## 4. The Sun

Sunrise, sunset and all three twilights are the same calculation with one
number changed. `solarParams` is the shared chokepoint:

```bash
sed -n '7,27p' solar.go
```

```output
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
```

Six steps — mean solar time, mean anomaly, equation of center, ecliptic
longitude, declination, transit — and everything downstream is a pure function
of `{delta, jTransit}` plus a latitude and a depression angle. A new
sun-based boundary ("blue hour", say) is a new depression value, not a new
algorithm.

`SunriseSunset` puts that to work:

```bash
sed -n '29,70p' solar.go
```

```output
// SunriseSunset computes sunrise, solar noon, and sunset for the given date
// and observer position. The observer must be constructed via [NewObserver].
// The date is converted to the observer's timezone to determine the local
// calendar day; the time-of-day is ignored.
// Output times are converted to the observer's timezone.
//
// The algorithm follows the NOAA solar calculator method (derived from Meeus,
// Astronomical Algorithms).
func SunriseSunset(date time.Time, obs Observer) (SunEvent, error) {
	err := validObserver(obs)
	if err != nil {
		return SunEvent{}, err
	}

	localDate := date.In(obs.loc)

	date = time.Date(localDate.Year(), localDate.Month(), localDate.Day(), 0, 0, 0, 0, time.UTC)

	err = validJulianDateRange(date)
	if err != nil {
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
```

The shape here repeats at every public entry point: **validate the observer,
resolve the calendar day in the observer's zone, validate the range, then
compute.** That second step is a contract callers must respect —
`date.In(obs.loc)` picks the local day and throws the time away, so a
`time.UTC` midnight lands on the *previous* day for anyone west of Greenwich.

The rise and set times are symmetric about transit: subtract the hour angle
for one, add it for the other.

### Where the polar cases come from

Everything interesting happens in `solarHourAngle`, and it is only twenty
lines:

```bash
sed -n '119,155p' solar.go
```

```output
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
```

`cosHA` is the cosine of the hour angle at which the sun reaches the target
depth. If that cosine falls outside [-1, 1] there is **no such moment** — and
which side it falls off tells you which polar case you are in. Midnight sun
and polar night are not error handling bolted on; they drop out of the
geometry.

The `depression` parameter is what makes one function serve four boundaries:
pass 0 for sunrise (with its −0.83° nod to refraction and the sun's radius),
or 6, 12, 18 for the twilights.

### Twilight

The three exported twilights are one line each:

```bash
sed -n '166,184p' solar.go
```

```output
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

```

And `twilight` spans **two calendar days**, because tonight's dusk and
tomorrow's dawn are the two ends of one night:

```bash
sed -n '208,244p' solar.go
```

```output
	if err != nil {
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

	err = validJulianDateRange(tomorrow)
	if err != nil {
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
```

Two `computeSolarParams` calls, one per day. Because both must succeed, a
polar transition date — where tonight's dusk exists but tomorrow's dawn does
not — fails the whole call rather than returning half an answer.

---

## 5. The Moon

The Moon is harder. There is no closed form for when it crosses the horizon,
so `lunar.go` is longer than everything above it combined — most of it
transcribed coefficient tables.

Its position comes from Meeus Chapter 47: a mean longitude plus sixty periodic
corrections, each a sine of some combination of five angles.

```bash
sed -n '21,60p' lunar.go
```

```output
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
```

The `switch r.M` is not a style choice. Terms involving the Sun's anomaly are
scaled by the Earth's changing orbital eccentricity — once for `M = ±1`,
twice for `M = ±2` — exactly as Meeus prescribes.

The tables themselves are Go array literals, with the Greek field names kept
so a reader can check them against the book:

```bash
sed -n '283,296p' lunar.go && echo '  ...' && echo "tableLongDist: $(grep -c '^\s*{' <(sed -n '292,367p' lunar.go)) rows, tableLat: $(grep -c '^\s*{' <(sed -n '369,445p' lunar.go)) rows"
```

```output
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
  ...
tableLongDist: 59 rows, tableLat: 57 rows
```

### Moonrise by brute force

With no closed form available, `MoonriseMoonset` simply walks the day a
minute at a time and watches for the altitude to change sign:

```bash
sed -n '168,215p' lunar.go
```

```output
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
```

That is ~1440 full Meeus evaluations per call, and the doc comment owns it:
"slow by design", 1–2 ms, and a warning that a 30-day calendar costs 30–60 ms.

Three details worth catching:

- The day is built **in the observer's zone and then converted to UTC**, so
  `scanMinutes` is 1380 on a spring-forward day and 1500 on a fall-back day
  rather than a hardcoded 1440.
- `aboveAtStart` is sampled before the loop. It is the only way to explain a
  moonset that appears *before* a moonrise on the same day.
- There is no sentinel error here. A Moon that never crosses returns two zero
  times, and the caller must consult `AboveHorizon` to tell "up all day" from
  "down all day".

### Phase

`LunarPhase` is the one entry point that uses the **exact instant** rather
than the calendar day — phase changes continuously, and the same moment looks
the same from everywhere on Earth:

```bash
sed -n '100,128p' lunar.go
```

```output
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
```

Elongation runs 0–360° and decides waxing from waning; the phase angle `PA`
is the Sun–Moon–Earth angle, and illumination follows from its cosine. The
raw elongation is also mapped to one of eight names by fixed 22.5° bands in
`lunarPhaseName`.

---

## 6. The reference CLI

`cmd/dusk` exists to be the worked example: it calls **every** exported
function, and every documented edge case is reachable with one flag. It is
three files — flags and errors, assembly, rendering.

### main.go: an error taxonomy

The exit status is always 1 on failure, so the *message* has to carry the
distinction:

```bash
sed -n '29,39p' cmd/dusk/main.go
```

```output
// errUsage reports a command line the tool cannot act on.
var errUsage = errors.New("usage")

// errNoBuildInfo reports a binary carrying no embedded build information,
// which happens only to one built in a way `go build` does not.
var errNoBuildInfo = errors.New("no build information is embedded")

// errUnsupportedDate reports a run that was asked for correctly and cannot be
// delivered. The command line was well formed; what failed was the date, and a
// reader told "usage" goes looking for a flag he typed wrong.
var errUnsupportedDate = errors.New("unsupported date")
```

`run` takes its streams as arguments so that every branch is reachable from a
test, leaving `main` with nothing but the real streams and the exit code:

```bash
sed -n '41,47p' cmd/dusk/main.go && echo '' && sed -n '82,116p' cmd/dusk/main.go
```

```output
func main() {
	err := run(os.Args[1:], os.Stdout, os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dusk:", err)
		os.Exit(1)
	}
}

	if *showVersion {
		return writeVersion(stdout)
	}

	obs, err := observerFromFlags(*lat, *lon, *tz, setFlags(fs))
	if err != nil {
		_ = writeUsage(stderr, fs)

		return err
	}

	// The date must be parsed in the observer's zone, not UTC: the library
	// derives the calendar day with date.In(observer location), so a UTC
	// midnight lands on the previous day for every observer west of Greenwich.
	date, err := parseDate(*dateArg, obs.Location())
	if err != nil {
		return err
	}

	day, err := buildReport(obs, date)
	if err != nil {
		// A date the library will not compute is not a misuse of the flags.
		if errors.Is(err, dusk.ErrDateOutOfRange) {
			return fmt.Errorf("%w: %w", errUnsupportedDate, err)
		}

		return err
	}

	if *asJSON {
		return renderJSON(stdout, day)
	}

	return renderText(stdout, day)
}
```

Two small decisions do a lot of work here:

- `setFlags` records which flags were *actually typed*, so `--lat 0` (the
  equator) is distinguishable from an omitted `--lat`. All three of `--lat`,
  `--lon` and `--tz` are required because none has a defensible default.
- `parseDate` anchors the date at **midday**, not midnight. A handful of zones
  move their clocks at midnight — Santiago in September, Havana in March —
  where 00:00 does not exist and would silently resolve to the previous day.

```bash
sed -n '149,167p' cmd/dusk/main.go
```

```output
// parseDate parses --date in the observer's zone, defaulting to today there.
func parseDate(arg string, loc *time.Location) (time.Time, error) {
	if arg == "" {
		return time.Now().In(loc), nil
	}

	// Parsed without a zone, then anchored at midday in the observer's. A few
	// zones move their clocks at midnight - America/Santiago in September,
	// America/Havana in March - and there 00:00 does not exist, so
	// ParseInLocation resolves it to 23:00 the day before and the whole report
	// silently comes out for the previous day. Midday exists everywhere; the
	// library ignores the time of day and keeps only the calendar date.
	day, err := time.Parse(dateLayout, arg)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: --date %q is not YYYY-MM-DD: %w", errUsage, arg, err)
	}

	return time.Date(day.Year(), day.Month(), day.Day(), 12, 0, 0, 0, loc), nil
}
```

### report.go: turning errors into a value

The library reports polar geometry with sentinel errors. The CLI has to
*print* something either way, so it converts the sentinels into a value the
renderer can branch on:

```bash
sed -n '14,36p' cmd/dusk/report.go
```

```output
// horizonState says whether a crossing happened, or why the geometry forbade
// it. The library signals this with two sentinel errors; carrying it as a value
// lets the renderer compose its own prose instead of parsing ours.
type horizonState int

const (
	stateCrosses    horizonState = iota // the times are real
	stateStaysAbove                     // ErrCircumpolar at this angle
	stateStaysBelow                     // ErrNeverRises at this angle
)

// stateOf maps the library's sentinels onto horizonState. Any other error is a
// genuine failure and is reported as not-a-state.
func stateOf(err error) (horizonState, bool) {
	switch {
	case errors.Is(err, dusk.ErrCircumpolar):
		return stateStaysAbove, true
	case errors.Is(err, dusk.ErrNeverRises):
		return stateStaysBelow, true
	default:
		return stateCrosses, false
	}
}
```

The `ok` return is the important half: a sentinel is a *state* to be
described, anything else is a real failure that should abort the run.

**The single subtlest thing in the codebase** lives in `twilightReports`.
Recall that `TwilightEvent.Dawn` is *tomorrow* morning's. A day report wants
*this* morning's — so each band is computed twice, against two different dates:

```bash
sed -n '195,237p' cmd/dusk/report.go
```

```output
// twilightReports computes all three twilight bands for the day.
//
// dusk.TwilightEvent is deliberately asymmetric: Dusk is tonight's boundary and
// Dawn is *tomorrow* morning's. A day report wants this morning's dawn instead,
// so each band is computed twice - Dawn from yesterday's event, Dusk from
// today's. This is the single most easily missed detail in the library's
// contract, which is why the reference implementation does it explicitly.
func twilightReports(date time.Time, obs dusk.Observer) ([]TwilightReport, error) {
	yesterday := date.AddDate(0, 0, -1)
	reports := make([]TwilightReport, 0, len(twilightBands))

	for _, band := range twilightBands {
		report := TwilightReport{Name: band.name, degrees: band.degrees}

		// Yesterday's call supplies this morning's dawn and nothing else. Its
		// state is discarded: on a polar transition day it describes a night
		// the report is not about, and letting it stand printed "twilight
		// never arrives" above tonight's real dusk time.
		morning, _, err := callTwilight(band.fn, yesterday, obs, band.name)
		if err != nil {
			return nil, err
		}

		report.Dawn = toSecond(morning.Dawn)

		// Tonight's geometry is the one the report is about. When it is not a
		// crossing the event is the zero value, so Dusk and Night fall out
		// empty without being cleared.
		evening, state, err := callTwilight(band.fn, date, obs, band.name)
		if err != nil {
			return nil, err
		}

		report.Dusk = toSecond(evening.Dusk)
		report.Night = shortDuration(evening.NightDuration)
		report.state = state
		report.Note = twilightNote(state, band.degrees)

		reports = append(reports, report)
	}

	return reports, nil
}
```

Yesterday's call supplies the dawn and nothing else — its *state* is thrown
away deliberately, because on a polar transition day it describes a night
this report is not about.

### render.go: a day in the order it is lived

The library returns results grouped by the call that produced them — sun,
three twilight bands, moon. Nobody experiences a day that way. `renderText`
flattens everything into one clock-ordered list:

```bash
sed -n '80,117p' cmd/dusk/render.go
```

```output
// timeline collects every event that actually happened and orders them by the
// clock. A zero time means the event did not occur today and is simply absent.
func timeline(report Report) []dayEvent {
	var events []dayEvent

	// A sunset at 00:03 belongs to the day after the one being reported. It
	// sorts to the end correctly, but a bare "00:03" under a "19:00" reads as
	// a mistake, so the day it lands on is said out loud.
	day := report.date.Day()

	add := func(at time.Time, label string) {
		if at.IsZero() {
			return
		}

		if at.Day() != day {
			label += " (" + at.Format("2 Jan") + ")"
		}

		events = append(events, dayEvent{at: at, label: label})
	}

	add(report.Sun.Rise, "Sunrise")
	add(report.Sun.Noon, "Solar noon")
	add(report.Sun.Set, "Sunset")

	for _, band := range report.Twilight {
		add(band.Dawn, band.Name+" dawn")
		add(band.Dusk, band.Name+" dusk")
	}

	add(report.Moon.Rise, "Moonrise")
	add(report.Moon.Set, "Moonset")

	slices.SortFunc(events, func(a, b dayEvent) int { return a.at.Compare(b.at) })

	return events
}
```

The timeline can only show events that *happened*. A polar day would print a
short list of times with no hint that the sun never came up, so a prose block
sits above it. The twilight half collapses three facts into one sentence:

```bash
sed -n '162,206p' cmd/dusk/render.go
```

```output
// twilightConditions collapses the bands that never arrive into one sentence
// each way. Printing one line per band repeats the same fact three times: if
// the sun never drops 6 degrees below the horizon it never drops 12 or 18
// either, so the shallowest band that fails is the whole story.
func twilightConditions(bands []TwilightReport) []string {
	var (
		lines    []string
		tooLight []string
		tooDark  []string
		lightest = 90
		deepest  int
	)

	for _, band := range bands {
		switch band.state {
		case stateStaysAbove:
			tooLight = append(tooLight, strings.ToLower(band.Name))
			lightest = min(lightest, band.degrees)
		case stateStaysBelow:
			tooDark = append(tooDark, strings.ToLower(band.Name))
			deepest = max(deepest, band.degrees)
		case stateCrosses:
		}
	}

	// Scoped to tonight, because a band's state is tonight's. On a transition
	// day this morning's dawn came from yesterday's call and is real, so an
	// unqualified "never drops 6° below the horizon" would sit directly above
	// the dawn that disproves it.
	if len(tooLight) > 0 {
		lines = append(lines, fmt.Sprintf(
			"The sun never drops %d° below the horizon tonight, so %s twilight never arrives.",
			lightest, join(tooLight),
		))
	}

	if len(tooDark) > 0 {
		lines = append(lines, fmt.Sprintf(
			"The sun never climbs within %d° of the horizon, so %s darkness lasts all day.",
			deepest, join(tooDark),
		))
	}

	return lines
}
```

If the sun never drops 6° below the horizon it never drops 12° or 18° either,
so naming all three bands says the same thing three times — only the
shallowest failing band is reported. And the sentence is scoped to *tonight*,
because on a transition day this morning's dawn is real and an unqualified
claim would sit directly above the dawn that disproves it.

`moonCondition` handles the case with no sentinel to lean on:

```bash
sed -n '208,232p' cmd/dusk/render.go
```

```output
// moonCondition explains a moon that does not cross, and flags a moon that is
// already up at midnight - which is why a moonset can precede a moonrise.
func moonCondition(moon MoonReport) string {
	switch {
	case moon.Rise.IsZero() && moon.Set.IsZero():
		// A lunar day runs about 24h50m, so at high latitudes the Moon can be
		// up for the whole calendar day without crossing. Unlike the Sun, the
		// library reports this with AboveHorizon rather than a sentinel error
		// - MoonriseMoonset never returns one - and without consulting it the
		// report reads as though the Moon were absent.
		if moon.AboveHorizon {
			return "The moon stays above the horizon all day."
		}

		return "The moon neither rises nor sets today."
	case moon.Rise.IsZero():
		return "The moon is already up at midnight and does not rise again today."
	case moon.Set.IsZero():
		return "The moon does not set today."
	case moon.AboveHorizon:
		return "The moon is already up at midnight, so today's moonset precedes its moonrise."
	default:
		return ""
	}
}
```

---

## 7. Seeing it run

An ordinary mid-latitude day. Everything crosses, so there is no prose block
at all — just the timeline and the totals:

```bash
go run ./cmd/dusk --lat 42.9634 --lon -85.6681 --tz America/Detroit --date 2025-06-21
```

```output
Saturday 21 June 2025
42.9634°N  85.6681°W  ·  America/Detroit

  02:37   Moonrise
  03:45   Astronomical dawn
  04:42   Nautical dawn
  05:28   Civil dawn
  06:03   Sunrise
  13:44   Solar noon
  17:35   Moonset
  21:25   Sunset
  22:00   Civil dusk
  22:46   Nautical dusk
  23:43   Astronomical dusk

  Daylight   15h21m
  Dark        4h02m  (astronomical, tonight)
  Moon       Waning Crescent, 19%
```

Tromsø, 69.6°N, on the winter solstice. `SunriseSunset` returns
`ErrNeverRises`, but civil twilight still arrives — and the report says so in
one sentence rather than leaving the two facts to contradict each other:

```bash
go run ./cmd/dusk --lat 69.6492 --lon 18.9553 --tz Europe/Oslo --date 2025-12-21
```

```output
Sunday 21 December 2025
69.6492°N  18.9553°E  ·  Europe/Oslo

  The sun does not rise today (polar night). Twilight still reaches
  civil depth around midday.
  The moon neither rises nor sets today.

  06:28   Astronomical dawn
  07:46   Nautical dawn
  09:31   Civil dawn
  13:53   Civil dusk
  15:37   Nautical dusk
  16:56   Astronomical dusk

  Dark       13h33m  (astronomical, tonight)
  Moon       New Moon, 2%
```

The same place six months later. Now the sun never sets, and no band of
twilight ever arrives — `twilightConditions` collapses all three into a single
clause instead of repeating the fact three times. And with the moon already up
at midnight, its **moonset precedes its moonrise**; without `moonCondition`
saying so, that timeline would read as a sorting bug:

```bash
go run ./cmd/dusk --lat 69.6492 --lon 18.9553 --tz Europe/Oslo --date 2025-06-21
```

```output
Saturday 21 June 2025
69.6492°N  18.9553°E  ·  Europe/Oslo

  The sun does not set today (midnight sun).
  The sun never drops 6° below the horizon tonight, so civil,
  nautical and astronomical twilight never arrives.
  The moon is already up at midnight, so today's moonset precedes
  its moonrise.

  19:41   Moonset
  22:18   Moonrise

  Moon       Waning Crescent, 21%
```

And the machine-readable form. Times use `omitzero`, not `omitempty` — a zero
`time.Time` is a struct, and `omitempty` would happily emit
`"0001-01-01T00:00:00Z"`. Events that did not occur are simply absent, and the
polar state survives as a `note`:

```bash
go run ./cmd/dusk --lat 69.6492 --lon 18.9553 --tz Europe/Oslo --date 2025-12-21 --json | head -22
```

```output
{
  "lat": 69.6492,
  "lon": 18.9553,
  "zone": "Europe/Oslo",
  "date": "2025-12-21",
  "sun": {
    "note": "polar night - the sun does not rise today"
  },
  "twilight": [
    {
      "name": "Civil",
      "dawn": "2025-12-21T09:31:12+01:00",
      "dusk": "2025-12-21T13:53:14+01:00",
      "night": "19h38m"
    },
    {
      "name": "Nautical",
      "dawn": "2025-12-21T07:46:45+01:00",
      "dusk": "2025-12-21T15:37:42+01:00",
      "night": "16h10m"
    },
    {
```

---

## 8. How the project holds itself together

CI runs exactly one command: `task`. Nothing is checked in CI that you cannot
run locally, and nothing runs locally that CI skips.

The unusual part is how coverage is enforced. Instead of a percentage, a file
records the **count of uncovered statements per package**:

```bash
cat coverage.ratchet
```

```output
# Uncovered statements per package. The gate fails in both directions:
# a number that rises is lost coverage, one that falls is coverage to lock in.
# Regenerate with: task ratchet:update
github.com/philoserf/dusk/v3 10
github.com/philoserf/dusk/v3/cmd/dusk 19
```

The gate diffs the current counts against that file and fails **in both
directions** — a number that rises is lost coverage, a number that falls is
coverage to lock in with `task ratchet:update`. It also fails if a package
appears or vanishes.

An integer rather than a percentage because a percentage holds still while a
guarded branch adds one covered statement and one uncovered, and it grows more
forgiving as the repository grows. The entire check is an awk program in
`Taskfile.yml` — there is no tool to maintain.

One consequence worth knowing before you panic at a red gate: reflowing blank
lines splits coverage blocks, so a pure refactor can move these numbers
without changing what the tests actually reach. Read the diff first.

---

## Where to look first

| Symptom | Start here |
| --- | --- |
| Wrong times for a given zone | the `date.In(obs.loc)` line at the top of the affected public function |
| Wrong rise time at high latitude | `solarHourAngle`'s `cosHA < -1` / `> 1` branches, and whether `depression` is 0 |
| Moon test drifting after a toolchain update | whether `clamp` is hiding a `NaN` in `asinx`/`acosx` |
| Wrong phase name | `lunarPhaseName`'s 22.5° cutoffs, not the phase math |
| Dawn appears to run backwards | `twilightReports` — `Dawn` comes from *yesterday's* call |

For the design rationale behind these shapes — why `Observer` validates once,
why v3 dropped elevation and `ObjectTransit` — see `THEORY.md`.
