package dusk

import (
	"math"
	"testing"
	"time"
)

func FuzzSunriseSunset(f *testing.F) {
	f.Add(40.7128, -74.006, int64(1710892800)) // NYC 2024-03-20
	f.Add(-33.87, 151.21, int64(1718928000))   // Sydney 2024-06-21
	f.Add(69.65, 18.96, int64(1718928000))     // Tromsø summer
	f.Add(-0.18, -78.47, int64(1710892800))    // Quito

	f.Fuzz(func(_ *testing.T, lat, lon float64, unix int64) {
		date := time.Unix(unix, 0).UTC()
		if date.Year() < 1800 || date.Year() > 2200 {
			return
		}

		obs, err := NewObserver(lat, lon, time.UTC)
		if err != nil {
			return // invalid coordinates rejected by NewObserver
		}

		_, err = SunriseSunset(date, obs)
		if err != nil {
			return // circumpolar or never-rises is valid
		}
	})
}

func FuzzLunarPhase(f *testing.F) {
	f.Add(int64(1704931200)) // 2024-01-11
	f.Add(int64(1706140800)) // 2024-01-25
	f.Add(int64(1710892800)) // 2024-03-20

	f.Fuzz(func(t *testing.T, unix int64) {
		date := time.Unix(unix, 0).UTC()
		if date.Year() < 1800 || date.Year() > 2200 {
			return
		}

		p, err := LunarPhase(date)
		if err != nil {
			return
		}

		if math.IsNaN(p.Illumination) {
			t.Error("NaN illumination")
		}

		if p.Illumination < 0 || p.Illumination > 100 {
			t.Errorf("illumination out of range: %f", p.Illumination)
		}
	})
}

func FuzzMoonriseMoonset(f *testing.F) {
	f.Add(40.7128, -74.006, int64(1705276800)) // NYC 2024-01-15
	f.Add(-33.87, 151.21, int64(1705276800))   // Sydney

	f.Fuzz(func(_ *testing.T, lat, lon float64, unix int64) {
		date := time.Unix(unix, 0).UTC()
		if date.Year() < 1800 || date.Year() > 2200 {
			return
		}

		obs, err := NewObserver(lat, lon, time.UTC)
		if err != nil {
			return // invalid coordinates rejected by NewObserver
		}

		_, err = MoonriseMoonset(date, obs)
		if err != nil {
			return
		}
	})
}
