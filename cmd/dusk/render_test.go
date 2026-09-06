package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

// at builds a time on the report's day, in UTC, for readable fixtures.
func at(hour, minute int) time.Time {
	return time.Date(2025, 6, 21, hour, minute, 0, 0, time.UTC)
}

// sampleReport is an ordinary day with one event missing: the Moon rises but
// does not set, which is routine because a lunar day runs about 24h50m.
func sampleReport() Report {
	return Report{
		Lat:  0,
		Lon:  0,
		Zone: "UTC",
		Date: "2025-06-21",
		date: at(0, 0),
		Sun: SunReport{
			Rise:     at(6, 3),
			Noon:     at(13, 44),
			Set:      at(21, 25),
			Daylight: "15h21m",
		},
		Twilight: []TwilightReport{
			{Name: "Civil", degrees: 6, Dawn: at(5, 28), Dusk: at(22, 0), Night: "7h28m"},
			{Name: "Nautical", degrees: 12, Dawn: at(4, 42), Dusk: at(22, 46), Night: "5h56m"},
			{Name: "Astronomical", degrees: 18, Dawn: at(3, 45), Dusk: at(23, 43), Night: "4h02m"},
		},
		Moon:  MoonReport{Rise: at(2, 37)},
		Phase: PhaseReport{Name: "Waning Crescent", Illumination: 23.9, DaysApprox: 24.7},
	}
}

