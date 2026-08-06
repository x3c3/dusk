package dusk

import (
	"testing"
	"time"
)

// benchEST is a fixed UTC-5 zone used by benchmarks to avoid depending on the
// system timezone database.
var benchEST = time.FixedZone("EST", -5*3600)

// benchObs is a pre-validated observer for benchmark use.
var benchObs, _ = NewObserver(40.7128, -74.006, benchEST)

func BenchmarkMoonriseMoonset(b *testing.B) {
	date := time.Date(2024, 1, 15, 0, 0, 0, 0, benchEST)

	for b.Loop() {
		MoonriseMoonset(date, benchObs) //nolint:errcheck // benchmark discards the result, timing is what matters
	}
}

func BenchmarkSunriseSunset(b *testing.B) {
	date := time.Date(2024, 3, 20, 0, 0, 0, 0, benchEST)

	for b.Loop() {
		SunriseSunset(date, benchObs) //nolint:errcheck // benchmark discards the result, timing is what matters
	}
}
