package render

import (
	"math"
	"testing"
	"time"

	"github.com/corygyarmathy/matrix-display/internal/sun"
)

// Times at Greenwich on the 2025 March equinox, when the sun rises due east
// and sets due west.
var (
	equinoxNight = time.Date(2025, 3, 20, 0, 0, 0, 0, time.UTC)  // About -38°.
	equinoxDawn  = time.Date(2025, 3, 20, 5, 45, 0, 0, time.UTC) // About -2°, rising.
	equinoxNoon  = time.Date(2025, 3, 20, 12, 7, 0, 0, time.UTC) // About 38°.
	equinoxDusk  = time.Date(2025, 3, 20, 17, 50, 0, 0, time.UTC)
)

// The field moves too slowly to see from one frame to the next at the
// sink's rate, but has clearly changed a minute later.
func TestArtMovesSlowly(t *testing.T) {
	const fps = 98
	for _, at := range []time.Time{equinoxNight, equinoxDawn, equinoxNoon, equinoxDusk} {
		f := greenwich.Render(at, State{})
		next := greenwich.Render(at.Add(time.Second/fps), State{})
		later := greenwich.Render(at.Add(time.Minute), State{})
		// A tenth of an 8-bit step: far below what the sink's dithering
		// can show from one refresh to the next.
		if d := maxDiff(f, next); d > 257/10 {
			t.Errorf("at %s, consecutive frames differ by up to %d, want at most %d", at.Format(time.Kitchen), d, 257/10)
		}
		// Relative to the field's brightness, so that night counts too.
		if d, mean := meanDiff(f, later), meanLevel(f); d < 0.1*mean {
			t.Errorf("at %s, frames a minute apart differ by %.0f on average, want at least 10%% of the mean level %.0f",
				at.Format(time.Kitchen), d, mean)
		}
	}
}

// Reshape and Drift set the pace.
func TestArtPaceIsConfigurable(t *testing.T) {
	slow := Renderer{Width: width, Height: height, Location: sun.Greenwich, Art: Art{Ceiling: 0.4, Reshape: 240 * time.Second, Drift: 0.1}}
	fast := greenwich
	if a, b := meanDiff(slow.Render(equinoxNoon, State{}), slow.Render(equinoxNoon.Add(10*time.Second), State{})),
		meanDiff(fast.Render(equinoxNoon, State{}), fast.Render(equinoxNoon.Add(10*time.Second), State{})); a*3 > b {
		t.Errorf("ten times slower art changed by %.0f in 10 s, against %.0f: want under a third", a, b)
	}
}

// Between keyframes the palette has no steps: a hundredth of a degree, which
// takes the sun several seconds, moves no colour by more than a quarter of
// an 8-bit step.
func TestPaletteBlendsSmoothly(t *testing.T) {
	const step = 0.01
	plo, phi, pglow := palette(-90)
	for el := -90 + step; el <= 90; el += step {
		lo, hi, glow := palette(el)
		for i, pair := range [][2][3]float32{{plo, lo}, {phi, hi}, {pglow, glow}} {
			for c := range 3 {
				if d := math.Abs(float64(fromLinear(pair[0][c]) - fromLinear(pair[1][c]))); d > 0.25/255 {
					t.Fatalf("at %.2f°, colour %d channel %d jumps by %.3f of an 8-bit step", el, i, c, d*255)
				}
			}
		}
		plo, phi, pglow = lo, hi, glow
	}
}

// No colour has a channel at 0, so the field never reaches black and never
// clips to a pure primary.
func TestPaletteNeverClips(t *testing.T) {
	for el := -90.0; el <= 90; el += 0.25 {
		lo, hi, glow := palette(el)
		for _, c := range [][3]float32{lo, hi, glow} {
			for _, v := range c {
				if v8 := fromLinear(v) * 255; v8 < 1.5 || v8 > 0.4*255+0.01 {
					t.Fatalf("at %.2f°, %v has a channel at %.2f of 255, want from 1.5 to the ceiling", el, eightBits(c), v8)
				}
			}
		}
	}
}

