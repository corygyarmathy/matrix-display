// Package sun computes where the sun is in the sky, with NOAA's approximation
// (the equations behind its solar calculator spreadsheets), which is good to
// a fraction of a degree for years either side of 2000.
package sun

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

// Location is a place on Earth, in degrees: north and east are positive.
type Location struct {
	Latitude, Longitude float64
}

// Greenwich is the Royal Observatory: the location tests and fixtures use, so
// that no real one is ever committed.
var Greenwich = Location{Latitude: 51.4769, Longitude: -0.0005}

// Position is where the sun is, in degrees, as seen from a Location.
type Position struct {
	// Elevation is the geometric angle above the horizon, with no correction
	// for refraction: negative below it, -6 at the end of civil twilight.
	Elevation float64
	// Azimuth is the compass bearing, clockwise from north: 90 is east.
	Azimuth float64
}

// At returns the sun's position at t, seen from loc.
func At(t time.Time, loc Location) Position {
	t = t.UTC()
	jd := float64(t.UnixNano())/float64(24*time.Hour) + 2440587.5
	c := (jd - 2451545) / 36525 // Julian centuries since J2000.

	meanLong := math.Mod(280.46646+c*(36000.76983+c*0.0003032), 360)
	meanAnom := 357.52911 + c*(35999.05029-0.0001537*c)
	ecc := 0.016708634 - c*(0.000042037+0.0000001267*c)
	centre := sin(meanAnom)*(1.914602-c*(0.004817+0.000014*c)) +
		sin(2*meanAnom)*(0.019993-0.000101*c) +
		sin(3*meanAnom)*0.000289
	omega := 125.04 - 1934.136*c
	appLong := meanLong + centre - 0.00569 - 0.00478*sin(omega)
	meanObliq := 23 + (26+(21.448-c*(46.815+c*(0.00059-c*0.001813)))/60)/60
	obliq := meanObliq + 0.00256*cos(omega)
	decl := asin(sin(obliq) * sin(appLong))

	y := math.Pow(math.Tan(rad(obliq/2)), 2)
	eqTime := 4 * deg(y*sin(2*meanLong)-
		2*ecc*sin(meanAnom)+
		4*ecc*y*sin(meanAnom)*cos(2*meanLong)-
		0.5*y*y*sin(4*meanLong)-
		1.25*ecc*ecc*sin(2*meanAnom)) // In minutes.

	midnight := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	minutes := t.Sub(midnight).Minutes()
	solarTime := math.Mod(minutes+eqTime+4*loc.Longitude, 1440)
	if solarTime < 0 {
		solarTime += 1440
	}
	hourAngle := solarTime/4 - 180

	lat := loc.Latitude
	cosZenith := clamp(sin(lat)*sin(decl) + cos(lat)*cos(decl)*cos(hourAngle))
	zenith := acos(cosZenith)

	var azimuth float64
	if s := cos(lat) * sin(zenith); s != 0 {
		a := acos(clamp((sin(lat)*cosZenith - sin(decl)) / s))
		if hourAngle > 0 {
			azimuth = math.Mod(a+180, 360)
		} else {
			azimuth = math.Mod(540-a, 360)
		}
	}
	return Position{Elevation: 90 - zenith, Azimuth: azimuth}
}

// LoadLocation reads a location from the file at path: a latitude and a
// longitude in decimal degrees, separated by a comma or spaces, such as
// "51.4769, -0.0005". Lines starting with # are comments.
//
// The file comes from a secret, so its errors never quote what it holds.
func LoadLocation(path string) (Location, error) {
	f, err := os.Open(path)
	if err != nil {
		return Location{}, err
	}
	defer f.Close()
	loc, err := parseLocation(f)
	if err != nil {
		return Location{}, fmt.Errorf("%s: %w", path, err)
	}
	return loc, nil
}

func parseLocation(r io.Reader) (Location, error) {
	var fields []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields = append(fields, strings.FieldsFunc(line, func(r rune) bool {
			return r == ',' || r == ' ' || r == '\t'
		})...)
	}
	if err := scanner.Err(); err != nil {
		return Location{}, err
	}
	if len(fields) != 2 {
		return Location{}, fmt.Errorf("want a latitude and a longitude, found %d values", len(fields))
	}
	lat, err1 := strconv.ParseFloat(fields[0], 64)
	lon, err2 := strconv.ParseFloat(fields[1], 64)
	if err1 != nil || err2 != nil {
		return Location{}, errors.New("the latitude and longitude must be decimal numbers")
	}
	// Written so that NaN, which ParseFloat accepts, fails it too.
	if !(math.Abs(lat) <= 90 && math.Abs(lon) <= 180) {
		return Location{}, errors.New("the latitude must be within ±90 and the longitude within ±180")
	}
	return Location{Latitude: lat, Longitude: lon}, nil
}

func rad(d float64) float64  { return d * math.Pi / 180 }
func deg(r float64) float64  { return r * 180 / math.Pi }
func sin(d float64) float64  { return math.Sin(rad(d)) }
func cos(d float64) float64  { return math.Cos(rad(d)) }
func asin(x float64) float64 { return deg(math.Asin(x)) }
func acos(x float64) float64 { return deg(math.Acos(x)) }

// clamp keeps a cosine within [-1, 1] against rounding.
func clamp(x float64) float64 { return math.Max(-1, math.Min(1, x)) }
