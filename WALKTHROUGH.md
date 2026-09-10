# dusk Walkthrough

*2026-09-09T22:54:40Z by Showboat 0.6.1*
<!-- showboat-id: f8ba9125-7a28-40de-9ad7-4d44ed44893f -->

## Overview

`dusk` is a zero-dependency Go library that answers three questions about one place on
one day: when does the Sun cross a given depth below the horizon, when does the Moon
cross the horizon, and how much of the Moon is lit. The algorithms are Meeus's, from
_Astronomical Algorithms_ (2nd ed.), with the sunrise/sunset path following the NOAA
simplification of them.

There are seven exported functions and one constructor. Everything else in the package
is unexported machinery reached through them.

- `NewObserver(lat, lon, loc)` — the only way to build the viewpoint every calculation needs
- `SunriseSunset(date, obs)` — sunrise, solar noon, sunset, daylight duration
- `CivilTwilight` / `NauticalTwilight` / `AstronomicalTwilight` `(date, obs)` — the Sun at 6°, 12°, 18° below the horizon
- `MoonriseMoonset(date, obs)` — moonrise and moonset, plus whether the Moon was up at the start of the day
- `LunarPhase(date)` — illumination, elongation, approximate age, waxing, name

A reference CLI under `cmd/dusk` calls every one of them and is the executable
specification for the library's contract.

This walkthrough runs bottom-up through the library — angles, then time, then the Sun,
then the Moon — and then top-down through the CLI, which is the only place all seven
functions are used together. Every fenced block below was executed to produce the
output beneath it.

## Architecture

The library is a single package at the repository root. Go's package rules mean the
five `.go` files are one namespace with one doc comment — the split is for readers, not
for the compiler, so any unexported helper is reachable from any file.

```bash
git ls-files ':!:WALKTHROUGH.md' ':!:THEORY.md' ':!:CHANGELOG.md' ':!:LICENSE'
```

```output
.claude/settings.json
.github/dependabot.yml
.github/workflows/ci.yml
.github/workflows/claude.yml
.gitignore
.golangci.yml
Brewfile
CLAUDE.md
README.md
Taskfile.yml
benchmark_test.go
cmd/dusk/main.go
cmd/dusk/main_test.go
cmd/dusk/render.go
cmd/dusk/render_test.go
cmd/dusk/report.go
cmd/dusk/report_test.go
coverage.ratchet
dusk.go
dusk_test.go
epoch.go
epoch_test.go
example_test.go
fuzz_test.go
go.mod
lunar.go
lunar_test.go
solar.go
solar_test.go
trig.go
trig_test.go
```

| File          | What lives there                                                               |
| ------------- | ------------------------------------------------------------------------------ |
| `trig.go`     | Degree-mode trig wrappers, `clamp`, `mod360`/`mod24`                            |
| `epoch.go`    | Julian dates and their range check, sidereal time, nutation, coordinate changes |
| `dusk.go`     | Package doc, `Observer`, the result types, the sentinel errors                  |
| `solar.go`    | `SunriseSunset`, the three twilights, the solar helpers                         |
| `lunar.go`    | `LunarPhase`, `MoonriseMoonset`, and the transcribed Meeus tables               |
| `cmd/dusk/`   | The reference CLI: `main.go` flags, `report.go` assembly, `render.go` output    |

The dependency direction is strictly one way: `trig.go` depends on nothing, `epoch.go`
on `trig.go`, and `solar.go` and `lunar.go` on both. `cmd/dusk` sees only the exported
API, exactly as any other consumer would.

## Layer one: angles

Meeus's formulae are written in degrees; Go's `math` is written in radians. Rather than
convert at each of the several hundred call sites, `trig.go` wraps every trig function
the package uses so that **every angle in every other file is in degrees**. There is no
radian anywhere outside this file.

```bash
sed -n '5,26p' trig.go
```

```output
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
```

