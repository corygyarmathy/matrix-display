// Package render computes the display's frames. Render is pure: the same
// time and State always give the same Frame, so the preview, tests and the
// live loop all see the same picture.
package render

import (
	"maps"
	"math"
	"time"

	"github.com/corygyarmathy/matrix-display/internal/sun"
)

// Frame is one picture for the sink, in the protocol's layout (see
// sinkproto.AppendFrame): rows top to bottom, each pixel red, green, blue,
// with 65535 as full brightness.
type Frame struct {
	Width, Height int
	Pix           []uint16
}

// At returns the pixel at (x, y).
func (f Frame) At(x, y int) [3]uint16 {
	p := f.Pix[3*(y*f.Width+x):]
	return [3]uint16{p[0], p[1], p[2]}
}

// State is what the sources know, as the renderer sees it for one frame.
// Sources replace its fields rather than change what they hold, so a shallow
// copy with Sources cloned is a snapshot.
type State struct {
	// Sources is each source's health, by name.
	Sources map[string]Health
}

// Clone returns a copy that shares nothing the sources change.
func (s State) Clone() State {
	s.Sources = maps.Clone(s.Sources)
	return s
}

// Health is how up to date one source is.
type Health struct {
	// LastOK is when the source last fetched successfully, or when it
	// started, if it never has.
	LastOK time.Time
	// StaleAfter is how long the source can go without a successful fetch
	// before the display says so.
	StaleAfter time.Duration
}

// Stale reports whether, at t, the source has gone too long without a
// successful fetch.
func (h Health) Stale(t time.Time) bool {
	return t.Sub(h.LastOK) > h.StaleAfter
}

// Renderer draws frames of one size, for one place.
type Renderer struct {
	Width, Height int
	Location      sun.Location
}

// scene is what every layer sees for one frame.
type scene struct {
	t     time.Time
	sun   sun.Position
	state State
}

// layer draws onto the canvas, over the layers before it.
type layer func(c *canvas, sc scene)

// layers in the order they compose. The sun's position is worked out first,
// into the scene. The presence fade (#8) will come last, after the overlays.
var layers = []layer{
	placeholderArt,
	staleMarker,
}

// Render draws the frame for time t and state s.
func (r Renderer) Render(t time.Time, s State) Frame {
	c := newCanvas(r.Width, r.Height)
	sc := scene{t: t, sun: sun.At(t, r.Location), state: s}
	for _, l := range layers {
		l(c, sc)
	}
	return c.frame()
}

// canvas is a frame in floating point, from 0 to 1 per channel. Layers work
// here and never round to 8 bits: the sink dithers from the full precision.
type canvas struct {
	width, height int
	pix           []float32
}

func newCanvas(width, height int) *canvas {
	return &canvas{width: width, height: height, pix: make([]float32, width*height*3)}
}

func (c *canvas) set(x, y int, rgb [3]float32) {
	copy(c.pix[3*(y*c.width+x):][:3], rgb[:])
}

// frame rounds the canvas to 16 bits, clamping to [0, 1].
func (c *canvas) frame() Frame {
	pix := make([]uint16, len(c.pix))
	for i, v := range c.pix {
		pix[i] = uint16(math.Round(float64(min(max(v, 0), 1)) * 65535))
	}
	return Frame{Width: c.width, Height: c.height, Pix: pix}
}

// eightBit is a colour given in the library's 8-bit terms, as the spike's
// findings are.
func eightBit(r, g, b float32) [3]float32 {
	return [3]float32{r / 255, g / 255, b / 255}
}
