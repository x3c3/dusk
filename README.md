# dusk

[![CI](https://github.com/philoserf/dusk/actions/workflows/ci.yml/badge.svg)](https://github.com/philoserf/dusk/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/philoserf/dusk/v3.svg)](https://pkg.go.dev/github.com/philoserf/dusk/v3)

A single, zero-dependency Go package for astronomical calculations — sunrise/sunset, moonrise/moonset, twilight, and lunar phase — based on Meeus's _Astronomical Algorithms_.

## Install

```bash
go get github.com/philoserf/dusk/v3
```

## Command line

A reference implementation lives in `cmd/dusk`. It calls every exported function and
renders the day as one chronological list, so each documented edge case is reachable
from the command line.

The library returns its results grouped by the call that produced them — sun, three
twilight bands, moon — but a day is not lived in that order. Sorted by the clock, a
twilight table's dawn column stops running backwards, and a moonset belonging to the
previous night's rise stops appearing above the moonrise it precedes.

```bash
go install github.com/philoserf/dusk/v3/cmd/dusk@latest

dusk --lat 42.9634 --lon -85.6681 --tz America/Detroit --date 2025-06-21
dusk --lat 69.6492 --lon 18.9553 --tz Europe/Oslo --date 2025-12-21   # polar night
dusk --lat 69.6492 --lon 18.9553 --tz Europe/Oslo --date 2025-06-21   # midnight sun
dusk --lat -1.2921 --lon 36.8219 --tz Africa/Nairobi --json
dusk --version
```

`--lat`, `--lon`, and `--tz` are all required: a latitude of 0 is the equator rather
than "unset", and the zone decides which calendar day is meant. `--date` defaults to
today in that zone.

```text
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

Polar geometry is a result, not a failure: the report renders and exits 0. A misuse of
the flags and a date outside the library's range both exit 1, told apart by the message
rather than the status (`usage:` versus `unsupported date:`).

## Examples

### Sunrise and sunset

A complete program showing error handling and formatted output:

```go
package main

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/philoserf/dusk/v3"
)

func main() {
	// Grand Rapids, Michigan, which keeps Eastern time.
	loc, err := time.LoadLocation("America/Detroit")
	if err != nil {
		log.Fatal(err)
	}

	obs, err := dusk.NewObserver(42.9634, -85.6681, loc)
	if err != nil {
		log.Fatal(err)
	}

	date := time.Date(2025, 6, 21, 0, 0, 0, 0, loc)

	sun, err := dusk.SunriseSunset(date, obs)
	if err != nil {
		if errors.Is(err, dusk.ErrCircumpolar) {
			fmt.Println("Midnight sun — the sun does not set today.")
			return
		}
		if errors.Is(err, dusk.ErrNeverRises) {
			fmt.Println("Polar night — the sun does not rise today.")
			return
		}
		log.Fatal(err)
	}

	fmt.Printf("Sunrise:  %s\n", sun.Rise.Format(time.Kitchen))
	fmt.Printf("Noon:     %s\n", sun.Noon.Format(time.Kitchen))
	fmt.Printf("Sunset:   %s\n", sun.Set.Format(time.Kitchen))
	fmt.Printf("Daylight: %s\n", sun.Duration)
}
```

### Moonrise and moonset

The Moon may not rise or set on a given day. Use `IsZero()` to check, and `AboveHorizon` to determine whether the Moon was up at the start of the day:

```go
moon, err := dusk.MoonriseMoonset(date, obs)
if err != nil {
	log.Fatal(err)
}

switch {
case moon.Rise.IsZero() && moon.Set.IsZero():
	if moon.AboveHorizon {
		fmt.Println("Moon is above the horizon all day.")
	} else {
		fmt.Println("Moon is below the horizon all day.")
	}
case moon.Rise.IsZero():
	fmt.Println("Moon was already up at midnight.")
	fmt.Printf("Moonset:  %s\n", moon.Set.Format(time.Kitchen))
case moon.Set.IsZero():
	fmt.Printf("Moonrise: %s\n", moon.Rise.Format(time.Kitchen))
	fmt.Println("Moon stays up past midnight.")
default:
	fmt.Printf("Moonrise: %s\n", moon.Rise.Format(time.Kitchen))
	fmt.Printf("Moonset:  %s\n", moon.Set.Format(time.Kitchen))
}
```

### Lunar phase

All result types implement `fmt.Stringer`. Printing a `LunarPhaseInfo` value directly produces output like `Waxing Gibbous 67.3% (day 10.1)`:

```go
phase, err := dusk.LunarPhase(time.Date(2024, 1, 18, 3, 0, 0, 0, time.UTC))
if err != nil {
	log.Fatal(err)
}

fmt.Println(phase) // e.g., "Waxing Gibbous 67.3% (day 10.1)"
fmt.Printf("Illumination: %.1f%%  Waxing: %t\n", phase.Illumination, phase.Waxing)
```

### Civil twilight

Twilight functions return tonight's **Dusk** and tomorrow morning's **Dawn**. To get _this morning's_ dawn, call with yesterday's date:

```go
loc, err := time.LoadLocation("America/Los_Angeles")
if err != nil {
	log.Fatal(err)
}

obs, err := dusk.NewObserver(47.6062, -122.3321, loc)
if err != nil {
	log.Fatal(err)
}

date := time.Date(2025, 6, 21, 0, 0, 0, 0, loc)

tw, err := dusk.CivilTwilight(date, obs)
if err != nil {
	log.Fatal(err)
}

fmt.Printf("Dusk:           %s\n", tw.Dusk.Format(time.Kitchen))
fmt.Printf("Dawn:           %s\n", tw.Dawn.Format(time.Kitchen))
fmt.Printf("Night duration: %s\n", tw.NightDuration)
```

`NauticalTwilight` and `AstronomicalTwilight` follow the same signature.

### Polar error handling

At extreme latitudes, sunrise/sunset and twilight may be geometrically impossible. Use `errors.Is` to match the sentinel errors:

```go
loc, err := time.LoadLocation("Arctic/Longyearbyen")
if err != nil {
	log.Fatal(err)
}

obs, err := dusk.NewObserver(78.2, 15.6, loc) // Svalbard
if err != nil {
	log.Fatal(err)
}

midsummer := time.Date(2025, 6, 21, 0, 0, 0, 0, loc)

_, err = dusk.SunriseSunset(midsummer, obs)
if errors.Is(err, dusk.ErrCircumpolar) {
	fmt.Println("Midnight sun — no sunset at this latitude today.")
}
if errors.Is(err, dusk.ErrNeverRises) {
	fmt.Println("Polar night — no sunrise at this latitude today.")
}
```

## API

### Solar

- `SunriseSunset(date, obs)` — sunrise, solar noon, sunset, and daylight duration

### Lunar

- `MoonriseMoonset(date, obs)` — moonrise/moonset times and whether the Moon was above the horizon at the start of the day
- `LunarPhase(date)` — illumination, elongation, approximate age, waxing/waning, phase angle, and name

### Twilight

- `CivilTwilight(date, obs)` — sun 6 degrees below horizon
- `NauticalTwilight(date, obs)` — sun 12 degrees below horizon
- `AstronomicalTwilight(date, obs)` — sun 18 degrees below horizon

### Observer

- `NewObserver(lat, lon, loc)` — create a validated observer from latitude, longitude, and timezone

### Result types

All result types implement `fmt.Stringer`:

- `SunEvent` — `Rise`, `Noon`, `Set` times and `Duration` (daylight)
- `MoonEvent` — `Rise`, `Set` times and `AboveHorizon`
- `TwilightEvent` — `Dusk`, `Dawn` times and `NightDuration` (overnight darkness)
- `LunarPhaseInfo` — `Illumination`, `Elongation`, `Angle`, `DaysApprox`, `Waxing`, `Name`

### Errors

- `ErrCircumpolar` — object always above the horizon (e.g., midnight sun)
- `ErrNeverRises` — object never rises (e.g., polar night)
- `ErrNilLocation` — nil timezone passed to `NewObserver`
- `ErrNonFiniteCoord` — NaN or Inf coordinates
- `ErrInvalidCoord` — latitude or longitude out of range
- `ErrDateOutOfRange` — date outside supported Julian date range (~1677–2262)

## Conventions

- All angles are in **degrees**.
- The **calendar day is resolved in the observer's timezone** — functions convert the date with `date.In(observer location)` and ignore the time of day. Build the date with the observer's `*time.Location`, not `time.UTC`, or an observer west of Greenwich silently gets the previous day.
- Longitude is **east-positive, west-negative** (e.g., New York is -74.006).
- `Observer` is constructed via `NewObserver`, which validates coordinates and rejects NaN/Inf.
- Functions that can fail return `error`. Two sentinel errors distinguish polar edge cases: `ErrCircumpolar` and `ErrNeverRises`.
- A **zero-value `time.Time`** signals "event did not occur" (e.g., the Moon does not rise on a given day). Check with `.IsZero()`.
- Twilight functions return tonight's **Dusk** and tomorrow morning's **Dawn**. To get this morning's dawn, call with yesterday's date.

## Accuracy

Sunrise/sunset times are typically within 1-2 minutes of USNO data. Moonrise/moonset uses a simplified Meeus approach with a minute-by-minute altitude scan and can differ from USNO by up to ~20 minutes. Lunar phase illumination is within 1-2% of published values. Lunar ecliptic position uses the full Meeus Chapter 47 periodic terms (100+ coefficients).

## Requirements

Go 1.24+. Zero dependencies.

## License

GPL-3.0. See [LICENSE](./LICENSE).

Originally created by [observerly](https://github.com/observerly/dusk). This fork includes bug fixes, algorithm improvements, and a complete rewrite.
