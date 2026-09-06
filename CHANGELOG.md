# Changelog

## v4.0.0 — 2026-09-06

Adds `cmd/dusk`, and completes the API shrink that v3 began. The astronomy is
unchanged: every calculation returns exactly what v3.0.0 returned.

### Breaking changes

- **Module path** changed from `github.com/philoserf/dusk/v3` to `github.com/philoserf/dusk/v4`
- **Result types no longer implement `fmt.Stringer`** — `SunEvent`, `MoonEvent`, `TwilightEvent` and `LunarPhaseInfo` lose their `String()` methods. Nothing in the library or the reference CLI consumed them; `cmd/dusk` formats every field itself. Callers who printed a result value directly should format the fields they want. `Observer.String()` is unaffected.
- **`LunarPhaseInfo.Angle` removed** — the Meeus phase angle was published but unused, including by the reference CLI. `Illumination` is derived from it and is unchanged; callers needing the angle can recover it as `acos(2*Illumination/100 - 1)`, signed by `Waxing`.

### Added

- **`cmd/dusk`, a reference CLI.** Reports a full day — sun, three twilight bands, moon, phase — for one place and date, as text or JSON. It calls every exported function, and every documented edge case is reachable with a single flag.

  ```bash
  go install github.com/philoserf/dusk/v4/cmd/dusk@latest
  dusk --lat 42.9634 --lon -85.6681 --tz America/Detroit --date 2025-06-21
  ```

  Events are rendered in **clock order** rather than grouped by the call that produced them, so a twilight table's dawn column no longer runs backwards and a moonset belonging to the previous night's rise no longer appears above the moonrise it precedes. Polar geometry is a result, not a failure: the report renders and exits 0. Only a misused command line and an out-of-range date fail.

### Bug fixes

- **Twilight bands no longer report yesterday's geometry.** A band's state was seeded from yesterday's call and overwritten only when tonight also failed, so on a polar transition day the report could claim "civil, nautical and astronomical twilight never arrives" directly above a real civil dusk (65°N, 2025-07-28, civil dusk 01:05, civil night 48m).
- **`--date` no longer reports the previous day where clocks move at midnight.** `ParseInLocation` anchors at 00:00, which does not exist in `America/Santiago` in September or `America/Havana` in March, and Go resolves it backwards. Dates are now anchored at midday in the observer's zone. This also samples the lunar phase at local noon rather than local midnight.
- **A Moon above the horizon all day is no longer reported as absent.** `MoonriseMoonset` signals that with zero rise and set plus `AboveHorizon`, not with a sentinel; the text renderer consulted only the times and disagreed with the JSON beside it.
- Whole-day twilight claims are scoped to tonight, since this morning's dawn comes from a separate call and can be real when tonight's is not.

### Documentation

- `julianDate`'s doc comment claimed `UnixNano` returns 0 outside its range. It does not — the result is undefined and wraps to an arbitrary value. The old wording could invite a maintainer to skip a range check on a new call path (#54).
- `walkthrough.md` rebuilt against the current source and extended to cover `cmd/dusk`; every snippet is verified executable.
- `THEORY.md` extended with the reference implementation's theory.
- Fixed three README examples that built dates with `time.UTC` while passing a non-UTC observer — because the library derives the day via `date.In(obs.loc)`, the sunrise example computed June 20 while claiming the solstice.

### Internal