// flat collapses whitespace so an assertion about wording is not also an
// assertion about where a line happened to wrap.
func flat(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// timelineLabels returns the event rows of a rendered report, in the order
// they were printed.
func timelineLabels(t *testing.T, out string) []string {
	t.Helper()

	row := regexp.MustCompile(`^  (\d\d:\d\d)   (.+)$`)

	var labels []string

	for line := range strings.SplitSeq(out, "\n") {
		if m := row.FindStringSubmatch(line); m != nil {
			labels = append(labels, m[1]+" "+m[2])
		}
	}

	return labels
}

// TestRenderTextIsChronological is the whole point of the layout: the library
// groups results by the call that produced them, and a day is not lived in
// that order.
func TestRenderTextIsChronological(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	err := renderText(&buf, sampleReport())
	if err != nil {
		t.Fatalf("renderText: %v", err)
	}

	got := timelineLabels(t, buf.String())
	want := []string{
		"02:37 Moonrise",
		"03:45 Astronomical dawn",
		"04:42 Nautical dawn",
		"05:28 Civil dawn",
		"06:03 Sunrise",
		"13:44 Solar noon",
		"21:25 Sunset",
		"22:00 Civil dusk",
		"22:46 Nautical dusk",
		"23:43 Astronomical dusk",
	}

	if !slices.Equal(got, want) {
		t.Errorf("timeline =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestRenderTextOmitsEventsThatDidNotHappen checks that a zero time leaves no
// row at all, rather than a placeholder the reader has to interpret.
func TestRenderTextOmitsEventsThatDidNotHappen(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	err := renderText(&buf, sampleReport())
	if err != nil {
		t.Fatalf("renderText: %v", err)
	}

	out := buf.String()

	if strings.Contains(out, "Moonset") {
		t.Error("the Moon does not set in this fixture; no row should be printed")
	}

	if strings.Contains(out, "0001") || strings.Contains(out, "00:00") {
		t.Errorf("a zero time leaked into the report:\n%s", out)
	}

	if !strings.Contains(out, "The moon does not set today.") {
		t.Errorf("the missing moonset should be stated in words:\n%s", out)
	}
}

// TestRenderTextMarksEventsOnTheNextDay covers the high-latitude case where
// sunset falls after local midnight: it sorts last, and without the date it
// reads as a mistake.
func TestRenderTextMarksEventsOnTheNextDay(t *testing.T) {
	t.Parallel()

	report := sampleReport()
	report.Sun.Set = time.Date(2025, 6, 22, 0, 3, 0, 0, time.UTC)
	report.Twilight = nil

	var buf bytes.Buffer

	err := renderText(&buf, report)
	if err != nil {
		t.Fatalf("renderText: %v", err)
	}

	labels := timelineLabels(t, buf.String())
	if len(labels) == 0 {
		t.Fatalf("no timeline rows were printed:\n%s", buf.String())
	}

	last := labels[len(labels)-1]
	if last != "00:03 Sunset (22 Jun)" {
		t.Errorf("last event = %q, want the sunset marked with its date", last)
	}
}

// TestRenderTextConditions checks the prose that the timeline cannot carry:
// why an event is missing rather than merely absent.
func TestRenderTextConditions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*Report)
		want   []string
		absent []string
	}{
		{
			name: "polar night names the twilight that still arrives",
			mutate: func(r *Report) {
				r.Sun = SunReport{state: stateStaysBelow}
			},
			want: []string{"does not rise today (polar night)", "reaches civil depth"},
		},
		{
			name: "midnight sun",
			mutate: func(r *Report) {
				r.Sun = SunReport{state: stateStaysAbove}
			},
			want: []string{"does not set today (midnight sun)"},
		},
		{
			name: "bands that never darken collapse to one sentence",
			mutate: func(r *Report) {
				for i := range r.Twilight {
					r.Twilight[i].state = stateStaysAbove
					r.Twilight[i].Dawn = time.Time{}
					r.Twilight[i].Dusk = time.Time{}
				}
			},
			want:   []string{"never drops 6° below the horizon", "civil, nautical and astronomical twilight never arrives"},
			absent: []string{"12°", "18°"},
		},
		{
			name: "bands that stay dark all day",
			mutate: func(r *Report) {
				for i := range r.Twilight {
					r.Twilight[i].state = stateStaysBelow
					r.Twilight[i].Dawn = time.Time{}
					r.Twilight[i].Dusk = time.Time{}
				}
			},
			want: []string{"never climbs within 18° of the horizon", "darkness lasts all day"},
		},
		{
			name: "a moon already up explains a moonset before a moonrise",
			mutate: func(r *Report) {
				r.Moon = MoonReport{Rise: at(23, 11), Set: at(10, 59), AboveHorizon: true}
			},
			want: []string{"already up at midnight", "precedes its moonrise"},
		},
		{
			name:   "a moon that neither rises nor sets",
			mutate: func(r *Report) { r.Moon = MoonReport{} },
			want:   []string{"neither rises nor sets today"},
		},
		{
			// A lunar day is about 24h50m, so the Moon can be up for a whole
			// calendar day without crossing. The library says so with
			// AboveHorizon rather than a sentinel, and reading it as an absent
			// Moon loses the one fact the report had.
			name:   "a moon up all day is not an absent moon",
			mutate: func(r *Report) { r.Moon = MoonReport{AboveHorizon: true} },
			want:   []string{"moon stays above the horizon all day"},
			absent: []string{"neither rises nor sets"},
		},
		{
			name: "an ordinary moon needs no explanation",
			mutate: func(r *Report) {
				r.Moon = MoonReport{Rise: at(2, 37), Set: at(17, 35)}
			},
			want:   []string{"02:37 Moonrise"},
			absent: []string{"The moon"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			report := sampleReport()
			tt.mutate(&report)

			var buf bytes.Buffer

			err := renderText(&buf, report)
			if err != nil {
				t.Fatalf("renderText: %v", err)
			}

			for _, want := range tt.want {
				if !strings.Contains(flat(buf.String()), want) {
					t.Errorf("report missing %q:\n%s", want, buf.String())
				}
			}

			for _, absent := range tt.absent {
				if strings.Contains(flat(buf.String()), absent) {
					t.Errorf("report should not repeat %q:\n%s", absent, buf.String())
				}
			}
		})
	}
}

// TestRenderTextHeader checks that coordinates carry their hemisphere, so a
// reader need not remember which sign means south.
func TestRenderTextHeader(t *testing.T) {
	t.Parallel()

	report := sampleReport()
	report.Lat = -77.8419
	report.Lon = 166.6863
	report.Zone = "Antarctica/McMurdo"

	var buf bytes.Buffer

	err := renderText(&buf, report)
	if err != nil {
		t.Fatalf("renderText: %v", err)
	}

	for _, want := range []string{"Saturday 21 June 2025", "77.8419°S", "166.6863°E", "Antarctica/McMurdo"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("header missing %q:\n%s", want, buf.String())
		}
	}
}

