package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/philoserf/dusk/v3"
)

// dateLayout is the only date format the CLI accepts, on input and output.
const dateLayout = "2006-01-02"

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

// Report is one observer's full day: sun, the three twilights, moon, and phase.
//
// Time fields use omitzero rather than omitempty. A zero time.Time is a struct,
// and omitempty does not drop struct zero values - it would emit the useless
// "0001-01-01T00:00:00Z". omitzero consults IsZero, so an event that did not
// occur is simply absent from the JSON.
type Report struct {
	Lat      float64          `json:"lat"`
	Lon      float64          `json:"lon"`
	Zone     string           `json:"zone"`
	Date     string           `json:"date"`
	Sun      SunReport        `json:"sun"`
	Twilight []TwilightReport `json:"twilight"`
	Moon     MoonReport       `json:"moon"`
	Phase    PhaseReport      `json:"phase"`

	date time.Time
}

// SunReport holds sunrise, solar noon, and sunset.
type SunReport struct {
	Rise     time.Time `json:"rise,omitzero"`
	Noon     time.Time `json:"noon,omitzero"`
	Set      time.Time `json:"set,omitzero"`
	Daylight string    `json:"daylight,omitempty"`
	Note     string    `json:"note,omitempty"`

	state horizonState
}

// TwilightReport holds one twilight band. Dawn is this morning's, Dusk is
// tonight's - see twilightReports for why those come from different calls.
type TwilightReport struct {
	Name  string    `json:"name"`
	Dawn  time.Time `json:"dawn,omitzero"`
	Dusk  time.Time `json:"dusk,omitzero"`
	Night string    `json:"night,omitempty"`
	Note  string    `json:"note,omitempty"`

	degrees int
	state   horizonState
}

// MoonReport holds moonrise and moonset.
type MoonReport struct {
	Rise         time.Time `json:"rise,omitzero"`
	Set          time.Time `json:"set,omitzero"`
	AboveHorizon bool      `json:"aboveHorizon"`
	Note         string    `json:"note,omitempty"`

	state horizonState
}

// PhaseReport holds the lunar phase.
type PhaseReport struct {
	Name         string  `json:"name"`
	Illumination float64 `json:"illumination"`
	Elongation   float64 `json:"elongation"`
	DaysApprox   float64 `json:"daysApprox"`
	Waxing       bool    `json:"waxing"`
}

// The JSON-facing prose for each state. Tables rather than switches: a switch
// over an integer type needs a trailing return the compiler cannot prove is
// unreachable, and that line can never be covered or tested.
var (
	solarNotes = map[horizonState]string{
		stateStaysAbove: "midnight sun - the sun does not set today",
		stateStaysBelow: "polar night - the sun does not rise today",
	}

	// At a depression angle the two sentinels mean the opposite of what they
	// mean at the horizon: staying above the angle means the night never gets
	// that dark, staying below it means the day never gets that light.
	twilightNotes = map[horizonState]string{
		stateStaysAbove: "never gets this dark - the sun stays above %d degrees",
		stateStaysBelow: "this dark all day - the sun stays below %d degrees",
	}

	lunarNotes = map[horizonState]string{
		stateStaysAbove: "the Moon stays above the horizon today",
		stateStaysBelow: "the Moon stays below the horizon today",
	}
)

// solarNote is the prose for a sun that never crosses the horizon.
func solarNote(state horizonState) string { return solarNotes[state] }

// lunarNote is the prose for a Moon that never crosses the horizon.
func lunarNote(state horizonState) string { return lunarNotes[state] }

// twilightNote is the prose for a twilight band that never arrives, or that
// never lifts.
func twilightNote(state horizonState, degrees int) string {
	format, ok := twilightNotes[state]
	if !ok {
		return ""
	}

	return fmt.Sprintf(format, degrees)
}

// buildReport assembles a full day by calling every public dusk entry point.
func buildReport(obs dusk.Observer, date time.Time) (Report, error) {
	sun, err := sunReport(date, obs)
	if err != nil {
		return Report{}, err
	}

	bands, err := twilightReports(date, obs)
	if err != nil {
		return Report{}, err
	}

	moon, err := moonReport(date, obs)
	if err != nil {
		return Report{}, err
	}

	phase, err := phaseReport(date)
	if err != nil {
		return Report{}, err
	}

	return Report{
		Lat:      obs.Lat(),
		Lon:      obs.Lon(),
		Zone:     obs.Location().String(),
		Date:     date.Format(dateLayout),
		date:     date,
		Sun:      sun,
		Twilight: bands,
		Moon:     moon,
		Phase:    phase,
	}, nil
}

