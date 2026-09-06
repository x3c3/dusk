// Package dusk provides astronomical calculations: twilight times,
// sunrise/sunset, moonrise/moonset, and lunar phase.
//
// All angles are in degrees. Time parameters use [time.Time].
// Functions that produce local times accept an [Observer] with a timezone.
//
// Two sentinel errors distinguish polar edge cases:
// [ErrCircumpolar] (object always above the horizon) and
// [ErrNeverRises] (object never rises).
//
// Zero-value [time.Time] in result structs signals "event did not occur"
// for a specific day (e.g., the Moon rises but does not set before midnight).
// Check with [time.Time.IsZero]. This is distinct from sentinel errors,
// which indicate the geometry makes the event impossible at the given latitude.
//
// # References
//
//   - Meeus, Jean. Astronomical Algorithms. 2nd ed. Willmann-Bell, 1998.
package dusk

import (
	"fmt"
	"math"
	"time"
)

// stringError is an immutable error type used for sentinel errors.
// Unlike errors.New, these can be declared as constants.
type stringError string

func (e stringError) Error() string { return string(e) }

// ErrCircumpolar is returned when a celestial object is circumpolar
// (always above the horizon) at the given latitude.
const ErrCircumpolar = stringError("dusk: object is circumpolar (always above the horizon)")

// ErrNeverRises is returned when a celestial object never rises above
// the horizon at the given latitude.
const ErrNeverRises = stringError("dusk: object never rises at this latitude")

// ErrNilLocation is returned when a nil *time.Location is passed to
// [NewObserver].
const ErrNilLocation = stringError("dusk: location must not be nil")

// ErrNonFiniteCoord is returned when NaN or Inf coordinates are passed
// to [NewObserver].
const ErrNonFiniteCoord = stringError("dusk: coordinates must be finite (NaN and Inf are not allowed)")

// ErrInvalidCoord is returned when latitude or longitude are outside
// the valid range in [NewObserver].
const ErrInvalidCoord = stringError("dusk: latitude must be in [-90, 90] and longitude in [-180, 180]")

// validObserver returns an error if obs was not constructed via NewObserver
// (i.e., is a zero-value Observer with a nil location).
func validObserver(obs Observer) error {
	if obs.loc == nil {
		return ErrNilLocation
	}

	return nil
}

// Observer represents a geographic position on Earth used as the viewpoint
// for all astronomical calculations.
type Observer struct {
	lat float64
	lon float64
	loc *time.Location
}

// NewObserver constructs an Observer after validating all inputs.
// lat must be in [-90, 90], lon in [-180, 180], and loc must not be nil.
// NaN and infinite values are rejected.
func NewObserver(lat, lon float64, loc *time.Location) (Observer, error) {
	if loc == nil {
		return Observer{}, ErrNilLocation
	}

	if math.IsNaN(lat) || math.IsInf(lat, 0) || math.IsNaN(lon) || math.IsInf(lon, 0) {
		return Observer{}, ErrNonFiniteCoord
	}

	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return Observer{}, ErrInvalidCoord
	}

	return Observer{lat: lat, lon: lon, loc: loc}, nil
}

// Lat returns the observer's latitude in degrees.
func (o Observer) Lat() float64 { return o.lat }

// Lon returns the observer's longitude in degrees (east positive, west negative).
func (o Observer) Lon() float64 { return o.lon }

// Location returns the observer's timezone.
func (o Observer) Location() *time.Location { return o.loc }

// String returns a human-readable representation of the observer.
func (o Observer) String() string {
	locName := "nil"
	if o.loc != nil {
		locName = o.loc.String()
	}

	return fmt.Sprintf("%.4f°, %.4f° (%s)", o.lat, o.lon, locName)
}

// SunEvent holds the times of sunrise, solar noon, sunset, and the duration
// of daylight for a single day.
type SunEvent struct {
	Rise     time.Time
	Noon     time.Time
	Set      time.Time
	Duration time.Duration
}

// MoonEvent holds the rise and set times for the Moon on a given day, along
// with the duration between rise and set.
type MoonEvent struct {
	Rise         time.Time // zero value if the Moon does not rise
	Set          time.Time // zero value if the Moon does not set
	AboveHorizon bool      // true if Moon was above the horizon at start of day
}

// TwilightEvent holds the dusk and dawn times of a twilight period.
// Dusk is tonight's boundary (sun passes below the depression angle).
// Dawn is tomorrow morning's boundary (sun passes above the depression angle).
// To get this morning's dawn, call with yesterday's date.
type TwilightEvent struct {
	Dusk          time.Time     // evening boundary (today)
	Dawn          time.Time     // morning boundary (tomorrow)
	NightDuration time.Duration // time from Dusk to Dawn (overnight darkness)
}

// LunarPhaseInfo describes the Moon's current phase.
type LunarPhaseInfo struct {
	Illumination float64 // percentage 0-100
	Elongation   float64 // degrees 0-360
	DaysApprox   float64 // rough days into lunation (linear estimate from elongation)
	Waxing       bool    // true from New Moon to Full Moon (elongation 0-180)
	Name         string  // "New Moon", "Waxing Crescent", etc.
}

// equatorial represents right ascension and declination in degrees.
// Used internally for coordinate conversions.
type equatorial struct {
	ra  float64
	dec float64
}

// horizontal represents altitude and azimuth in degrees.
// Used internally for coordinate conversions.
type horizontal struct {
	alt float64
	az  float64
}

// ecliptic represents ecliptic coordinates: longitude and latitude in degrees,
// and distance in kilometers.
// Used internally for lunar position calculations.
type ecliptic struct {
	lon  float64
	lat  float64
	dist float64
}