// Brightness never falls as the sun rises.
func TestPaletteBrightensWithTheSun(t *testing.T) {
	last := 0.0
	for el := -90.0; el <= 90; el += 0.25 {
		lo, hi, _ := palette(el)
		l := (luminance(lo) + luminance(hi)) / 2
		if l < last-1e-9 {
			t.Fatalf("at %.2f°, luminance %.5f, down from %.5f", el, l, last)
		}
		last = l
	}
}

// Saturation peaks in twilight and golden hour, and the day stays coloured.
func TestPaletteSaturation(t *testing.T) {
	day := saturation(30)
	if day < 0.3 {
		t.Errorf("by day, saturation %.2f, want at least 0.3: never white", day)
	}
	for _, el := range []float64{-6, -3, 0, 3, 6, 10} {
		if s := saturation(el); s < day+0.1 {
			t.Errorf("at %.0f°, saturation %.2f, want well above the day's %.2f", el, s, day)
		}
	}
}

// The two colours are far enough apart that the field visibly varies at
// every hour: about three times the spread of the art direction
// prototype's first palettes, which measure about 0.06 here by day, and
// twice it in golden hour, which spreads further anyway.
func TestArtVariesAtEveryHour(t *testing.T) {
	for h := range 24 {
		// Over ten minutes, since one frame can fall mostly in one colour.
		var mean float64
		for m := range 10 {
			at := equinoxNight.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute)
			mean += spread(greenwich.Render(at, State{})) / 10
		}
		if mean < 0.14 {
			t.Errorf("at %02d:00, the field's colours spread %.2f of its level, want at least 0.14", h, mean)
		}
	}
}

// The night field ranges from about (4, 2, 12) to (16, 7, 40), and no pixel
// is black.
func TestNightField(t *testing.T) {
	f := greenwich.Render(equinoxNight, State{})
	lo, hi := [3]float64{255, 255, 255}, [3]float64{}
	for y := range f.Height {
		for x := range f.Width {
			for c, v := range f.At(x, y) {
				lo[c], hi[c] = min(lo[c], float64(v)/257), max(hi[c], float64(v)/257)
			}
		}
	}
	for c, want := range [][2][3]float64{{{3, 1.5, 10}, {8, 4, 22}}, {{12, 5, 32}, {21, 10, 44}}} {
		got := [][3]float64{lo, hi}[c]
		for i := range 3 {
			if got[i] < want[0][i] || got[i] > want[1][i] {
				t.Errorf("the night field's %s is %.1f, want it within %.1f to %.1f",
					[]string{"darkest", "brightest"}[c], got, want[0], want[1])
				break
			}
		}
	}
}

// At noon, the brightest channel is near the ceiling and never above it,
// and the ceiling scales the day.
func TestNoonCeiling(t *testing.T) {
	for _, ceiling := range []float64{0.4, 0.2, 0.8} {
		r := greenwich
		r.Art = DefaultArt
		r.Art.Ceiling = ceiling
		f := r.Render(equinoxNoon, State{})
		peak := float64(maxValue(f)) / 65535
		if peak > ceiling+0.5/255 || peak < 0.85*ceiling {
			t.Errorf("with a ceiling of %.2f, the noon field peaks at %.3f", ceiling, peak)
		}
	}
	// The night doesn't change with it.
	dim := greenwich
	dim.Art = DefaultArt
	dim.Art.Ceiling = 0.1
	if d := maxDiff(dim.Render(equinoxNight, State{}), greenwich.Render(equinoxNight, State{})); d > 0 {
		t.Errorf("a lower ceiling changed the night field by up to %d", d)
	}
}

// An Art with only some fields set takes DefaultArt's Ceiling and Reshape
// for the rest, rather than drawing black.
func TestPartialArtDefaults(t *testing.T) {
	partial := greenwich
	partial.Art = Art{Drift: DefaultArt.Drift}
	if d := maxDiff(partial.Render(equinoxNoon, State{}), greenwich.Render(equinoxNoon, State{})); d > 0 {
		t.Errorf("Art{Drift: %g} differs from DefaultArt by up to %d", DefaultArt.Drift, d)
	}
}