`clamp` is the piece worth pausing on, because its own comment argues against it. Values
that ought to lie in `[-1, 1]` arrive at `asin` and `acos` through long chains of
degree-mode trig, and floating-point rounding puts them at 1.0000000000000002 often
enough to matter. Clamping turns a NaN that would poison the whole result into a correct
answer — at the price of also absorbing a genuinely wrong 1.3. The package accepts that
trade and pays for it with reference-value tests against USNO and Meeus, which is why
weakening those tests is more dangerous here than a coverage number suggests.

The other half of the discipline is normalisation. An angle that has been added to or
subtracted from is wrapped at the point it becomes a result, not left to its consumer.

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

## Layer two: time

Every calculation happens in Julian days — the continuous count astronomers use — and
the polynomials are written around **J2000**, Julian date 2451545.0, which is noon UT on
1 January 2000. `j1970` is the same constant for the Unix epoch, and it is what lets
`julianDate` go through `time.Time`.

```bash
sed -n '8,26p' epoch.go
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
```

Going through `UnixNano` buys `time.Time` interop and costs a date range. The comment is
precise about the failure mode, and the precision is the point: `UnixNano` outside its
range does not return zero or any recognisable marker, it returns an arbitrary wrong
number. A missing range check here would not crash, it would produce a plausible
sunrise for the wrong century. So the bound is checked, and the sentinel that reports it
lives next to the bound rather than with the other errors in `dusk.go`.

```bash
sed -n '28,47p' epoch.go
```

```output
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

	return nil
}
```

Three small conversions sit on top of `julianDate`, and the difference between them is
load-bearing. `julianCentury` returns `T`, centuries since J2000, which is what the
Meeus polynomials take. `julianDay` **rounds** to an integer day number, which is what
the NOAA sunrise method takes. `meanSolarTime` applies the observer's longitude to that
day number — and note that it does so itself, after the rounding, which is why the solar
path must not be handed a timezone-adjusted instant.

```bash
sed -n '79,95p' epoch.go
```

```output
// julianCentury returns the number of Julian centuries elapsed since J2000.0.
func julianCentury(t time.Time) float64 {
	return (julianDate(t) - j2000) / 36525.0
}

// julianDay returns the number of days since J2000.0, rounded to the nearest
// integer (used for mean solar time).
func julianDay(t time.Time) int {
	JD := julianDate(t)

	return int(math.Round(JD - j2000))
}

// meanSolarTime returns the mean solar time for a given instant and longitude.
func meanSolarTime(t time.Time, longitude float64) float64 {
	return float64(julianDay(t)) - longitude/360.0
}
```

Sidereal time is where the Moon's path begins. The comment here is a warning left by
someone who was tempted:

```bash
sed -n '49,68p' epoch.go
```

```output
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
```

Two lines in that function take different arguments on purpose: `T` comes from midnight
UTC, `JD` from the instant itself. Collapse them and every moonrise moves.

## Layer three: the Observer and the result types

The `Observer` is the viewpoint every calculation needs, and it is not a container of
three numbers — it is a *validated* container of three numbers. That distinction is the
package's smallest and most consequential decision. Its fields are unexported and
`NewObserver` is the only way in.

```bash
sed -n '63,88p' dusk.go
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
```

Because construction validates, the entry points do not have to. Each one calls
`validObserver`, which tests exactly one thing — is `loc` nil — because a zero-value
`Observer` is the only invalid one Go's type system still permits.

```bash
sed -n '53,61p' dusk.go
```

```output
// validObserver returns an error if obs was not constructed via NewObserver
// (i.e., is a zero-value Observer with a nil location).
func validObserver(obs Observer) error {
	if obs.loc == nil {
		return ErrNilLocation
	}

	return nil
}
```

The errors themselves are `const`, not `var`. Go cannot declare an `error` constant
directly, so the package defines a string type with an `Error()` method and declares the
sentinels as constants of it — immutable, unlike anything returned by `errors.New`.

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

The four result types are deliberately inert — no methods, no computed accessors, no
behaviour. v4 removed the `String()` methods they briefly had. They are the boundary at
which this package stops having opinions about your output format.