// TestRenderTextSummary checks the totals under the timeline.
func TestRenderTextSummary(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	err := renderText(&buf, sampleReport())
	if err != nil {
		t.Fatalf("renderText: %v", err)
	}

	out := buf.String()

	// The deepest band that actually has a night is the one worth reporting.
	for _, want := range []string{"Daylight   15h21m", "4h02m  (astronomical, tonight)", "Moon       Waning Crescent, 24%"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q:\n%s", want, out)
		}
	}
}

// TestRenderJSONOmitsZeroTimes checks that omitzero drops events that did not
// occur, rather than emitting "0001-01-01T00:00:00Z".
func TestRenderJSONOmitsZeroTimes(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	err := renderJSON(&buf, sampleReport())
	if err != nil {
		t.Fatalf("renderJSON: %v", err)
	}

	if strings.Contains(buf.String(), "0001-01-01") {
		t.Errorf("JSON leaked a zero time:\n%s", buf.String())
	}

	var decoded struct {
		Moon map[string]any `json:"moon"`
	}

	err = json.Unmarshal(buf.Bytes(), &decoded)
	if err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	if _, ok := decoded.Moon["set"]; ok {
		t.Error("moonset did not occur, so the key should be absent")
	}

	if _, ok := decoded.Moon["rise"]; !ok {
		t.Error("moonrise did occur, so the key should be present")
	}
}

// TestFormatCoordinates covers the hemisphere suffixes.
func TestFormatCoordinates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		lat, lon float64
		wantLat  string
		wantLon  string
	}{
		{name: "north east", lat: 69.6492, lon: 18.9553, wantLat: "69.6492°N", wantLon: "18.9553°E"},
		{name: "south west", lat: -1.2921, lon: -85.6681, wantLat: "1.2921°S", wantLon: "85.6681°W"},
		{name: "null island", wantLat: "0.0000°N", wantLon: "0.0000°E"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := formatLat(tt.lat); got != tt.wantLat {
				t.Errorf("formatLat(%v) = %q, want %q", tt.lat, got, tt.wantLat)
			}

			if got := formatLon(tt.lon); got != tt.wantLon {
				t.Errorf("formatLon(%v) = %q, want %q", tt.lon, got, tt.wantLon)
			}
		})
	}
}

// TestJoin covers the English list.
func TestJoin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   []string
		want string
	}{
		{name: "none", in: nil, want: ""},
		{name: "one", in: []string{"civil"}, want: "civil"},
		{name: "two", in: []string{"civil", "nautical"}, want: "civil and nautical"},
		{name: "three", in: []string{"civil", "nautical", "astronomical"}, want: "civil, nautical and astronomical"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := join(tt.in); got != tt.want {
				t.Errorf("join(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestWrapAt checks that a long condition breaks into indented lines.
func TestWrapAt(t *testing.T) {
	t.Parallel()

	got := wrapAt("the quick brown fox jumps over the lazy dog", 20, "  ")

	for line := range strings.SplitSeq(got, "\n") {
		if len(line) > 20 {
			t.Errorf("line exceeds the width: %q", line)
		}
	}

	if strings.Join(strings.Fields(got), " ") != "the quick brown fox jumps over the lazy dog" {
		t.Errorf("wrapping changed the words: %q", got)
	}
}

// TestRenderErrors checks that a failing writer surfaces an error rather than
// being silently dropped.
func TestRenderErrors(t *testing.T) {
	t.Parallel()

	t.Run("text", func(t *testing.T) {
		t.Parallel()

		err := renderText(failingWriter{}, sampleReport())
		if err == nil {
			t.Error("expected an error from a failing writer")
		}
	})

	t.Run("json", func(t *testing.T) {
		t.Parallel()

		err := renderJSON(failingWriter{}, sampleReport())
		if err == nil {
			t.Error("expected an error from a failing writer")
		}
	})
}

// failingWriter fails every write.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errWriteFailed
}

// errWriteFailed is the failure returned by failingWriter.
var errWriteFailed = errors.New("write failed")
