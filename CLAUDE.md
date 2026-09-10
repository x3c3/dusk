# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

# Dusk

Go library for astronomical calculations: twilight, lunar phase, rise/set times.

Onboarding references: `THEORY.md` (Naur-style theory of the codebase) and `WALKTHROUGH.md` (linear code tour).
`THEORY.md` is hand-maintained prose — extend it when a load-bearing idea changes. `WALKTHROUGH.md` is a
showboat document whose snippets are verified executable; keep them runnable, and re-verify before tagging.

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
task setup           # Install the toolchain: Brewfile, plus `go install` for nilaway
task deps:check      # Verify the toolchain is installed
task clean           # Remove bin/ and coverage artifacts

FUZZTIME=2m task fuzz                # Longer fuzzing run
go test -v -run TestName .           # Run a single library test
go test -v -run TestName ./cmd/dusk  # Run a single CLI test
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

Library is a single package at the repo root, with a reference CLI under `cmd/dusk`. Zero external
dependencies — nothing outside the standard library, and the Meeus coefficient tables are transcribed
into the source rather than fetched. Module path: `github.com/philoserf/dusk/v4`.

| File       | Domain                                                                                     |
| ---------- | ------------------------------------------------------------------------------------------ |
| `dusk.go`  | Package doc, `Observer`/`NewObserver`, event types, `stringError`, all sentinels but one    |
| `solar.go` | `SunriseSunset`, civil/nautical/astronomical twilight, all unexported solar helpers        |
| `lunar.go` | `MoonriseMoonset`, `LunarPhase`, unexported lunar helpers, Meeus Table 47.A/B coefficients |
| `epoch.go` | Julian dates, sidereal time, nutation, obliquity, coordinate conversions — unexported apart from `ErrDateOutOfRange`, which lives beside the range check that returns it |
| `trig.go`  | Degree-based trig wrappers, `clamp`, `mod360`/`mod24` normalization                        |
| `cmd/dusk/` | Reference CLI over the public API: `main.go` (flags, errors), `report.go` (assembly), `render.go` (text/JSON) |

`cmd/dusk` is an executable specification, not a product: it calls every exported function, and every
documented edge case is reachable with a single flag. Its exit contract is part of that — polar geometry
is a result, so the report renders and exits 0; only a misused command line (`usage:`) and an
out-of-range date (`unsupported date:`) exit 1, told apart by the message rather than the status.

## Key Conventions

- All angles in **degrees** (trig helpers in `trig.go` handle conversion)
- Angle normalization via `mod360()` and `mod24()` helpers
- Longitude is **east-positive, west-negative** (New York is -74.006)
- Meeus algorithms preferred; `solarMeanAnomaly(J)` takes days, not centuries
- `Observer` constructed via `NewObserver` — validates once at creation, fields unexported
- **Callers must build dates in the observer's timezone.** Public entry points resolve the
  calendar day with `date.In(obs.loc)` and ignore time-of-day, so a `time.UTC` midnight
  selects the previous day for any observer west of Greenwich
- `LunarPhase` is the exception to the day regime: it takes an **instant**, not a day, and no
  `Observer` — phase is Sun-Earth-Moon geometry, so the observer is irrelevant
- Twilight functions return tonight's `Dusk` and **tomorrow morning's** `Dawn`. To get this
  morning's dawn, call with yesterday's date
- Zero-value `time.Time` signals "event did not occur" — check with `.IsZero()`
- `ErrCircumpolar` / `ErrNeverRises` for geometrically impossible events (polar)
- `error` returns for date out of range (validated at all public entry points)
- **Sentinel errors are `const`, not `var`** — declared as the unexported `stringError` string type
  in `dusk.go` so they cannot be reassigned. New sentinels follow that pattern, not `errors.New`
- Table-driven tests everywhere, expected values from USNO/Stellarium/Meeus. Tolerances that
  reference data actually supports: **1-2 minutes** for sunrise/sunset, up to **~20 minutes** for
  moonrise/moonset (simplified Meeus with a minute-by-minute scan), **1-2%** for lunar illumination
- **Errors are checked on their own line**, never inline: `err := f()` then `if err != nil`,
  not `if err := f(); err != nil`. Enforced by `noinlineerr`, matching the other Go repos
- **Every test calls `t.Parallel()`**, top level and subtest. Enforced by `paralleltest`.
  Accumulating state across parallel subtests is a race — check table-wide properties in
  their own sequential test instead

## Lint posture

`.golangci.yml` runs `default: all` and disables only what fights this repo's deliberate design.
Each disable carries a **measured finding count and a reason** — the file's own rule, and the bar for
adding another: count the findings, read them, and write down why they are wrong here.

- `nolintlint` requires a **specific** linter and an **explanation**, and fails on an unused
  `//nolint` — a blanket directive will not pass the gate
- `depguard` is `list-mode: strict`, allowing only `$gostd` and this module's own path
- `_test.go` relaxes the linters that table-driven, white-box tests trip (`dupl`, `goconst`,
  `funlen`, `gocognit`, `cyclop`, `lll`, `gosec`, `noctx`, `testpackage`, `gochecknoglobals`)
- Terse Meeus notation (`T`, `M`, `Lp`, `Mp`, `h0`) is permitted by name in the `varnamelen`
  ignore list and by disabling `gocritic`'s `captLocal`; a new one-letter name needs an entry
- gofumpt and goimports run **inside** golangci-lint, which is the single definition of formatted
  this repo has. A `PostToolUse` hook in `.claude/settings.json` also runs `gofumpt -w` on Go
  file writes — but the Brewfile does not install gofumpt, so on a fresh machine that hook fails
  rather than formats (exit 127), and `task lint` is what catches the formatting

## Gotchas

- Moonrise/moonset iterates minute-by-minute (1440 iterations) — slow by design
- `solarHourAngle` returns `(float64, error)` — returns `ErrCircumpolar` (midnight sun) or `ErrNeverRises` (polar night)
- `solarHourAngle` takes `depression` (positive degrees below horizon) for twilight reuse; pass 0 for sunrise/sunset
- `LunarPhaseInfo.Waxing` distinguishes waxing (elongation 0-180) from waning; `DaysApprox` is a linear approximation
- `eclipticToEquatorial` applies full nutation (Δψ + Δε); `solarDeclination` uses mean obliquity only (intentional asymmetry — NOAA simplified method for sunrise/sunset)

## Major version bumps

The `/vN` in the module path is load-bearing in two config files that a `go mod edit` will not touch,
and both turn the gate red until they are updated by hand:

- `.golangci.yml` — the `depguard` allow-list names `github.com/philoserf/dusk/v4`. Under
  `list-mode: strict` the new path is not allowed, so every internal import is reported as forbidden
  (measured: 5 findings across `cmd/dusk` and `example_test.go`) — loudly, naming each import
- `coverage.ratchet` — the keys are full import paths. The ratchet reads the old packages as vanished
  and the new ones as appeared, so it fails even when coverage is unchanged; re-record with
  `task ratchet:update` once the path is right

Also update the imports in `cmd/dusk` and `example_test.go`, this file's Architecture line, and the
README's badge, `go get`, and `go install` lines.

## Releases

No release automation — the repo has exactly two workflows, `ci.yml` (the gate) and `claude.yml`.
`CHANGELOG.md` is written by hand. **Tag last**, by hand, after the gate is green and `WALKTHROUGH.md`
has been re-verified against the current source.

## CI

One GitHub Actions job that installs the toolchain and runs `task`. No paths-ignore: a
gate that skips a documentation-only push cannot catch a broken example, and the examples
here compile.
