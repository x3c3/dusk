package main

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// clockLayout is how times of day are printed in the text report.
const clockLayout = "15:04"

// headingLayout spells the date the way a person reads it.
const headingLayout = "Monday 2 January 2006"

// renderJSON writes the report as indented JSON.
func renderJSON(w io.Writer, report Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	err := enc.Encode(report)
	if err != nil {
		return fmt.Errorf("encoding JSON: %w", err)
	}

	return nil
}

// renderText writes the report as a chronological account of the day.
//
// The library returns its results grouped by the call that produced them -
// sun, three twilight bands, moon - but a day is not lived in that order.
// Printed that way, the twilight table's dawn column runs backwards and a
// moonset that belongs to the previous night's rise appears above the
// moonrise it precedes. Sorting every event by clock time removes both.
func renderText(w io.Writer, report Report) error {
	blocks := []string{
		fmt.Sprintf("%s\n%s  %s  ·  %s",
			report.date.Format(headingLayout),
			formatLat(report.Lat), formatLon(report.Lon), report.Zone),
	}

	if lines := conditions(report); len(lines) > 0 {
		indented := make([]string, 0, len(lines))
		for _, line := range lines {
			indented = append(indented, "  "+wrapAt(line, 66, "  "))
		}

		blocks = append(blocks, strings.Join(indented, "\n"))
	}

	if events := timeline(report); len(events) > 0 {
		rows := make([]string, 0, len(events))
		for _, e := range events {
			rows = append(rows, fmt.Sprintf("  %s   %s", e.at.Format(clockLayout), e.label))
		}

		blocks = append(blocks, strings.Join(rows, "\n"))
	}

	blocks = append(blocks, strings.Join(summary(report), "\n"))

	_, err := io.WriteString(w, strings.Join(blocks, "\n\n")+"\n")
	if err != nil {
		return fmt.Errorf("writing the report: %w", err)
	}

	return nil
}

// dayEvent is one moment worth printing, with the words to print for it.
type dayEvent struct {
	at    time.Time
	label string
}

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

// conditions describes what the timeline cannot: the geometry that stopped an
// event from happening at all. Without these lines a polar day prints a short
// list of times and no hint that the sun never came up.
func conditions(report Report) []string {
	var lines []string

	if line := sunCondition(report); line != "" {
		lines = append(lines, line)
	}

	lines = append(lines, twilightConditions(report.Twilight)...)

	if line := moonCondition(report.Moon); line != "" {
		lines = append(lines, line)
	}

	return lines
}

// sunCondition reports a sun that never crosses the horizon, and says whether
// twilight still arrives - otherwise "the sun does not rise" sits beside a
// civil dawn with no explanation of how both are true.
func sunCondition(report Report) string {
	switch report.Sun.state {
	case stateStaysAbove:
		return "The sun does not set today (midnight sun)."
	case stateStaysBelow:
		line := "The sun does not rise today (polar night)."

		for _, band := range report.Twilight {
			if band.state == stateCrosses {
				return line + " Twilight still reaches " +
					strings.ToLower(band.Name) + " depth around midday."
			}
		}

		return line
	case stateCrosses:
	}

	return ""
}

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

	if len(tooLight) > 0 {
		lines = append(lines, fmt.Sprintf(
			"The sun never drops %d° below the horizon, so %s twilight never arrives.",
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

// moonCondition explains a moon that does not cross, and flags a moon that is
// already up at midnight - which is why a moonset can precede a moonrise.
func moonCondition(moon MoonReport) string {
	switch moon.state {
	case stateStaysAbove:
		return "The moon stays above the horizon all day."
	case stateStaysBelow:
		return "The moon stays below the horizon all day."
	case stateCrosses:
	}

	switch {
	case moon.Rise.IsZero() && moon.Set.IsZero():
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

// summary is the handful of totals worth having after the times.
func summary(report Report) []string {
	var rows []string

	if report.Sun.Daylight != "" {
		rows = append(rows, fmt.Sprintf("  %-10s%7s", "Daylight", report.Sun.Daylight))
	}

	// The deepest band that actually has a night is the interesting one: at
	// most latitudes that is astronomical, but in a bright summer it is
	// whichever band still manages to arrive.
	for _, band := range slices.Backward(report.Twilight) {
		if band.Night != "" {
			rows = append(rows, fmt.Sprintf("  %-10s%7s  (%s, tonight)",
				"Dark", band.Night, strings.ToLower(band.Name)))

			break
		}
	}

	// The name and the percentage are the whole phase. Whether it is waxing
	// carries in the name where it matters ("Waning Crescent") and is obvious
	// from the percentage where it does not; the JSON keeps the flag for
	// anything that wants to branch on it.
	// Whole percent: the library documents illumination as within 1-2% of
	// published values, so a tenth of a percent claims twenty times the
	// accuracy the number has.
	rows = append(rows, fmt.Sprintf("  %-10s %s, %.0f%%",
		"Moon", report.Phase.Name, report.Phase.Illumination))

	return rows
}

// formatLat renders a latitude with its hemisphere, so that a reader does not
// have to remember which sign means south.
func formatLat(lat float64) string {
	if lat < 0 {
		return fmt.Sprintf("%.4f°S", -lat)
	}

	return fmt.Sprintf("%.4f°N", lat)
}

// formatLon renders a longitude with its hemisphere.
func formatLon(lon float64) string {
	if lon < 0 {
		return fmt.Sprintf("%.4f°W", -lon)
	}

	return fmt.Sprintf("%.4f°E", lon)
}

// join lists names as English does: "civil", "civil and nautical",
// "civil, nautical and astronomical".
func join(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	default:
		return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
	}
}

// wrapAt breaks a sentence onto lines no longer than width, indenting the
// continuations, so a condition reads as a paragraph rather than one long row.
func wrapAt(text string, width int, indent string) string {
	var (
		out  strings.Builder
		line int
	)

	for i, word := range strings.Fields(text) {
		// Runes, not bytes: a degree sign is two bytes and one column.
		runcount := utf8.RuneCountInString(word)

		switch {
		case i == 0:
			out.WriteString(word)

			line = runcount
		case line+1+runcount > width:
			out.WriteString("\n" + indent + word)
			line = utf8.RuneCountInString(indent) + runcount
		default:
			out.WriteString(" " + word)

			line += 1 + runcount
		}
	}

	return out.String()
}
