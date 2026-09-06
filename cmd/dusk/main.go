// Command dusk reports a full day of astronomical events - sunrise and sunset,
// civil, nautical, and astronomical twilight, moonrise and moonset, and the
// lunar phase - for one place on one date.
//
// It is the reference consumer of the dusk library: it calls every exported
// entry point, and every documented edge case is reachable with a single flag.
//
// Usage:
//
//	dusk --lat DEG --lon DEG --tz ZONE [--date YYYY-MM-DD] [--json]
//	dusk --version
//
// Polar geometry is a result, not a failure. Above the Arctic Circle the sun
// may never rise or never set, and dusk reports that and exits 0.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"time"

	"github.com/philoserf/dusk/v3"
)

// errUsage reports a command line the tool cannot act on.
var errUsage = errors.New("usage")

// errNoBuildInfo reports a binary carrying no embedded build information,
// which happens only to one built in a way `go build` does not.
var errNoBuildInfo = errors.New("no build information is embedded")

// errUnsupportedDate reports a run that was asked for correctly and cannot be
// delivered. The command line was well formed; what failed was the date, and a
// reader told "usage" goes looking for a flag he typed wrong.
var errUnsupportedDate = errors.New("unsupported date")

func main() {
	err := run(os.Args[1:], os.Stdout, os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dusk:", err)
		os.Exit(1)
	}
}

// run is the whole program. main only supplies the real streams and the exit
// status, which keeps every branch below reachable from a test.
//
// stdout is the data channel - the report, the version - and stderr is where
// the tool talks to whoever is driving it, so that `dusk ... --json | jq`
// pipes a record and not a complaint.
func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("dusk", flag.ContinueOnError)

	// The flag package writes its own error and usage block on a bad flag.
	// Discarded here so that a failure is reported once, by main, in the same
	// shape as every other failure.
	fs.SetOutput(io.Discard)

	lat := fs.Float64("lat", 0, "latitude in degrees, north positive")
	lon := fs.Float64("lon", 0, "longitude in degrees, east positive")
	tz := fs.String("tz", "", "IANA timezone name, for example America/Detroit")
	dateArg := fs.String("date", "", "date as YYYY-MM-DD (default: today, in the observer's zone)")
	asJSON := fs.Bool("json", false, "emit JSON instead of a text report")
	showVersion := fs.Bool("version", false, "print the build and exit")

	err := fs.Parse(args)
	if err != nil {
		// Help asked for is not a misuse: it leaves by stdout and exits 0.
		if errors.Is(err, flag.ErrHelp) {
			return writeUsage(stdout, fs)
		}

		_ = writeUsage(stderr, fs)

		return fmt.Errorf("%w: %w", errUsage, err)
	}

	if *showVersion {
		return writeVersion(stdout)
	}

	obs, err := observerFromFlags(*lat, *lon, *tz, setFlags(fs))
	if err != nil {
		_ = writeUsage(stderr, fs)

		return err
	}

	// The date must be parsed in the observer's zone, not UTC: the library
	// derives the calendar day with date.In(observer location), so a UTC
	// midnight lands on the previous day for every observer west of Greenwich.
	date, err := parseDate(*dateArg, obs.Location())
	if err != nil {
		return err
	}

	day, err := buildReport(obs, date)
	if err != nil {
		// A date the library will not compute is not a misuse of the flags.
		if errors.Is(err, dusk.ErrDateOutOfRange) {
			return fmt.Errorf("%w: %w", errUnsupportedDate, err)
		}

		return err
	}

	if *asJSON {
		return renderJSON(stdout, day)
	}

	return renderText(stdout, day)
}

// setFlags reports which flags were actually given, so that an explicit
// --lat 0 is distinguishable from an omitted one.
func setFlags(fs *flag.FlagSet) map[string]bool {
	set := make(map[string]bool)

	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })

	return set
}

// observerFromFlags builds the observer from the coordinate flags. All three
// are required and none has a defensible default: a latitude of 0 is the
// equator, not "unset", and the zone decides which calendar day is meant.
func observerFromFlags(lat, lon float64, tz string, set map[string]bool) (dusk.Observer, error) {
	if !set["lat"] || !set["lon"] || !set["tz"] {
		return dusk.Observer{}, fmt.Errorf("%w: --lat, --lon, and --tz are all required", errUsage)
	}

	loc, err := time.LoadLocation(tz)
	if err != nil {
		return dusk.Observer{}, fmt.Errorf("%w: unknown timezone %q: %w", errUsage, tz, err)
	}

	obs, err := dusk.NewObserver(lat, lon, loc)
	if err != nil {
		return dusk.Observer{}, fmt.Errorf("%w: %w", errUsage, err)
	}

	return obs, nil
}

// parseDate parses --date in the observer's zone, defaulting to today there.
func parseDate(arg string, loc *time.Location) (time.Time, error) {
	if arg == "" {
		return time.Now().In(loc), nil
	}

	// Parsed without a zone, then anchored at midday in the observer's. A few
	// zones move their clocks at midnight - America/Santiago in September,
	// America/Havana in March - and there 00:00 does not exist, so
	// ParseInLocation resolves it to 23:00 the day before and the whole report
	// silently comes out for the previous day. Midday exists everywhere; the
	// library ignores the time of day and keeps only the calendar date.
	day, err := time.Parse(dateLayout, arg)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: --date %q is not YYYY-MM-DD: %w", errUsage, arg, err)
	}

	return time.Date(day.Year(), day.Month(), day.Day(), 12, 0, 0, 0, loc), nil
}

// writeVersion reports the build.
//
// A binary built from a working tree carries no version, so it reads
// "(devel)" - the same answer `go version -m` gives, rather than a number
// that would be wrong.
func writeVersion(out io.Writer) error {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return errNoBuildInfo
	}

	version := info.Main.Version
	if version == "" {
		version = "(devel)"
	}

	_, err := fmt.Fprintf(out, "%s %s\n", info.Main.Path, version)
	if err != nil {
		return fmt.Errorf("writing the version: %w", err)
	}

	return nil
}

// writeUsage writes the help text. The flag set's own defaults are printed
// through it, so the flags are described in exactly one place.
func writeUsage(w io.Writer, fs *flag.FlagSet) error {
	const help = `dusk - a day of astronomical events for one place and date

usage:
  dusk --lat DEG --lon DEG --tz ZONE [--date YYYY-MM-DD] [--json]
  dusk --version

flags:
`

	_, err := io.WriteString(w, help)
	if err != nil {
		return fmt.Errorf("writing the usage: %w", err)
	}

	fs.SetOutput(w)
	fs.PrintDefaults()
	fs.SetOutput(io.Discard)

	return nil
}
