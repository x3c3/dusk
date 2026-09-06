package dusk_test

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/philoserf/dusk/v4"
)

func ExampleNewObserver() {
	// Valid observer
	obs, err := dusk.NewObserver(40.7128, -74.006, time.UTC)
	if err != nil {
		fmt.Println("unexpected error:", err)

		return
	}

	fmt.Println(obs)

	// Invalid: latitude out of range
	_, err = dusk.NewObserver(91, 0, time.UTC)
	fmt.Println(err)

	// Invalid: NaN
	_, err = dusk.NewObserver(math.NaN(), 0, time.UTC)
	fmt.Println(err)
	// Output:
	// 40.7128°, -74.0060° (UTC)
	// dusk: latitude must be in [-90, 90] and longitude in [-180, 180]
	// dusk: coordinates must be finite (NaN and Inf are not allowed)
}

func ExampleSunriseSunset_polar() {
	loc, err := time.LoadLocation("Europe/Oslo")
	if err != nil {
		fmt.Println("error:", err)

		return
	}

	obs, err := dusk.NewObserver(69.65, 18.96, loc)
	if err != nil {
		fmt.Println("error:", err)

		return
	}

	// Tromsø on June 21 — midnight sun
	date := time.Date(2024, 6, 21, 0, 0, 0, 0, loc)

	_, err = dusk.SunriseSunset(date, obs)
	if errors.Is(err, dusk.ErrCircumpolar) {
		fmt.Println("Midnight sun — no sunrise or sunset")
	}

	// Tromsø on December 21 — polar night
	date = time.Date(2024, 12, 21, 0, 0, 0, 0, loc)

	_, err = dusk.SunriseSunset(date, obs)
	if errors.Is(err, dusk.ErrNeverRises) {
		fmt.Println("Polar night — sun never rises")
	}
	// Output:
	// Midnight sun — no sunrise or sunset
	// Polar night — sun never rises
}

func ExampleSunriseSunset() {
	// Grand Rapids, Michigan, which keeps Eastern time.
	loc, err := time.LoadLocation("America/Detroit")
	if err != nil {
		fmt.Println("error:", err)

		return
	}

	date := time.Date(2025, 6, 21, 0, 0, 0, 0, loc)

	obs, err := dusk.NewObserver(42.9634, -85.6681, loc)
	if err != nil {
		fmt.Println("error:", err)

		return
	}

	sun, err := dusk.SunriseSunset(date, obs)
	if err != nil {
		fmt.Println("error:", err)

		return
	}

	fmt.Printf("Sunrise: %s\n", sun.Rise.Format("15:04"))
	fmt.Printf("Sunset:  %s\n", sun.Set.Format("15:04"))
	// Output:
	// Sunrise: 06:03
	// Sunset:  21:25
}

func ExampleLunarPhase() {
	date := time.Date(2024, 1, 25, 18, 0, 0, 0, time.UTC)

	phase, err := dusk.LunarPhase(date)
	if err != nil {
		fmt.Printf("error: %v\n", err)

		return
	}

	fmt.Printf("Phase: %s\n", phase.Name)
	fmt.Printf("Illumination: %.0f%%\n", phase.Illumination)
	// Output:
	// Phase: Full Moon
	// Illumination: 100%
}

func ExampleCivilTwilight() {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		fmt.Println("error:", err)

		return
	}

	date := time.Date(2025, 6, 21, 0, 0, 0, 0, loc)

	obs, err := dusk.NewObserver(47.6062, -122.3321, loc)
	if err != nil {
		fmt.Println("error:", err)

		return
	}

	tw, err := dusk.CivilTwilight(date, obs)
	if err != nil {
		fmt.Println("error:", err)

		return
	}

	fmt.Printf("Dusk: %s\n", tw.Dusk.Format("15:04"))
	fmt.Printf("Dawn: %s\n", tw.Dawn.Format("15:04"))
	// Output:
	// Dusk: 21:51
	// Dawn: 04:31
}

func ExampleMoonriseMoonset() {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		fmt.Println("error:", err)

		return
	}

	date := time.Date(2024, 1, 15, 0, 0, 0, 0, loc)

	obs, err := dusk.NewObserver(40.7128, -74.0060, loc)
	if err != nil {
		fmt.Println("error:", err)

		return
	}

	evt, err := dusk.MoonriseMoonset(date, obs)
	if err != nil {
		fmt.Println("error:", err)

		return
	}

	if !evt.Rise.IsZero() {
		fmt.Printf("Moonrise: %s\n", evt.Rise.Format("15:04"))
	}

	if !evt.Set.IsZero() {
		fmt.Printf("Moonset:  %s\n", evt.Set.Format("15:04"))
	}
	// Output:
	// Moonrise: 10:06
	// Moonset:  22:13
}
