package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/philoserf/dusk/v4"
)

// The coordinates the tests drive the command with. Each returns a fresh
// slice, so a table entry can append its own flags without aliasing the next.
// They are named for the places they are, because a test that fails on
// "69.6492" says less than one that fails on tromso.

func tromso(rest ...string) []string {
	return append([]string{"--lat", "69.6492", "--lon", "18.9553", "--tz", "Europe/Oslo"}, rest...)
}

func mcmurdo(rest ...string) []string {
	return append([]string{"--lat", "-77.8419", "--lon", "166.6863", "--tz", "Antarctica/McMurdo"}, rest...)
}

func grandRapids(rest ...string) []string {
	return append([]string{"--lat", "42.9634", "--lon", "-85.6681", "--tz", "America/Detroit"}, rest...)
}

func nairobi(rest ...string) []string {
	return append([]string{"--lat", "-1.2921", "--lon", "36.8219", "--tz", "Africa/Nairobi"}, rest...)
}

// TestRun drives the whole program through run, asserting the contract - which
// sentinel a failure carries, and the shape of the output - rather than
// specific clock values. Golden times would couple these tests to the
// library's algorithms and break on any future accuracy improvement.
func TestRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		args       []string
		wantErr    error
		wantErrTxt []string
		wantStdout []string
	}{
		{
			name:       "ordinary day",
			args:       grandRapids("--date", "2025-06-21"),
			wantStdout: []string{"Sunrise", "Solar noon", "Sunset", "Civil dawn", "Moonrise", "Daylight", "Moon"},
		},
		{
			name:       "polar night is a result, not an error",
			args:       tromso("--date", "2025-12-21"),
			wantStdout: []string{"polar night", "does not rise"},
		},
		{
			name:       "midnight sun is a result, not an error",
			args:       tromso("--date", "2025-06-21"),
			wantStdout: []string{"midnight sun", "does not set"},
		},
		{
			name:       "twilight that never darkens is reported per band",
			args:       tromso("--date", "2025-06-21"),
			wantStdout: []string{"twilight never arrives", "6°"},
		},
		{
			name:       "southern polar location",
			args:       mcmurdo("--date", "2025-06-21"),
			wantStdout: []string{"77.8419°S"},
		},
		{
			name:       "equatorial location",
			args:       nairobi("--date", "2025-03-20"),
			wantStdout: []string{"Sunrise", "Daylight"},
		},
		{
			name:       "the header reports the observer it used",
			args:       []string{"--lat", "51.4779", "--lon", "-0.0015", "--tz", "Europe/London", "--date", "2025-03-20"},
			wantStdout: []string{"51.4779°N", "Europe/London", "20 March 2025"},
		},
		{
			name:       "date defaults to today",
			args:       nairobi(),
			wantStdout: []string{"Moon"},
		},
		{
			name:       "help is not a misuse and leaves by stdout",
			args:       []string{"--help"},
			wantStdout: []string{"usage:", "flags:", "--lat"},
		},
		{
			name:       "version",
			args:       []string{"--version"},
			wantStdout: []string{"dusk"},
		},
		{
			name:       "a date the library will not compute is not a misuse",
			args:       tromso("--date", "1600-01-01"),
			wantErr:    errUnsupportedDate,
			wantErrTxt: []string{"date outside valid range"},
		},
		{
			name:       "invalid latitude",
			args:       []string{"--lat", "200", "--lon", "0", "--tz", "UTC"},
			wantErr:    errUsage,
			wantErrTxt: []string{"latitude must be in"},
		},
		{
			name:       "no coordinates at all",
			args:       nil,
			wantErr:    errUsage,
			wantErrTxt: []string{"are all required"},
		},
		{
			name:       "incomplete coordinates",
			args:       []string{"--lat", "5", "--lon", "5"},
			wantErr:    errUsage,
			wantErrTxt: []string{"are all required"},
		},
		{
			name:       "an explicit zero latitude is not an omitted one",
			args:       []string{"--lat", "0", "--lon", "0", "--tz", "UTC", "--date", "2025-03-20"},
			wantStdout: []string{"0.0000°N", "Sunrise"},
		},
		{
			name:       "unknown timezone",
			args:       []string{"--lat", "5", "--lon", "5", "--tz", "Mars/Olympus"},
			wantErr:    errUsage,
			wantErrTxt: []string{"unknown timezone"},
		},
		{
			name:       "malformed date",
			args:       tromso("--date", "yesterday"),
			wantErr:    errUsage,
			wantErrTxt: []string{"is not YYYY-MM-DD"},
		},
		{
			name:       "unknown flag",
			args:       []string{"--bogus"},
			wantErr:    errUsage,
			wantErrTxt: []string{"bogus"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer

			err := run(tt.args, &stdout, &stderr)

			if tt.wantErr == nil && err != nil {
				t.Fatalf("run returned %v, want nil", err)
			}

			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("run returned %v, want it to wrap %v", err, tt.wantErr)
			}

			for _, want := range tt.wantErrTxt {
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Errorf("error %v does not mention %q", err, want)
				}
			}

			for _, want := range tt.wantStdout {
				if !strings.Contains(stdout.String(), want) {
					t.Errorf("stdout missing %q:\n%s", want, stdout.String())
				}
			}
		})
	}
}

