package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/philoserf/dusk/v3"
)

// testObserver returns a mid-latitude observer for deterministic tests.
func testObserver(t *testing.T) (dusk.Observer, *time.Location) {
	t.Helper()

	loc, err := time.LoadLocation("America/Detroit")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}

	obs, err := dusk.NewObserver(42.9634, -85.6681, loc)
	if err != nil {
		t.Fatalf("NewObserver: %v", err)
	}

	return obs, loc
}

// TestTwilightDawnComesFromYesterday pins down the library's most easily missed
// detail. dusk.TwilightEvent.Dawn is always *tomorrow* morning's, so a day
// report must take this morning's dawn from yesterday's event. Reporting
// today's Dawn field would silently show tomorrow's time.
func TestTwilightDawnComesFromYesterday(t *testing.T) {
	t.Parallel()

	obs, loc := testObserver(t)
	date := time.Date(2025, 6, 21, 0, 0, 0, 0, loc)

	got, err := twilightReports(date, obs)
	if err != nil {
		t.Fatalf("twilightReports: %v", err)
	}

	if len(got) != len(twilightBands) {
		t.Fatalf("got %d bands, want %d", len(got), len(twilightBands))
	}

	yesterday, err := dusk.CivilTwilight(date.AddDate(0, 0, -1), obs)
	if err != nil {
		t.Fatalf("CivilTwilight(yesterday): %v", err)
	}

	today, err := dusk.CivilTwilight(date, obs)
	if err != nil {
		t.Fatalf("CivilTwilight(today): %v", err)
	}

	civil := got[0]

	if !civil.Dawn.Equal(toSecond(yesterday.Dawn)) {
		t.Errorf("Dawn = %v, want yesterday's Dawn %v", civil.Dawn, toSecond(yesterday.Dawn))
	}

	if !civil.Dusk.Equal(toSecond(today.Dusk)) {
		t.Errorf("Dusk = %v, want today's Dusk %v", civil.Dusk, toSecond(today.Dusk))
	}

	if civil.Dawn.Equal(toSecond(today.Dawn)) {
		t.Error("Dawn came from today's event, which is tomorrow morning's dawn")
	}

	if civil.Dawn.Day() != date.Day() {
		t.Errorf("Dawn falls on day %d, want the reported day %d", civil.Dawn.Day(), date.Day())
	}
}

// TestStateOfMapsTheSentinels checks that the library's two geometric
// sentinels become states and every other error stays an error.
func TestStateOfMapsTheSentinels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		err       error
		wantState horizonState
		wantOK    bool
	}{
		{name: "circumpolar", err: dusk.ErrCircumpolar, wantState: stateStaysAbove, wantOK: true},
		{name: "never rises", err: dusk.ErrNeverRises, wantState: stateStaysBelow, wantOK: true},
		{name: "a real failure", err: dusk.ErrDateOutOfRange, wantState: stateCrosses, wantOK: false},
		{name: "wrapped sentinel", err: fmt.Errorf("twilight: %w", dusk.ErrCircumpolar), wantState: stateStaysAbove, wantOK: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			state, ok := stateOf(tt.err)

			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}

			if state != tt.wantState {
				t.Errorf("state = %v, want %v", state, tt.wantState)
			}
		})
	}
}

// TestNotesDistinguishStates checks the JSON-facing prose, and in particular
// that a depression angle inverts what the two sentinels mean: above the angle
// is a night that never darkens, below it is a day that never lightens.
func TestNotesDistinguishStates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		got     string
		wantSub string
	}{
		{"solar stays above", solarNotes[stateStaysAbove], "midnight sun"},
		{"solar stays below", solarNotes[stateStaysBelow], "polar night"},
		{"solar crosses", solarNotes[stateCrosses], ""},
		{"twilight stays above", twilightNote(stateStaysAbove, 6), "never gets this dark"},
		{"twilight stays below", twilightNote(stateStaysBelow, 6), "this dark all day"},
		{"twilight crosses", twilightNote(stateCrosses, 6), ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.wantSub == "" {
				if tt.got != "" {
					t.Errorf("note = %q, want empty", tt.got)
				}

				return
			}

			if !strings.Contains(tt.got, tt.wantSub) {
				t.Errorf("note = %q, want it to mention %q", tt.got, tt.wantSub)
			}
		})
	}
}

// TestBuildReportPropagatesRealErrors checks that a genuine failure is not
// swallowed as a polar note.
func TestBuildReportPropagatesRealErrors(t *testing.T) {
	t.Parallel()

	obs, loc := testObserver(t)
	date := time.Date(1600, 1, 1, 0, 0, 0, 0, loc)

	_, err := buildReport(obs, date)
	if !errors.Is(err, dusk.ErrDateOutOfRange) {
		t.Errorf("err = %v, want ErrDateOutOfRange", err)
	}
}

// TestBuildReportPolar checks that polar geometry produces a complete report
// with notes instead of an error.
func TestBuildReportPolar(t *testing.T) {
	t.Parallel()

	loc, err := time.LoadLocation("Europe/Oslo")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}

	obs, err := dusk.NewObserver(69.6492, 18.9553, loc)
	if err != nil {
		t.Fatalf("NewObserver: %v", err)
	}

	date := time.Date(2025, 12, 21, 0, 0, 0, 0, loc)

	report, err := buildReport(obs, date)
	if err != nil {
		t.Fatalf("buildReport: %v", err)
	}

	if report.Sun.Note == "" {
		t.Error("expected a polar note on the sun")
	}

	if !report.Sun.Rise.IsZero() || !report.Sun.Set.IsZero() {
		t.Error("expected zero rise and set times during polar night")
	}

	if report.Phase.Name == "" {
		t.Error("lunar phase should still be reported during polar night")
	}
}

// TestShortDuration covers the duration formatter.
func TestShortDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   time.Duration
		want string
	}{
		{"hours and minutes", 15*time.Hour + 21*time.Minute, "15h21m"},
		{"pads minutes", 4*time.Hour + 2*time.Minute, "4h02m"},
		{"rounds seconds away", 4*time.Hour + 2*time.Minute + 40*time.Second, "4h03m"},
		{"zero", 0, ""},
		{"negative", -time.Hour, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := shortDuration(tt.in); got != tt.want {
				t.Errorf("shortDuration(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestToSecond checks that sub-second noise is dropped and zero stays zero.
func TestToSecond(t *testing.T) {
	t.Parallel()

	t.Run("zero stays zero", func(t *testing.T) {
		t.Parallel()

		if got := toSecond(time.Time{}); !got.IsZero() {
			t.Errorf("toSecond(zero) = %v, want zero", got)
		}
	})

	t.Run("drops nanoseconds", func(t *testing.T) {
		t.Parallel()

		in := time.Date(2025, 6, 21, 5, 3, 12, 987654321, time.UTC)
		if got := toSecond(in); got.Nanosecond() != 0 {
			t.Errorf("toSecond kept %d nanoseconds", got.Nanosecond())
		}
	})
}
