# THEORY.md

A Naur-style theory of `dusk`: not what the files contain, but what you have to be
holding in your head to change them without breaking something that currently works.

## What this models

The Earth turns, and an observer standing on it watches two bodies cross a horizon.
That is the entire domain. Everything in the package is an answer to one of three
questions about a particular place on a particular day: when does the Sun cross a given
depth below the horizon, when does the Moon cross the horizon, and how much of the Moon
is lit.

The vocabulary is Meeus's, taken from _Astronomical Algorithms_ and used as if its
meanings were settled, because within this package they are. A **depression angle** is
degrees below the geometric horizon, positive downward; the three twilight bands are
just that number fixed at 6, 12 and 18. **Elongation** is the Sun-Moon angle seen from
Earth, running 0 to 360 with waxing on the first half. **Hour angle** is how far west of
the meridian a body has travelled, in degrees. The **Julian date** is the continuous day
count everything is computed in, and **J2000** — Julian date 2451545.0, noon UT on 1
January 2000 — is the zero the polynomials are written around. When you see a bare `T`
it is Julian centuries since J2000; a bare `J` is days since J2000. Those two are not
interchangeable and the package has been bitten by their confusion before: `solar.go`
carries both `solarMeanAnomaly(J)` and `solarMeanAnomalyFromCentury(T)`, computing the
same physical quantity from different units, because the solar path has days in hand and
the lunar path has centuries.

The core entity is the `Observer`: a latitude, a longitude, and a `*time.Location`. It is
not a container of three numbers, it is a **validated** container of three numbers, and
that distinction is the package's smallest and most consequential design decision. See
"Validate once" below.

The four result types — `SunEvent`, `MoonEvent`, `TwilightEvent`, `LunarPhaseInfo` — are
deliberately inert. They have no methods (v4 removed the `String()` methods they briefly
had), no computed accessors, and no behaviour. They are the boundary at which this
package stops having opinions.

## The organizing ideas

### Degrees, all the way down, and one place that converts

Meeus's formulae are written in degrees. Go's `math` is written in radians. Rather than
convert at each call site, `trig.go` wraps the six trig functions the package needs so
that every angle in every other file is in degrees — `sinx`, `cosx`, `tanx`, `asinx`,
`acosx`, `atan2x`, plus `sincosx` for the coefficient loops that need both. There is no
radian anywhere outside `trig.go`. This is why the coefficient tables can be transcribed
from the book without a conversion pass and checked against the printed page, and it is
why introducing a raw `math.Sin` into `lunar.go` would be a silent, catastrophic bug
rather than a compile error. Keep the discipline absolute.

`mod360` and `mod24` are the other half of the same idea: an angle that has been added
to or subtracted from is normalised at the point it becomes a result, not left to a
consumer. Almost every helper in `epoch.go` and `lunar.go` returns through one of them.
The one place this discipline lapses is azimuth (see the seam below).

`clamp` is the third piece and the most interesting, because its own comment argues
against it:

> Note: it also silently clamps genuinely wrong values (e.g., a miscalculated 1.3 → 1),
> which could mask upstream bugs. Correctness is validated by test coverage against
> Meeus and USNO reference data rather than runtime detection.

That is a deliberate trade and it names its own cost. `asin` and `acos` receive values
that ought to lie in [-1, 1] but arrive there through long chains of degree-mode trig,
and rounding puts them at 1.0000000000000002 often enough to matter. Clamping turns a
NaN that would propagate through the whole result into a correct answer. The price is
that a real error of the same shape is also absorbed. The package pays it, and pays for
it with reference-data tests — which is why weakening those tests is more dangerous here
than the coverage number alone suggests.

### Validate once, at construction

`NewObserver` is the only way to get a usable `Observer`: it rejects a nil location,
NaN and Inf coordinates, and out-of-range latitude or longitude, and its fields are
unexported so the checks cannot be bypassed. Every public entry point then calls
`validObserver`, which tests one thing — is `loc` nil — because a zero-value `Observer`
is the only invalid one the type system still permits. That is the whole invariant:
**an `Observer` with a non-nil location has already been fully validated**. It is why
`SunriseSunset` does not re-check for NaN, why the benchmark file can build one at
package scope and discard the error, and why the fuzz targets can treat a
`NewObserver` failure as "not a case worth exploring" and return.

