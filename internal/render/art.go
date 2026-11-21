package render

import (
	"errors"
	"math"
	"time"
)

// Art is how the art layer looks and moves. The zero Art is DefaultArt, and
// in any other Art a zero Ceiling or Reshape is DefaultArt's.
type Art struct {
	// Ceiling is the brightest the field gets, at noon, as a fraction of
	// full colour value: the scale of the library's --led-brightness, which
	// the protocol has no field for. The night field is fixed, so the
	// ceiling dims or brightens the daylight hours only. Below about 0.16,
	// the night field's brightest, the day is dimmer than the night.
	Ceiling float64
	// EastRight puts east, where the sun rises, at the right-hand end of the
	// strip. Which end it is depends on which way the desk faces.
	EastRight bool
	// Reshape is how long the noise takes to change shape completely.
	Reshape time.Duration
	// Drift is how fast the field moves sideways, in pixels per second:
	// positive towards the right.
	Drift float64
}

// DefaultArt is the art direction's (#4): a noon ceiling of 40%, the noise
// reshaping every 24 s and drifting at 1 px/s, with east on the left.
var DefaultArt = Art{Ceiling: 0.4, Reshape: 24 * time.Second, Drift: 1}

// paletteCeiling is the ceiling the keyframes' colours are chosen at. Other
// ceilings scale the day's colours from it.
const paletteCeiling = 0.4

// Validate reports whether the art can be drawn.
func (a Art) Validate() error {
	if !(a.Ceiling > 0 && a.Ceiling <= 1) {
		return errors.New("the ceiling must be above 0 and at most 1")
	}
	if a.Reshape <= 0 {
		return errors.New("the reshape time must be positive")
	}
	if math.IsNaN(a.Drift) || math.IsInf(a.Drift, 0) {
		return errors.New("the drift must be a number")
	}
	return nil
}

// featureSize is about how far apart, in pixels, the field's features are:
// three along the strip.
const featureSize = 256.0 / 3

// glowWeight is how far the field is pulled towards the glow colour at the
// sun's end of the strip.
const glowWeight = 0.2

// artStep is the spacing, in pixels, of the grid the field is computed on.
// Between its points it is interpolated: the field's features are tens of
// pixels across, so this is invisible, and it is 16 times less work.
const artStep = 4

// keyframe is the field at one solar elevation: two colours the noise mixes
// between, and a warmer, brighter one it is pulled towards at the sun's end.
// Colours are in the library's 8-bit terms, at paletteCeiling.
type keyframe struct {
	elevation    float64
	lo, hi, glow [3]float32
}

// keyframes run from night to day. Between two of them the colours blend,
// in linear light, along a smoothstep, so there are no visible steps.
//
// The night field runs from (4, 2, 12) to (16, 7, 40), about 20% of the
// spike's (#2) indigo peak, well above the 3% it found smooth. Brightness
// (the mean luminance of lo and hi) never falls as the sun rises, up to the
// ceiling at day. Saturation peaks in twilight and golden hour, and the day
// stays a soft blue. The pairs are spread about 3× further apart than the
// art direction prototype's, about 2× in golden hour, and no channel is ever
// 0, so nothing clips to black or to a pure primary.
var keyframes = func() []keyframe {
	k := []keyframe{
		{-18, eightBit(4, 2, 12), eightBit(16, 7, 40), eightBit(20, 9, 42)},     // Night: indigo.
		{-12, eightBit(6, 4, 20), eightBit(28, 14, 64), eightBit(36, 16, 62)},   // Nautical twilight.
		{-6, eightBit(14, 10, 44), eightBit(64, 30, 86), eightBit(80, 36, 72)},  // Civil twilight: violet.
		{-1, eightBit(24, 20, 72), eightBit(96, 40, 64), eightBit(100, 46, 40)}, // The Belt of Venus: rose over violet.
		{3, eightBit(56, 32, 58), eightBit(100, 58, 16), eightBit(102, 62, 14)}, // Golden hour: amber.
		{10, eightBit(40, 52, 86), eightBit(100, 72, 30), eightBit(102, 76, 34)},
		{22, eightBit(30, 52, 88), eightBit(76, 94, 102), eightBit(86, 96, 102)}, // Day: soft blue.
	}
	// Blended in linear light.
	for i := range k {
		for _, c := range []*[3]float32{&k[i].lo, &k[i].hi, &k[i].glow} {
			for j := range c {
				c[j] = toLinear(c[j])
			}
		}
	}
	return k
}()

// palette returns the field's colours at the sun's elevation, in linear
// light, at paletteCeiling.
func palette(elevation float64) (lo, hi, glow [3]float32) {
	i := 0
	for i < len(keyframes)-2 && elevation > keyframes[i+1].elevation {
		i++
	}
	a, b := keyframes[i], keyframes[i+1]
	t := float32(smoothstep(a.elevation, b.elevation, elevation))
	return lerp3(a.lo, b.lo, t), lerp3(a.hi, b.hi, t), lerp3(a.glow, b.glow, t)
}

// artLayer draws the field. It keeps the grid the field is computed on
// between frames, so that drawing allocates nothing.
type artLayer struct {
	Art
	// cols × rows grid points, artStep pixels apart, covering the canvas.
	cols, rows int
	grid       [][3]float32
	glow       []float32    // The glow's weight, by grid column.
	row        [][3]float32 // One row of the canvas's, by grid column.
}

func newArtLayer(a Art, width, height int) *artLayer {
	if a == (Art{}) {
		a = DefaultArt
	}
	if a.Ceiling == 0 {
		a.Ceiling = DefaultArt.Ceiling
	}
	if a.Reshape == 0 {
		a.Reshape = DefaultArt.Reshape
	}
	cols, rows := (width-1)/artStep+2, (height-1)/artStep+2
	return &artLayer{Art: a, cols: cols, rows: rows, grid: make([][3]float32, cols*rows), glow: make([]float32, cols), row: make([][3]float32, cols)}
}