// sunReport computes sunrise, noon, and sunset for the day.
func sunReport(date time.Time, obs dusk.Observer) (SunReport, error) {
	event, err := dusk.SunriseSunset(date, obs)
	if err != nil {
		state, ok := stateOf(err)
		if !ok {
			return SunReport{}, fmt.Errorf("sunrise/sunset: %w", err)
		}

		return SunReport{Note: solarNote(state), state: state}, nil
	}

	return SunReport{
		Rise:     toSecond(event.Rise),
		Noon:     toSecond(event.Noon),
		Set:      toSecond(event.Set),
		Daylight: shortDuration(event.Duration),
	}, nil
}

// twilightFunc is the shared signature of the three exported twilight calls.
type twilightFunc func(time.Time, dusk.Observer) (dusk.TwilightEvent, error)

// twilightBands names each twilight, its depression angle, and its constructor.
var twilightBands = []struct {
	name    string
	degrees int
	fn      twilightFunc
}{
	{"Civil", 6, dusk.CivilTwilight},
	{"Nautical", 12, dusk.NauticalTwilight},
	{"Astronomical", 18, dusk.AstronomicalTwilight},
}

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

		morning, state, err := callTwilight(band.fn, yesterday, obs, band.name)
		if err != nil {
			return nil, err
		}

		report.Dawn = toSecond(morning.Dawn)
		report.state = state

		evening, state, err := callTwilight(band.fn, date, obs, band.name)
		if err != nil {
			return nil, err
		}

		report.Dusk = toSecond(evening.Dusk)
		report.Night = shortDuration(evening.NightDuration)

		// Tonight's geometry is the one the report is about; yesterday's call
		// only ever supplies this morning's dawn.
		if state != stateCrosses {
			report.state = state
			report.Night = ""
		}

		report.Note = twilightNote(report.state, band.degrees)

		reports = append(reports, report)
	}

	return reports, nil
}

// callTwilight separates the library's three outcomes: a real event, an
// expected polar-geometry state, or a genuine error worth failing on.
func callTwilight(
	fn twilightFunc, date time.Time, obs dusk.Observer, name string,
) (dusk.TwilightEvent, horizonState, error) {
	event, err := fn(date, obs)
	if err == nil {
		return event, stateCrosses, nil
	}

	state, ok := stateOf(err)
	if !ok {
		return dusk.TwilightEvent{}, stateCrosses, fmt.Errorf("%s twilight: %w", name, err)
	}

	return dusk.TwilightEvent{}, state, nil
}

// moonReport computes moonrise and moonset for the day.
//
// Unlike the sun, the Moon routinely rises without setting before midnight (or
// the reverse), because a lunar day runs about 24h50m. dusk signals that with a
// zero-value time, which is a normal result and distinct from the sentinel
// errors above.
func moonReport(date time.Time, obs dusk.Observer) (MoonReport, error) {
	event, err := dusk.MoonriseMoonset(date, obs)
	if err != nil {
		state, ok := stateOf(err)
		if !ok {
			return MoonReport{}, fmt.Errorf("moonrise/moonset: %w", err)
		}

		return MoonReport{Note: lunarNote(state), state: state}, nil
	}

	return MoonReport{
		Rise:         toSecond(event.Rise),
		Set:          toSecond(event.Set),
		AboveHorizon: event.AboveHorizon,
	}, nil
}

// phaseReport computes the lunar phase. It depends only on the date, not on the
// observer - the Moon shows the same face to the whole Earth.
func phaseReport(date time.Time) (PhaseReport, error) {
	phase, err := dusk.LunarPhase(date)
	if err != nil {
		return PhaseReport{}, fmt.Errorf("lunar phase: %w", err)
	}

	return PhaseReport{
		Name:         phase.Name,
		Illumination: phase.Illumination,
		Elongation:   phase.Elongation,
		DaysApprox:   phase.DaysApprox,
		Waxing:       phase.Waxing,
	}, nil
}

// toSecond drops sub-second precision. The underlying algorithms do not have
// nanosecond accuracy, and printing it would imply precision that is not there.
// A zero time - meaning "this did not happen" - stays zero.
func toSecond(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}

	return t.Truncate(time.Second)
}

// shortDuration renders a duration as "15h22m", dropping seconds. A duration
// that is zero or negative renders as empty.
func shortDuration(d time.Duration) string {
	if d <= 0 {
		return ""
	}

	total := int(d.Round(time.Minute).Minutes())

	return fmt.Sprintf("%dh%02dm", total/60, total%60)
}