Note `MoonEvent`'s third field. `AboveHorizon` exists because "no rise and no set" is
ambiguous between the Moon being up all day and down all day, and only that flag tells
them apart. Its doc comment still promises a duration the type does not have — see
`moonevent-doc-promises-a-duration-field-removed-in-v3` in `.issues/`.

```bash
sed -n '109,143p' dusk.go
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
	DaysApprox   float64 // rough days into lunation (linear estimate from elongation)
	Waxing       bool    // true from New Moon to Full Moon (elongation 0-180)
	Name         string  // "New Moon", "Waxing Crescent", etc.
}
```

## Layer four: the Sun

`SunriseSunset` is the shortest path through the whole system, so it is the right place
to start. Six steps produce two numbers — the Sun's declination and the Julian date of
solar transit — and the pair is shared with the twilight code, which is why it is
factored out.

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

```bash
sed -n '37,72p' solar.go
```

```output
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
	}, nil
}
```

Two lines in the middle of that function carry most of the contract. First
`localDate := date.In(obs.loc)`, then a `time.Date(...)` rebuilt from its year, month
and day.

The calendar day is resolved **in the observer's zone**, and the time of day is then
thrown away. A caller who builds the date with `time.UTC` and hands it to an observer
in Detroit gets the previous day, silently, with entirely plausible times — this is the
single easiest mistake to make against this API.

The day is then rebuilt as **UTC** midnight, not local midnight. That looks like a bug
and is not: `meanSolarTime` applies the observer's longitude itself, so handing it a
zone-adjusted instant would apply longitude twice. Remember this when the lunar path
does the opposite.

Everything hinges on `solarHourAngle` — how far from the meridian the Sun is when it
touches the angle you asked about. This is where polar geometry becomes an error.

```bash
sed -n '122,155p' solar.go
```

```output
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

The `-0.83` for sunrise/sunset is atmospheric refraction plus the Sun's semidiameter:
the Sun is *seen* to rise while still geometrically below the horizon. Twilight uses its
depression angle unadjusted, per the IAU/USNO definition — the two conventions are
different, which is why the function branches rather than always subtracting 0.83.

The two out-of-range branches are the whole polar story. `cosHA < -1` means the Sun
never gets *down* to the angle; `cosHA > 1` means it never gets *up* to it. At the
horizon those are midnight sun and polar night. At 18° below it they mean something a
reader gets backwards on first contact — the night never gets that dark, and the day
never gets that light — and the CLI has to say so out loud.

The three twilight functions are one function with the depression angle bound.

```bash
sed -n '166,183p' solar.go
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

`twilight` itself computes the solar parameters **twice** — once for the date you asked
about and once for the day after — because a `TwilightEvent` is not a symmetric bracket
around a night. It is the night that *starts* on the date you gave.

```bash
sed -n '197,244p' solar.go
```