The parallel invariant on the time axis is `validJulianDateRange`. `julianDate` computes
through `UnixNano`, which is undefined outside roughly 1677-2262 — and, as its comment
is careful to say since issue #54, undefined means *an arbitrary wrong number*, not zero
and not a sentinel. So every public entry point range-checks before computing, and
`MoonriseMoonset` checks three times: the caller's instant, and both derived local
midnights, because the conversion can push a boundary date over the edge.

### A day is resolved in the observer's zone — and then two things happen to it

This is the subtlety that costs the most when it is missed, and the reference CLI
comments on it twice because of that. All the day-based entry points begin the same way:

```go
localDate := date.In(obs.loc)
```

The time of day is then discarded. Only the calendar date survives, and which calendar
date that is depends on the observer's zone — so a caller who builds `time.Date(...,
time.UTC)` and hands it to an observer in Detroit gets the previous day, silently, with
entirely plausible times. `cmd/dusk` anchors its parsed `--date` at **midday** in the
observer's zone rather than midnight for a second-order version of the same trap: a few
zones (`America/Santiago` in September, `America/Havana` in March) have no 00:00 on
transition days, and Go resolves the missing hour backwards into the previous day.

What happens *after* the calendar date is extracted is where the two halves of the
package part company, and the split is principled rather than accidental:

- **The solar path** rebuilds the day as UTC midnight:
  `time.Date(y, m, d, 0, 0, 0, 0, time.UTC)`. It does not want the observer's offset.
  The NOAA method it implements takes an integer day number (`julianDay` rounds
  `JD - J2000` to the nearest integer) and applies the longitude correction itself, in
  `meanSolarTime`, as `n - lon/360`. Handing it a true local midnight would apply the
  observer's longitude twice, once through the zone and once through the formula.
- **The lunar path** rebuilds the day as true local midnight in `obs.loc` and converts
  to UTC, and computes the next local midnight the same way. It needs real instants,
  because it is going to walk the day one minute at a time and ask for the Moon's
  altitude at each. It also needs the real *length* of the day: the scan runs
  `int(nextMidnight.Sub(d).Minutes())` iterations, which is 1380 on a spring-forward
  day and 1500 on a fall-back day, not a hard-coded 1440.

If you unify those two, you break one of them. The comment in
`greenwichMeanSiderealTime` — "Do not 'simplify' by passing t here" — is the same
warning about the same class of mistake one layer down.

### Two vocabularies for "this did not happen"

The package distinguishes an event that is *geometrically impossible* from one that
merely *did not fall inside this calendar day*, and it uses different mechanisms for
each, on purpose.

**Sentinel errors** say the geometry forbids it. `solarHourAngle` computes the cosine of
the hour angle and, when it falls outside [-1, 1], returns `ErrCircumpolar` (the Sun
never gets down to the angle) or `ErrNeverRises` (never gets up to it). At a depression
angle these mean something a reader will get backwards on first contact, which is why
`cmd/dusk/report.go` writes it down: at 18° below the horizon, "circumpolar" means the
night never gets that dark, and "never rises" means the day never gets that light.

**A zero `time.Time`** says the event did not occur today. `MoonriseMoonset` never
returns a polar sentinel — a lunar day is about 24h50m, so the Moon routinely rises
without setting before local midnight at any latitude at all, and that is not an error
anywhere. It returns a zero `Rise` or `Set`, plus `AboveHorizon` to say which side of the
horizon the Moon started the day on. Without that flag, "no rise and no set" is
ambiguous between up-all-day and down-all-day, and `cmd/dusk`'s `moonCondition` needs
exactly that distinction to choose between "stays above the horizon all day" and
"neither rises nor sets today".

### `TwilightEvent` is asymmetric, and every consumer pays for it

`CivilTwilight(date, obs)` returns **tonight's** dusk and **tomorrow morning's** dawn.
It is not a symmetric bracket around the night you asked about; it is the night that
*starts* on the date you asked about. To get this morning's dawn you call with
yesterday's date.

The implementation makes this unavoidable rather than incidental: `twilight` computes
solar parameters twice, once for `date` and once for `date.AddDate(0, 0, 1)`, and
returns an error if *either* day's hour angle is impossible. Near 65-70°N there are
transition dates where tonight's dusk is real and tomorrow's dawn is not, and the whole
call fails.

`cmd/dusk` is the worked example of living with this, and its comments are the clearest
statement of the contract anywhere in the repository: each band is computed twice, dawn
taken from yesterday's call and dusk from today's, with yesterday's *state* deliberately
discarded because it describes a night the report is not about. The v4 changelog records
what happened when it was not discarded — a polar transition day printed "twilight never
arrives" directly above a real civil dusk time.

### `LunarPhase` is the exception to every rule above

It takes no `Observer`, because the Moon shows the same face to the whole Earth. It uses
the exact instant rather than the calendar day, because phase changes continuously. It
is the only public function whose answer does not depend on where you are standing. When
you are reasoning about "what do all the entry points do", `LunarPhase` is not one of
them.

## The seams

**Between the package and the world**: `NewObserver` and the six exported sentinel
errors. That is the whole surface for failure. Callers are expected to use `errors.Is`,
which the sentinels support by being constants of an unexported `stringError` string
type — immutable, unlike anything from `errors.New`.

**Between the two halves of the astronomy**: `epoch.go`. Julian dates, sidereal time,
nutation, obliquity, and the two coordinate conversions are the shared floor that
`solar.go` and `lunar.go` both stand on. The seam is real but not symmetric, and the
asymmetry is the intentional kind: `eclipticToEquatorial` applies full nutation, both
Δψ and Δε, while `solarDeclination` uses mean obliquity alone. That is not an oversight
to be tidied. The sunrise path is the NOAA simplified method, whose accuracy budget is
1-2 minutes and which does not earn back the nutation terms; the lunar path is the full
Chapter 47 series and does.

**Between the library and its reference consumer**: `cmd/dusk` exists to be an
executable specification. It calls every exported function, reaches every documented
edge case from a single flag, and — this is the part that matters when you change it —
re-sorts everything into clock order, because the library's grouping by call is not the
order a day is lived in. It is also the only place the library's harder contract points
are written down as running code rather than prose.

**Where the theory is thinnest**: the horizontal coordinate conversion.
`equatorialToHorizontal` returns both altitude and azimuth, but nothing in the package
reads azimuth — `MoonriseMoonset` takes `.alt` at both call sites and the field is
touched only by its own tests. Azimuth is also the one angle not routed through
`mod360`, and the zero-division guard at the poles interacts with the west-correction
below it to produce 360 rather than 0. It is a general-purpose conversion in a package
that otherwise contains nothing general-purpose, and it is half-dead. `solarPosition` is
the same shape: a complete, tested, continuous-time solar position with no caller left
in the library, stranded when v3 unexported `SolarPosition`.

## What this is shaped to accommodate

**A new twilight band** costs one line. `twilight(date, obs, depression)` is fully
parameterised; the three exported wrappers are the only thing fixing 6, 12 and 18. A
Danish "blue hour" at 4° or an aviation band would slot straight in, plus an entry in
`cmd/dusk`'s `twilightBands` table.

**Better lunar coefficients** cost a table edit. `tableLongDist` and `tableLat` are
transcribed Meeus 47.A and 47.B, read by two loops that switch on the `M` column to
apply the eccentricity factor `E` to the right powers. Adding terms is additive and
local. Note that there is no validation of the tables at all — an earlier attempt at
`init()`-time checks was added and then removed (`8a240d9`, then `57e4f04`). Their
correctness rests entirely on the reference-value tests in `lunar_test.go`.

**A new consumer of the library** costs nothing, which is the point of the inert result
types.

What would require rethinking:

**Sub-minute moonrise accuracy, or making it fast.** The minute-by-minute scan is not an
implementation detail that can be optimised behind the same result; it *is* the
algorithm, and its resolution is its accuracy. A day where the Moon grazes the horizon
for under a minute is invisible to it. Replacing it with interpolation between three
positions (Meeus ch. 15, the standard approach) would change every moonrise time in the
test suite by minutes, which is exactly why nobody has.

**Partial twilight results at polar transition latitudes.** The all-or-nothing error
from `twilight` is baked into its shape: one call, one error, two days of geometry. The
doc comment already tells callers to compute each boundary separately if they need
partial results, which is an admission that the type is wrong for that case rather than
a workaround.

**Elevation above sea level.** It was there in v2 and was deliberately removed in v3.
Putting it back means a fourth `Observer` field and a term in `solarHourAngle`, and
every reference value in `solar_test.go` was recorded without it.

**Dates outside 1677-2262.** The bound is `UnixNano`'s, inherited from representing
Julian dates through `time.Time`. Escaping it means not going through `time.Time` at
all in `julianDate`, and every result type is a `time.Time`.

Where a maintainer who did not hold this theory would do damage, in order of likelihood:
"simplifying" the solar and lunar day-extraction into one shared helper; making
`TwilightEvent` symmetric because the asymmetry looks like a bug; deleting `clamp`
because it hides errors; passing `t` instead of midnight into `julianCentury` inside
`greenwichMeanSiderealTime`; and reflowing `lunar.go` for style, which will move the
coverage ratchet without changing a single thing the tests reach.

## Uncertainties

Marked plainly, because these are inferences from code rather than recovered intent.

- **The missing 0.0009-day term.** The published NOAA/Meeus sunrise equation carries a
  `+ 0.0009` fractional-day constant in mean solar time; `meanSolarTime` does not, and
  `julianDay` rounds rather than taking a ceiling. 0.0009 days is 78 seconds. I could not
  determine whether the rounding was chosen to absorb it or whether the term was simply
  dropped. Results match USNO within the documented 1-2 minutes either way, so nothing
  observable is at stake — but a maintainer "restoring" the term should know the rounding
  is doing part of its job.
- **Whether `solarPosition` is meant to be used.** Its doc comment argues for a design
  distinction (continuous versus rounded Julian days) that the package no longer acts
  on. I read it as v3 residue; it could be a deliberate placeholder for a solar-altitude
  feature. Filed as a finding either way, because the comment currently misleads.
- **`lunarPhaseName`'s parameter is called `age` and receives an elongation.** Inside, it
  is immediately `mod360`'d and compared against degree boundaries, so it is
  unambiguously an angle. `DaysApprox` — the actual age — is derived separately as a
  linear scaling of the same elongation. I take `age` to be a leftover from an earlier
  formulation rather than a claim, but I cannot rule out that the two were once the same
  parameter.
- **The `E`/`E2` switch in the coefficient loops.** `switch r.M { case 0 / 1,-1 / 2,-2 }`
  has no default, so a table row with `|M| > 2` would contribute nothing and fail
  silently. Meeus 47.A contains no such row and neither does the transcription, so this
  is correct today. Whether the absence of a default is a considered decision or an
  accident of the table's contents, I cannot tell.
- **The scope of `AboveHorizon`.** It is computed from the altitude at the first scanned
  instant, compared against `-lunarHorizonDepression` rather than against 0 — consistent
  with the crossing tests in the loop below it, but it means "above the refraction-
  corrected horizon", not "above the geometric horizon". No caller appears to depend on
  the difference.

## Index

| # | Severity | Issue | Primary location |
| --- | --- | --- | --- |
| 1 | medium | `moonevent-doc-promises-a-duration-field-removed-in-v3` | `dusk.go:118-119` |
| 2 | medium | `readme-lists-a-phase-angle-that-v4-removed` | `README.md:218` |
| 3 | medium | `readme-claims-go-1-24-while-go-mod-requires-1-27` | `README.md:264`, `go.mod:3` |
| 4 | medium | `solarposition-has-no-production-caller` | `solar.go:74-89` |
| 5 | medium | `sun-and-moon-fuzz-targets-assert-nothing` | `fuzz_test.go:15-30`, `63-78` |
| 6 | low | `horizontal-azimuth-can-be-360-at-the-pole-guard` | `epoch.go:197-215` |
| 7 | low | `claude-md-file-table-misplaces-the-date-range-sentinel` | `CLAUDE.md` Architecture table, `epoch.go:31` |

**Total: 7 issues (0 critical, 0 high, 5 medium, 2 low)**