- `MoonriseMoonset` now calls `lunarPosition` instead of reimplementing it inline twice (#53).
- Coverage is held by a ratchet — uncovered statements per package, checked in, diffed both ways — replacing an 80% threshold that had permitted 46 uncovered statements to appear silently. CI runs exactly `task`.
- Go directive raised to 1.27; golangci-lint moved to `default: all`.
- Removed ~158 lines of tests that exercised the standard library rather than the astronomy, and inlined three single-use helpers (#58).
- `.golangci.yml`'s depguard allow-list and `coverage.ratchet`'s keys both name the module path; both were updated with the bump. Uncovered-statement counts are unchanged at 10 and 19.

## v3.0.0 — 2026-03-30

### Breaking changes

- **Module path** changed from `github.com/philoserf/dusk/v2` to `github.com/philoserf/dusk/v3`
- **`NewObserver` constructor** replaces direct struct construction — validates at creation, rejects NaN/Inf/nil
- **`Observer` fields unexported** — `Lat`/`Lon`/`Loc` → `lat`/`lon`/`loc`; use `NewObserver` to construct
- **Elevation removed** — `Observer.Elev` field deleted; elevation correction (~0.5' for typical altitudes) dropped from `solarHourAngle` for simplicity
- **`LunarPhase` returns `(LunarPhaseInfo, error)`** — now validates date range; callers must handle the error
- **`MoonEvent.Duration` removed** — was incorrect when Moon set before rise; callers should compute from Rise/Set as needed
- **Public API surface reduced** — removed `ObjectTransit`, `Transit`, `SolarPosition`, `LunarPosition`, `LunarEclipticPosition`, `EclipticToEquatorial`, `EquatorialToHorizontal`, `HourAngle`, `AngularSeparation`, `JulianDate`, `ValidJulianDateRange`, `LocalSiderealTime`
- **Coordinate types unexported** — `Equatorial`, `Horizontal`, `Ecliptic` → `equatorial`, `horizontal`, `ecliptic`
- **`TwilightEvent.Duration` renamed to `NightDuration`** — clarifies this is the overnight darkness period, not daylight
- **Sentinel errors are now constants** — `ErrCircumpolar`, `ErrNeverRises` use an unexported `errString` type; immutable, no longer reassignable
- **Newly exported errors** — `ErrNilLocation`, `ErrInvalidCoord` (were unexported in v2), `ErrNonFiniteCoord` (new, for NaN/Inf inputs); `ErrDateOutOfRange` remains exported but is now a constant

### Improvements

- `eclipticToEquatorial` now applies full nutation (both Δψ and Δε), improving RA accuracy by up to ~17"
- Observer validation (NaN/Inf rejection) happens once at construction, not repeated in every function call
- Date range validation (`validJulianDateRange`) at all public entry points
- `SunriseSunset` and `twilight` normalize input to UTC midnight — safe for any time-of-day
- `MoonEvent.AboveHorizon` field indicates whether the Moon was above the horizon at start of day
- `Observer` has `Lat()`, `Lon()`, `Location()` accessors and `String()` method
- Zero panics in library code

### File consolidation

10 source files → 5: `dusk.go`, `solar.go`, `lunar.go`, `epoch.go`, `trig.go`. Twilight merged into solar, lunar tables merged into lunar, coordinate conversions merged into epoch, types/stringers/errors consolidated into dusk.

## v2.2.0 — 2026-03-30

### Bug fixes

- `DaysApprox` now uses a linear elongation-to-days formula matching the "days into lunation" documentation (#36)
- `solarHourAngle` no longer applies elevation correction to twilight calculations, per USNO convention (#39)
- Example tests handle errors from `time.LoadLocation` and computation functions instead of discarding (#43)

### Testing

- Fuzz tests assert error returns for out-of-range coordinates instead of silently skipping (#37)
- `TestLunarPhase` now asserts on `Angle` and `Elongation` ranges for all 8 phase test cases (#41)

### CI

- Enforce `gofumpt` formatting via `golangci-lint` formatters config (#38)

### Documentation

- `HourAngle` doc comment clarifies mixed-unit parameters (RA in degrees, LST in hours) (#40)
- `MoonriseMoonset` doc comment notes ~1-2 ms wall-clock cost per call (#42)

## v2.1.0 — 2026-03-10

### New features

- `fmt.Stringer` interface on all 8 exported result types (`SunEvent`, `MoonEvent`, `LunarPhaseInfo`, `Transit`, `TwilightEvent`, `Equatorial`, `Ecliptic`, `Horizontal`)
- `ValidJulianDateRange` helper and `ErrDateOutOfRange` sentinel for guarding `JulianDate` range (~1677–2262)

### Bug fixes

- `asinx`/`acosx` now clamp inputs to [-1, 1] via `clamp()`, preventing NaN from floating-point rounding
- `validateEquatorial` normalizes RA via `mod360` instead of rejecting values at 360.0; rejects NaN/Inf inputs

### Performance

- `ObjectTransit` transit maximum: replaced O(n) minute-by-minute scan with O(1) analytical solution (hour angle = 0)

### Refactoring

- Extracted `computeSolarParams` helper eliminating 3× duplication of the 6-step solar parameter sequence; `twilight()` reduced from 44 to 29 lines
- Renamed shadowed variable `F` → `frac` in `LunarPhase`

### Testing

- Fuzz tests for `SunriseSunset`, `LunarPhase`, `ObjectTransit`, `MoonriseMoonset`
- Benchmarks for `MoonriseMoonset`, `LunarEclipticPosition`, `SunriseSunset`, `ObjectTransit` (0 allocations)
- Polar twilight transition test (75°N, Nov 25→26)
- Negative elevation clamping, summer solstice solar position, GMST/julianCentury helper tests
- Southern hemisphere moonrise/moonset regression references
- Consistent `t.Run` subtests for `TestMod360`/`TestMod24`; table-driven `TestEclipticToEquatorial`

### Infrastructure

- `.golangci.yml` with explicit linter list (gocritic, revive, misspell, etc.); `captLocal` disabled for Meeus conventions
- 80% coverage threshold in CI (currently 99.7%); removed duplicate `go vet` step

### Documentation

- `Observer.Elev` doc comment: negative value clamping, scope (sunrise/sunset/twilight only)
- `gstToUT` precision note about J1900-epoch model accuracy

## v2.0.0 — 2026-03-06

Complete rewrite of the library. Zero external dependencies.

### Breaking changes

- **Module path** changed from `github.com/philoserf/dusk` to `github.com/philoserf/dusk/v2`
- **Removed `timezonemapper` dependency** — callers pass `*time.Location` explicitly via the new `Observer` struct
- **Renamed all exported functions** — `Get` prefix removed (e.g., `GetJulianDate` → `JulianDate`, `GetLocalSiderealTime` → `LocalSiderealTime`)
- **Replaced coordinate types** — `Coordinate`, `EquatorialCoordinate`, `EclipticCoordinate`, `HorizontalCoordinate` replaced by `Equatorial`, `Ecliptic`, `Horizontal` with named fields (`RA`/`Dec`, `Lon`/`Lat`, `Alt`/`Az`)
- **New `Observer` struct** — replaces separate `latitude`, `longitude` parameters; includes `Loc *time.Location` and `Elev float64`
- **New event structs** — `SunEvent`, `MoonEvent`, `TwilightEvent`, `Transit` replace raw time returns and ad-hoc structs
- **Removed exports** — `JulianPeriod`, `TemporalHorizontalCoordinate`, `TransitHorizontalCoordinate`, `GetEarthObliquity`, `GetUniversalTime`, `GetCurrentJulianPeriod`, `GetMeanSolarTime`, `GetDatetimeZeroHour`, `ConvertLocalSiderealTimeToGreenwichSiderealTime`, `ConvertGreenwichSiderealTimeToUniversalTime`, `GetAtmosphericRefraction`, `GetRelativeAirMass`, `GetApparentAltitude`, `GetArgumentOfLocalSiderealTimeForTransit`, `SunriseStatus`, `AboveHorizon`, `AtHorizon`, `BelowHorizon`, and others
- **Removed files** — `astrometry.go`, `coordinates.go`, `trigonometry.go`, `utils.go`, `lawrence.go` consolidated into domain-focused files

### New features

- `LunarPhase` — illumination, age in days, waxing/waning, and phase name
- `LunarEclipticPosition` — Meeus Chapter 47 ecliptic position with full periodic terms
- `MoonriseMoonset` — moonrise and moonset times via minute-by-minute altitude scan
- `ObjectTransit` — rise, set, and transit maximum for arbitrary equatorial coordinates
- `CivilTwilight`, `NauticalTwilight`, `AstronomicalTwilight` — twilight dusk/dawn pairs
- `ErrCircumpolar` and `ErrNeverRises` sentinel errors for polar edge cases
- `AngularSeparation` — robust atan2-based formula

### Improvements

- Meeus algorithms throughout (solar mean anomaly, lunar ecliptic, nutation, obliquity)
- Degree-based trig helpers (`sinx`, `cosx`, etc.) eliminate manual conversion
- `mod360`/`mod24` normalization helpers
- Zero-value `time.Time` convention for events that don't occur (circumpolar, never rises)
- High test coverage with table-driven tests and values from USNO, Stellarium, and Meeus
- Edge-case tests for equatorial, polar, and southern hemisphere observers

## v1.0.0

Initial release. Forked from [observerly/dusk](https://github.com/observerly/dusk).