```output
func twilight(date time.Time, obs Observer, depression float64) (TwilightEvent, error) {
	err := validObserver(obs)
	if err != nil {
		return TwilightEvent{}, err
	}

	localDate := date.In(obs.loc)

	date = time.Date(localDate.Year(), localDate.Month(), localDate.Day(), 0, 0, 0, 0, time.UTC)

	err = validJulianDateRange(date)
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

Two consequences fall out of that shape, and both bite in practice:

1. **To get this morning's dawn, call with yesterday's date.** Nothing enforces this;
   the doc comment is the whole contract.
2. **Either day failing fails the whole call.** Near 65–70°N there are transition dates
   where tonight's dusk is real and tomorrow's dawn is not. The function returns an
   error and discards the dusk it already computed. The doc comment concedes this and
   tells callers needing partial results to compute each boundary themselves.

## Layer five: the Moon

The lunar path shares `epoch.go` with the solar path and almost nothing else. It runs
the full Meeus Chapter 47 series — sixty periodic terms for longitude and distance,
sixty more for latitude — transcribed into the source as two tables.

```bash
sed -n '21,58p' lunar.go
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
```

The `switch r.M` is not a style choice. Meeus's `E` factor corrects for the eccentricity
of the Earth's orbit, and it is applied once to terms containing the Sun's mean anomaly
`M` and twice to terms containing `2M`. The switch reads that power off the table's own
`M` column. There is no `default` branch, so a row with `|M| > 2` would contribute
nothing at all — Meeus 47.A contains no such row, and neither does the transcription.

The tables are Go struct literals with Meeus's own column names, including the primed
`Mʹ` that distinguishes the Moon's mean anomaly from the Sun's.

```bash
sed -n '276,296p' lunar.go
```

```output
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
```

`LunarPhase` is the odd one out among the public functions: it takes no `Observer`,
because the Moon shows the same face to the whole Earth, and it uses the exact instant
rather than the calendar day, because phase changes continuously.

```bash
sed -n '92,128p' lunar.go
```

```output
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
		DaysApprox:   days,
		Waxing:       d < 180,
		Name:         lunarPhaseName(d),
	}, nil
}
```

`acosx` can only return 0–180, so the line after it unfolds the elongation to the full
0–360 circle — which is what makes `Waxing: d < 180` a real test rather than a guess.
`PA`, the Meeus phase angle, was an exported field until v4 removed it; it is now a
local, consumed one line later by the illumination formula and discarded. (The README's
API list has not caught up — see `readme-lists-a-phase-angle-that-v4-removed` in
`.issues/`.)

Moonrise and moonset need the Moon's position in the observer's sky, not on the
ecliptic, so two conversions from `epoch.go` come in here. The first applies full
nutation, both Δψ and Δε — unlike `solarDeclination`, which uses mean obliquity alone.
That asymmetry is deliberate: the sunrise path is a simplification whose budget is 1–2
minutes and does not earn the nutation terms back.

```bash
sed -n '161,185p' epoch.go
```

```output
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
		dec: dec,
	}
}
```

```bash
sed -n '187,216p' epoch.go
```

```output
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
```

Only `alt` is ever read. `az` is computed on every one of the ~1440 iterations of every
moonrise scan and thrown away — and its pole guard interacts with the west-correction
below it in a way that can produce 360 rather than 0
(`horizontal-azimuth-can-be-360-at-the-pole-guard` in `.issues/`).

With altitude available, `MoonriseMoonset` is a brute-force scan: walk the local day one
minute at a time and watch for the altitude crossing the refraction-corrected horizon.

```bash
sed -n '158,217p' lunar.go
```

```output
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
```

Compare the day construction here with `SunriseSunset`'s. This one builds midnight **in
`obs.loc`** and converts to UTC, and it computes tomorrow's local midnight the same way,
because it needs real instants to evaluate positions at — and it needs the day's real
*length*: `scanMinutes` is 1380 on a spring-forward day and 1500 on a fall-back day, not
a hard-coded 1440. The solar path wanted the opposite. Unify them and one of the two
breaks. Neither comment mentions the other, which is filed as
`two-day-constructions-read-as-a-contradiction-in-sequence` in `.issues/`.

Three more things worth noticing:

- The range check runs three times — the caller's instant and both derived midnights —
  because the timezone conversion can push a boundary date over the edge.
- The threshold is `-lunarHorizonDepression`, 0.833°, which is refraction (~0.566°) plus
  the Moon's mean semidiameter (~0.25°). The Moon is *seen* to rise while geometrically
  below the horizon, exactly as the Sun is.
- The loop `break`s once both events are found, so the 1440 evaluations are a worst
  case, not a cost every call pays. A day with neither event — the Moon up or down
  throughout — is the expensive one, and it is the case `AboveHorizon` exists to explain.

## The reference CLI

`cmd/dusk` is not a product; it is an executable specification. It calls all seven
exported functions and reaches every documented edge case from a single flag. `main`
does almost nothing so that every branch below it is reachable from a test.

```bash
sed -n '41,60p' cmd/dusk/main.go
```

```output
func main() {
	err := run(os.Args[1:], os.Stdout, os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dusk:", err)
		os.Exit(1)
	}
}

