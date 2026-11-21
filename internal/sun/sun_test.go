package sun

import (
	"math"
	"strings"
	"testing"
	"time"
)

// The reference positions are astropy's (6.x, its built-in ephemeris, no
// refraction) for Greenwich. They agree with published figures: London's
// sunrise on 21 June 2025 is 04:43 BST, when the sun's centre is 0.83° below
// the horizon.
func TestAtGreenwich(t *testing.T) {
	for _, tc := range []struct {
		name      string
		at        string
		elevation float64
		azimuth   float64
	}{
		{"equinox noon", "2025-03-20T12:00:00Z", 38.546, 177.647},
		{"solstice sunrise", "2025-06-21T03:43:00Z", -0.809, 48.975},
		{"midnight", "2025-12-21T00:00:00Z", -61.958, 1.007},
		{"equinox sunset", "2025-09-22T18:00:00Z", -1.158, 271.461},
	} {
		at, err := time.Parse(time.RFC3339, tc.at)
		if err != nil {
			t.Fatal(err)
		}
		got := At(at, Greenwich)
		if math.Abs(got.Elevation-tc.elevation) > 0.5 || angleDiff(got.Azimuth, tc.azimuth) > 0.5 {
			t.Errorf("%s: At = %+v, want elevation %.3f, azimuth %.3f, within 0.5°",
				tc.name, got, tc.elevation, tc.azimuth)
		}
	}
}

// The same instant in any time zone is the same position.
func TestAtIgnoresZone(t *testing.T) {
	utc := time.Date(2025, 6, 21, 23, 30, 0, 0, time.UTC)
	if a, b := At(utc, Greenwich), At(utc.In(time.FixedZone("UTC+10", 10*3600)), Greenwich); a != b {
		t.Errorf("At in UTC = %+v, in UTC+10 = %+v", a, b)
	}
}

func angleDiff(a, b float64) float64 {
	d := math.Mod(math.Abs(a-b), 360)
	return math.Min(d, 360-d)
}

func TestParseLocation(t *testing.T) {
	for _, in := range []string{"51.4769, -0.0005", "51.4769 -0.0005\n", "# Greenwich\n51.4769\n-0.0005\n"} {
		got, err := parseLocation(strings.NewReader(in))
		if err != nil || got != Greenwich {
			t.Errorf("parseLocation(%q) = %+v, %v; want Greenwich", in, got, err)
		}
	}
}

// A bad file's error never echoes its contents, which come from a secret.
func TestParseLocationErrorsHideContents(t *testing.T) {
	for _, in := range []string{"", "51.4769", "51.4769x, -0.0005", "91.25, 0", "51.4769, -0.0005, 7", "NaN, 0", "51.4769, nan"} {
		_, err := parseLocation(strings.NewReader(in))
		if err == nil {
			t.Errorf("parseLocation(%q) succeeded, want an error", in)
			continue
		}
		for _, field := range strings.FieldsFunc(in, func(r rune) bool { return r == ',' || r == ' ' }) {
			if len(field) > 1 && strings.Contains(err.Error(), field) {
				t.Errorf("parseLocation(%q) = %q, which quotes the file", in, err)
			}
		}
	}
}