// TestRunUsageGoesToStderrOnFailure checks that a misuse still shows the flags,
// and that it does so on the channel that is not the data channel.
func TestRunUsageGoesToStderrOnFailure(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	err := run([]string{"--lat", "5"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected an error")
	}

	if !strings.Contains(stderr.String(), "usage:") {
		t.Errorf("stderr carries no usage block:\n%s", stderr.String())
	}

	if stdout.Len() != 0 {
		t.Errorf("a failure wrote to the data channel:\n%s", stdout.String())
	}
}

// TestRunErrorsAreDistinguishable checks that the two sentinels do not
// overlap: a misuse must never read as an unsupported date, or the reader
// goes looking for a flag he typed correctly.
func TestRunErrorsAreDistinguishable(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	dateErr := run(tromso("--date", "1600-01-01"), &stdout, &stderr)
	if errors.Is(dateErr, errUsage) {
		t.Error("an unsupported date was reported as a usage error")
	}

	if !errors.Is(dateErr, dusk.ErrDateOutOfRange) {
		t.Error("the library's own sentinel was lost in wrapping")
	}

	stdout.Reset()
	stderr.Reset()

	usageErr := run([]string{"--lat", "5"}, &stdout, &stderr)
	if errors.Is(usageErr, errUnsupportedDate) {
		t.Error("a usage error was reported as an unsupported date")
	}
}

// TestRunJSON checks that --json emits parseable JSON with the expected shape,
// and that events which did not occur are absent rather than rendered as the
// year 1.
func TestRunJSON(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	err := run(tromso("--date", "2025-12-21", "--json"), &stdout, &stderr)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := stdout.String()
	if strings.Contains(out, "0001-01-01") {
		t.Error("JSON rendered a zero time as year 1; omitzero should drop it")
	}

	for _, key := range []string{`"lat"`, `"zone"`, `"date"`, `"sun"`, `"twilight"`, `"moon"`, `"phase"`} {
		if !strings.Contains(out, key) {
			t.Errorf("JSON missing key %s:\n%s", key, out)
		}
	}
}

// TestParseDate covers the detail most likely to be got wrong by a consumer:
// the date must be interpreted in the observer's zone, because the library
// derives the calendar day from date.In(observer location). Parsing as UTC
// would silently select the previous day for any observer west of Greenwich.
func TestParseDate(t *testing.T) {
	t.Parallel()

	chicago, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}

	t.Run("parses in the observer zone", func(t *testing.T) {
		t.Parallel()

		got, err := parseDate("2025-06-21", chicago)
		if err != nil {
			t.Fatalf("parseDate: %v", err)
		}

		if got.Location() != chicago {
			t.Errorf("location = %v, want %v", got.Location(), chicago)
		}

		if y, m, d := got.Date(); y != 2025 || m != time.June || d != 21 {
			t.Errorf("date = %d-%02d-%02d, want 2025-06-21", y, m, d)
		}
	})

	t.Run("UTC midnight would land on the previous day", func(t *testing.T) {
		t.Parallel()

		utcMidnight := time.Date(2025, 6, 21, 0, 0, 0, 0, time.UTC)
		if day := utcMidnight.In(chicago).Day(); day != 20 {
			t.Fatalf("premise broken: UTC midnight is day %d in Chicago, want 20", day)
		}

		correct, err := parseDate("2025-06-21", chicago)
		if err != nil {
			t.Fatalf("parseDate: %v", err)
		}

		if correct.In(chicago).Day() != 21 {
			t.Error("parseDate lost a day; it must parse in the observer's zone")
		}
	})

	t.Run("empty defaults to today", func(t *testing.T) {
		t.Parallel()

		got, err := parseDate("", chicago)
		if err != nil {
			t.Fatalf("parseDate: %v", err)
		}

		if got.IsZero() {
			t.Error("default date is zero")
		}
	})

	t.Run("malformed", func(t *testing.T) {
		t.Parallel()

		_, err := parseDate("21/06/2025", chicago)
		if !errors.Is(err, errUsage) {
			t.Errorf("err = %v, want it to wrap errUsage", err)
		}
	})
}

// TestObserverFromFlags covers the required-flag rules. An omitted flag and an
// explicit zero must not be confused: --lat 0 is the equator.
func TestObserverFromFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		lat, lon float64
		tz       string
		set      map[string]bool
		wantErr  bool
	}{
		{name: "all three given", lat: 51.5, lon: -0.1, tz: "Europe/London", set: map[string]bool{"lat": true, "lon": true, "tz": true}},
		{name: "explicit zeroes are given", tz: "UTC", set: map[string]bool{"lat": true, "lon": true, "tz": true}},
		{name: "missing tz", lat: 5, lon: 5, set: map[string]bool{"lat": true, "lon": true}, wantErr: true},
		{name: "missing lon", lat: 5, tz: "UTC", set: map[string]bool{"lat": true, "tz": true}, wantErr: true},
		{name: "nothing given", set: map[string]bool{}, wantErr: true},
		{name: "bad coords", lat: 91, tz: "UTC", set: map[string]bool{"lat": true, "lon": true, "tz": true}, wantErr: true},
		{name: "bad zone", lat: 5, lon: 5, tz: "Mars/Olympus", set: map[string]bool{"lat": true, "lon": true, "tz": true}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := observerFromFlags(tt.lat, tt.lon, tt.tz, tt.set)

			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}

			// Every refusal here is a misuse of the flags, and must say so.
			if tt.wantErr && !errors.Is(err, errUsage) {
				t.Errorf("err = %v, want it to wrap errUsage", err)
			}
		})
	}
}