// run is the whole program. main only supplies the real streams and the exit
// status, which keeps every branch below reachable from a test.
//
// stdout is the data channel - the report, the version - and stderr is where
// the tool talks to whoever is driving it, so that `dusk ... --json | jq`
// pipes a record and not a complaint.
func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("dusk", flag.ContinueOnError)

	// The flag package writes its own error and usage block on a bad flag.
	// Discarded here so that a failure is reported once, by main, in the same
	// shape as every other failure.
```

```bash
sed -n '82,116p' cmd/dusk/main.go
```

```output
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

`parseDate` is a two-line function carrying a hard-won bug fix. It anchors the parsed
date at **midday** in the observer's zone rather than midnight, because a few zones —
`America/Santiago` in September, `America/Havana` in March — have no 00:00 on transition
days, and Go resolves the missing hour backwards into the previous day.

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

`buildReport` calls every public entry point in turn. The twilight assembly is the part
worth reading, because it is where the library's asymmetric `TwilightEvent` meets a
report that wants *this* morning's dawn.

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

`callTwilight` in between them separates three outcomes the library conflates into one
`error` return: a real event, an expected polar state, and a genuine failure. Only the
third aborts the report — polar geometry is a *result* here, and the command exits 0.

The renderer's job is to undo the library's grouping. `dusk` returns results grouped by
the call that produced them; a day is not lived in that order.

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

A zero time simply never enters the list, which is how "the Moon did not set today"
renders as an absence rather than as `00:00`. The `at.Day() != day` check exists because
a sunset at 00:03 belongs to the day *after* the one being reported, and sorts correctly
but reads as a mistake without the date said out loud.

## Running it

Grand Rapids on the June solstice — a full crossing day, everything present:

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

USNO gives sunrise 06:03 and sunset 21:25 EDT for that date and place, so the solar path
is inside its stated 1–2 minute budget.

Tromsø at the winter solstice — the sun never rises, and the whole polar vocabulary
comes out at once:

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

Every line of that is a specific branch. "The sun does not rise today" is
`ErrNeverRises` from `solarHourAngle` at depression 0. "Twilight still reaches civil
depth around midday" is `sunCondition` looking through the bands for one whose state is
still `stateCrosses` — without it, "the sun does not rise" would sit unexplained beside
a civil dawn. "The moon neither rises nor sets today" is the zero-time path with
`AboveHorizon` false. And the whole thing exits 0.

The JSON shape is the same data with the notes carried as fields. Time fields use
`omitzero`, not `omitempty` — a zero `time.Time` is a struct, and `omitempty` would emit
a useless `0001-01-01T00:00:00Z` rather than dropping it.

```bash
go run ./cmd/dusk --lat -1.2921 --lon 36.8219 --tz Africa/Nairobi --date 2025-03-20 --json
```

```output
{
  "lat": -1.2921,
  "lon": 36.8219,
  "zone": "Africa/Nairobi",
  "date": "2025-03-20",
  "sun": {
    "rise": "2025-03-20T06:36:52+03:00",
    "noon": "2025-03-20T12:40:12+03:00",
    "set": "2025-03-20T18:43:33+03:00",
    "daylight": "12h07m"
  },
  "twilight": [
    {
      "name": "Civil",
      "dawn": "2025-03-20T06:16:11+03:00",
      "dusk": "2025-03-20T19:04:14+03:00",
      "night": "11h12m"
    },
    {
      "name": "Nautical",
      "dawn": "2025-03-20T05:52:11+03:00",
      "dusk": "2025-03-20T19:28:14+03:00",
      "night": "10h24m"
    },
    {
      "name": "Astronomical",
      "dawn": "2025-03-20T05:28:10+03:00",
      "dusk": "2025-03-20T19:52:14+03:00",
      "night": "9h36m"
    }
  ],
  "moon": {
    "rise": "2025-03-20T23:11:00+03:00",
    "set": "2025-03-20T10:59:00+03:00",
    "aboveHorizon": true
  },
  "phase": {
    "name": "Waning Gibbous",
    "illumination": 69.77342282616303,
    "elongation": 246.8470248943793,
    "daysApprox": 20.24871745798808,
    "waxing": false
  }
}
```