// The sun's end of the strip is warmer and brighter, and the other end is
// still lit. At the equinox the sun rises due east and sets due west.
func TestAzimuthBias(t *testing.T) {
	eastRight := greenwich
	eastRight.Art = DefaultArt
	eastRight.Art.EastRight = true
	for _, c := range []struct {
		name     string
		at       time.Time
		westward bool
	}{{"dawn", equinoxDawn, false}, {"dusk", equinoxDusk, true}} {
		left := greenwich.Render(c.at, State{})
		right := eastRight.Render(c.at, State{})
		// With east on the left, the sun's end is the left at dawn and
		// the right at dusk. Swapping the ends must move the glow, and the
		// noise is the same in both, so only the glow differs.
		sunny, shady := left, right
		if c.westward {
			sunny, shady = right, left
		}
		sl, sw := endLevels(sunny, 0, 32)
		dl, dw := endLevels(shady, 0, 32)
		if sl <= dl || sw <= dw {
			t.Errorf("%s: the left end with the sun is level %.0f and warmth %.0f, without it %.0f and %.0f: want both higher with it",
				c.name, sl, sw, dl, dw)
		}
		far, _ := endLevels(sunny, width-32, width)
		if far < 0.6*sl {
			t.Errorf("%s: the far end is level %.0f against the sun's end %.0f, want at least 60%%", c.name, far, sl)
		}
	}
}

func BenchmarkLive(b *testing.B) {
	live := greenwich.Live()
	pix := make([]uint16, width*height*3)
	at := equinoxDusk
	for b.Loop() {
		live.RenderInto(pix, at, State{})
		at = at.Add(time.Second / 98)
	}
}

// saturation is the mean HSV saturation of the field's two colours at an
// elevation.
func saturation(el float64) float64 {
	lo, hi, _ := palette(el)
	var s float64
	for _, c := range [][3]float32{lo, hi} {
		v := eightBits(c)
		mx, mn := max(v[0], v[1], v[2]), min(v[0], v[1], v[2])
		s += (mx - mn) / mx / 2
	}
	return s
}

func eightBits(c [3]float32) [3]float64 {
	return [3]float64{float64(fromLinear(c[0])) * 255, float64(fromLinear(c[1])) * 255, float64(fromLinear(c[2])) * 255}
}

// luminance of a linear colour, by Rec. 709.
func luminance(c [3]float32) float64 {
	return 0.2126*float64(c[0]) + 0.7152*float64(c[1]) + 0.0722*float64(c[2])
}

// endLevels returns the mean level, and the mean of red less blue, of
// columns x0 to x1.
func endLevels(f Frame, x0, x1 int) (level, warmth float64) {
	n := 0.0
	for y := range f.Height {
		for x := x0; x < x1; x++ {
			p := f.At(x, y)
			level += (float64(p[0]) + float64(p[1]) + float64(p[2])) / 3
			warmth += float64(p[0]) - float64(p[2])
			n++
		}
	}
	return level / n, warmth / n
}

// spread is the root mean square distance of the pixels from the frame's
// mean colour, relative to its mean level.
func spread(f Frame) float64 {
	n := float64(f.Width * f.Height)
	var mean [3]float64
	for i, v := range f.Pix {
		mean[i%3] += float64(v) / n
	}
	var sq float64
	for i, v := range f.Pix {
		d := float64(v) - mean[i%3]
		sq += d * d / n
	}
	return math.Sqrt(sq) / ((mean[0] + mean[1] + mean[2]) / 3)
}

func maxDiff(a, b Frame) int {
	d := 0
	for i := range a.Pix {
		d = max(d, abs(int(a.Pix[i])-int(b.Pix[i])))
	}
	return d
}

func meanDiff(a, b Frame) float64 {
	var sum float64
	for i := range a.Pix {
		sum += math.Abs(float64(a.Pix[i]) - float64(b.Pix[i]))
	}
	return sum / float64(len(a.Pix))
}

func maxValue(f Frame) uint16 {
	var m uint16
	for _, v := range f.Pix {
		m = max(m, v)
	}
	return m
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
