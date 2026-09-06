# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

# Dusk

Go library for astronomical calculations: twilight, lunar phase, rise/set times.

Onboarding references: `THEORY.md` (Naur-style theory of the codebase) and `walkthrough.md` (linear code tour).

## Commands

```bash
task                 # The whole gate: tidy, vet, lint, nilaway, test, ratchet
task fix             # Every autofix golangci-lint offers (deliberately not in the gate)
task test            # The suite with -race, writing coverage.out
task ratchet:update  # Re-record uncovered counts after a deliberate coverage change
task build           # Build the reference CLI to bin/dusk
task run -- --lat 69.6492 --lon 18.9553 --tz Europe/Oslo
task fuzz            # The fuzzing engine, 10s per target (not in the gate)
task bench           # Benchmarks (benchmark_test.go)

go test -v -run TestName .           # Run a single test
```

**CI runs exactly `task`.** Never add a check to CI that the local gate does not run,
and never add a tool to the gate without also installing it in the workflow. The Go
toolchain and golangci-lint are deliberately unpinned: a red gate on untouched code is
the signal working — answer the finding rather than pinning the tool.

Coverage is held by a **ratchet**, not a percentage: `coverage.ratchet` records the
count of uncovered statements per package, and `task ratchet` diffs the current counts
against it — so the gate fails in both directions and on a package appearing or
vanishing. A number that rises is lost coverage; one that falls is coverage to lock in
with `task ratchet:update`.

An integer rather than a percentage because a percentage holds still while a guarded
branch adds one covered statement and one uncovered, and it grows more forgiving as the
repository grows. The whole check is an awk program in `Taskfile.yml`; there is no tool
to maintain.

Reflowing blank lines splits coverage blocks, so a refactor can move these counts without
changing what the tests reach — read the diff before assuming a regression.

## Architecture

Library is a single package at the repo root, with a reference CLI under `cmd/dusk`. Zero dependencies. Module path: `github.com/philoserf/dusk/v4`.

| File       | Domain                                                                                     |
| ---------- | ------------------------------------------------------------------------------------------ |
| `dusk.go`  | Package doc, `Observer`/`NewObserver`, event types, sentinel errors                        |
| `solar.go` | `SunriseSunset`, civil/nautical/astronomical twilight, all unexported solar helpers        |
| `lunar.go` | `MoonriseMoonset`, `LunarPhase`, unexported lunar helpers, Meeus Table 47.A/B coefficients |
| `epoch.go` | Julian dates, sidereal time, nutation, obliquity, coordinate conversions (all unexported)  |
| `trig.go`  | Degree-based trig wrappers, `clamp`, `mod360`/`mod24` normalization                        |
| `cmd/dusk/` | Reference CLI over the public API: `main.go` (flags, errors), `report.go` (assembly), `render.go` (text/JSON) |

## Key Conventions

- All angles in **degrees** (trig helpers in `trig.go` handle conversion)
- Angle normalization via `mod360()` and `mod24()` helpers
- Meeus algorithms preferred; `solarMeanAnomaly(J)` takes days, not centuries
- `Observer` constructed via `NewObserver` — validates once at creation, fields unexported
- **Callers must build dates in the observer's timezone.** Public entry points resolve the
  calendar day with `date.In(obs.loc)` and ignore time-of-day, so a `time.UTC` midnight
  selects the previous day for any observer west of Greenwich
- Zero-value `time.Time` signals "event did not occur" — check with `.IsZero()`
- `ErrCircumpolar` / `ErrNeverRises` for geometrically impossible events (polar)
- `error` returns for date out of range (validated at all public entry points)
- Table-driven tests everywhere, expected values from USNO/Stellarium/Meeus
- **Errors are checked on their own line**, never inline: `err := f()` then `if err != nil`,
  not `if err := f(); err != nil`. Enforced by `noinlineerr`, matching the other Go repos
- **Every test calls `t.Parallel()`**, top level and subtest. Enforced by `paralleltest`.
  Accumulating state across parallel subtests is a race — check table-wide properties in
  their own sequential test instead

## Gotchas

- Moonrise/moonset iterates minute-by-minute (1440 iterations) — slow by design
- `solarHourAngle` returns `(float64, error)` — returns `ErrCircumpolar` (midnight sun) or `ErrNeverRises` (polar night)
- `solarHourAngle` takes `depression` (positive degrees below horizon) for twilight reuse; pass 0 for sunrise/sunset
- `LunarPhaseInfo.Waxing` distinguishes waxing (elongation 0-180) from waning; `DaysApprox` is a linear approximation
- `eclipticToEquatorial` applies full nutation (Δψ + Δε); `solarDeclination` uses mean obliquity only (intentional asymmetry — NOAA simplified method for sunrise/sunset)

## Dependencies

None. Zero external dependencies.

## CI

One GitHub Actions job that installs the toolchain and runs `task`. No paths-ignore: a
gate that skips a documentation-only push cannot catch a broken example, and the examples
here compile.