Look at the moon block: `set` is 10:59 and `rise` is 23:11 — the set precedes the rise,
and `aboveHorizon` is true. That is not a bug and it is why the flag exists. A lunar day
runs about 24h50m, so the Moon that rose late yesterday evening was still up at local
midnight and set this morning before rising again tonight.

## How it is held together

There is no release automation and no coverage percentage. The gate is one task list,
and CI runs exactly it.

```bash
sed -n '39,50p' Taskfile.yml
```

```output

tasks:
  default:
    desc: The whole gate, in the order a failure is cheapest to read
    cmds:
      - task: tidy
      - task: vet
      - task: lint
      - task: nilaway
      - task: test
      - task: ratchet

```

Coverage is held by a **ratchet** rather than a threshold: the count of uncovered
statements per package is checked in, and the gate diffs the current counts against it.
It therefore fails in both directions — a number that rises is lost coverage, one that
falls is coverage to lock in — and on a package appearing or vanishing.

```bash
cat coverage.ratchet
```

```output
# Uncovered statements per package. The gate fails in both directions:
# a number that rises is lost coverage, one that falls is coverage to lock in.
# Regenerate with: task ratchet:update
github.com/philoserf/dusk/v4 10
github.com/philoserf/dusk/v4/cmd/dusk 19
```

An integer rather than a percentage because a percentage holds still while a guarded
branch adds one covered statement and one uncovered, and grows more forgiving as the
repository grows. The catch, documented in the `ratchet` task itself: blank lines split
coverage blocks, so any reflow moves these counts without changing what the tests reach.
Read the diff before assuming a regression.

Behind those two numbers, the count of test, fuzz, benchmark and example functions:

```bash
grep -rhE '^func (Test|Fuzz|Benchmark|Example)' --include='*_test.go' . | wc -l | tr -d ' '
```

```output
74
```

```bash
go build ./... && go vet ./... && go test ./... >/dev/null 2>&1 && echo 'build, vet and the suite all pass'
```

```output
build, vet and the suite all pass
```

---

## What this pass turned up

Tracing the code end to end raised two things a reader should not have to rediscover.
Both are filed in `.issues/` under `**Source:** code-walkthrough`.

The first is the day-boundary contrast above: `SunriseSunset` builds the calendar day as
UTC midnight and `MoonriseMoonset` builds it as true local midnight, both correctly and
for different reasons, and neither comment acknowledges the other. Read in call order
the second looks like a contradiction of the first, and it is the one place this
narrative had to stop and hold two things in view at once.

The second is structural. `epoch.go` holds two layers — the time helpers that everything
sits on, and the coordinate conversions that only the Moon's scan uses — and no reading
order keeps them together. The file says as much itself, in a banner comment reading
"moved from coord.go"; two sibling banners in `solar.go` and `lunar.go` record the same
consolidation.

Worth recording as a negative result: the previous `WALKTHROUGH.md` was rebuilt at
v4.0.0, and no `.go` file has changed since, so its narrative had not gone stale. The
drift found in this repository is in `README.md` and in a doc comment, not here.

### Index

| # | Severity | Issue | Primary location |
| --- | --- | --- | --- |
| 1 | medium | `two-day-constructions-read-as-a-contradiction-in-sequence` | `solar.go:43-45`, `lunar.go:169-173` |
| 2 | low | `epoch-go-holds-two-layers-that-no-reading-order-can-keep-together` | `epoch.go:109-111`, `epoch.go:157-159` |

**Total: 2 issues (0 critical, 0 high, 1 medium, 1 low)**

### Related existing findings

`code-theory` filed seven findings against this repository in the same run, and three of
them touch code this walkthrough steps through: `solarposition-has-no-production-caller`
(the continuous-time solar position, stranded when v3 unexported it),
`horizontal-azimuth-can-be-360-at-the-pole-guard` (the unread `az` field), and
`sun-and-moon-fuzz-targets-assert-nothing`. See `THEORY.md` for the full index.