// draw fills the canvas with two colours of the hour, mixed by noise that
// drifts and reshapes over time, pulled warmer and brighter towards the
// sun's end of the strip. It is one seamless field: nothing in it knows
// where the panels meet.
func (a *artLayer) draw(c *canvas, sc scene) {
	lo, hi, glow := palette(sc.sun.Elevation)

	// The sun's position along the strip, from 0 at the east end to 1 at
	// the west, follows its east–west bearing. Using the sine keeps it
	// continuous as the sun passes north at night.
	along := (1 - math.Sin(sc.sun.Azimuth*math.Pi/180)) / 2
	if a.EastRight {
		along = 1 - along
	}
	sunX := along * float64(c.width-1)
	for i := range a.glow {
		d := (float64(i*artStep) - sunX) / float64(c.width)
		a.glow[i] = float32(glowWeight * math.Exp(-6*d*d))
	}

	// The ceiling scales the day, not the night, which is set in absolute
	// terms by what the panels show smoothly.
	scale := float32(1 + (a.Ceiling/paletteCeiling-1)*smoothstep(-18, 22, sc.sun.Elevation))

	// Time, in noise units: the field reshapes once per unit of z.
	s := float64(sc.t.UnixNano()) / 1e9
	z := s / a.Reshape.Seconds()
	shift := a.Drift * s
	for j := range a.rows {
		ny := float64(j*artStep) / featureSize
		for i := range a.cols {
			nx := (float64(i*artStep) - shift) / featureSize
			t := float32(fieldNoise(nx, ny, z))
			col := lerp3(lerp3(lo, hi, t), glow, a.glow[i])
			for k := range col {
				col[k] = fromLinear(col[k]) * scale
			}
			a.grid[j*a.cols+i] = col
		}
	}

	// Interpolate the grid onto the canvas: down to each row, then across.
	for y := range c.height {
		j, fy := y/artStep, float32(y%artStep)/artStep
		above, below := a.grid[j*a.cols:], a.grid[(j+1)*a.cols:]
		for i := range a.row {
			a.row[i] = lerp3(above[i], below[i], fy)
		}
		pix := c.pix[3*y*c.width : 3*(y+1)*c.width]
		for x := range c.width {
			i, fx := x/artStep, float32(x%artStep)/artStep
			l, r := a.row[i], a.row[i+1]
			p := pix[3*x : 3*x+3 : 3*x+3]
			p[0] = l[0] + (r[0]-l[0])*fx
			p[1] = l[1] + (r[1]-l[1])*fx
			p[2] = l[2] + (r[2]-l[2])*fx
		}
	}
}

// fieldNoise is smooth noise from 0 to 1: two octaves of value noise,
// stretched so the field reaches both of its colours.
func fieldNoise(x, y, z float64) float64 {
	n := 0.7*valueNoise(x, y, z) + 0.3*valueNoise(2.1*x+17, 2.1*y+5, 1.7*z)
	return smoothstep(noiseLow, noiseHigh, n)
}

// The noise's sum is mostly near 0.5. Between noiseLow and noiseHigh is
// most of its range, which the stretch in fieldNoise maps onto 0 to 1.
const noiseLow, noiseHigh = 0.2, 0.8

// valueNoise is 3D value noise: random values at the integer lattice,
// blended along smoothsteps. It is 0 to 1 and never repeats in practice.
func valueNoise(x, y, z float64) float64 {
	xf, yf, zf := math.Floor(x), math.Floor(y), math.Floor(z)
	xi, yi, zi := int64(xf), int64(yf), int64(zf)
	u, v, w := smooth(x-xf), smooth(y-yf), smooth(z-zf)
	corner := func(dx, dy, dz int64) float64 { return hash(xi+dx, yi+dy, zi+dz) }
	return lerp(
		lerp(lerp(corner(0, 0, 0), corner(1, 0, 0), u), lerp(corner(0, 1, 0), corner(1, 1, 0), u), v),
		lerp(lerp(corner(0, 0, 1), corner(1, 0, 1), u), lerp(corner(0, 1, 1), corner(1, 1, 1), u), v),
		w,
	)
}

// hash is a random value from 0 to 1 for a lattice point.
func hash(x, y, z int64) float64 {
	h := uint64(x)*0x9e3779b97f4a7c15 ^ uint64(y)*0xc2b2ae3d27d4eb4f ^ uint64(z)*0x165667b19e3779f9
	h ^= h >> 31
	h *= 0xbf58476d1ce4e5b9
	h ^= h >> 29
	h *= 0x94d049bb133111eb
	h ^= h >> 32
	return float64(h>>11) / (1 << 53)
}

// smooth is smoothstep from 0 to 1, for t already in [0, 1].
func smooth(t float64) float64 { return t * t * (3 - 2*t) }

// The field is blended in linear light and drawn in the library's terms.
// The gamma is 2, close to the prototype's 2.2 and far cheaper: a square
// root is one instruction, where a power is two logarithms' worth.
func toLinear(v float32) float32   { return v * v }
func fromLinear(v float32) float32 { return float32(math.Sqrt(float64(max(v, 0)))) }

func lerp[T float32 | float64](a, b, t T) T { return a + (b-a)*t }

func lerp3(a, b [3]float32, t float32) [3]float32 {
	return [3]float32{lerp(a[0], b[0], t), lerp(a[1], b[1], t), lerp(a[2], b[2], t)}
}
