package render

import "math"

// The placeholder art's colours at night and by day. Night is the spike's
// (#2) indigo at about 15%, inside the range it found smooth on the panels.
var (
	nightColour = eightBit(9, 4.5, 24)
	dayColour   = eightBit(40, 70, 100)
)

// placeholderArt fills the canvas with a vertical gradient, brighter at the
// top, whose colour follows the sun from night (below -12°) to day (above
// 20°). It only proves the layers; the art layer (#7) replaces it.
func placeholderArt(c *canvas, sc scene) {
	day := float32(smoothstep(-12, 20, sc.sun.Elevation))
	var top [3]float32
	for i := range top {
		top[i] = nightColour[i] + (dayColour[i]-nightColour[i])*day
	}
	for y := range c.height {
		// From 100% at the top to 60% at the bottom.
		k := float32(1)
		if c.height > 1 {
			k -= 0.4 * float32(y) / float32(c.height-1)
		}
		row := [3]float32{top[0] * k, top[1] * k, top[2] * k}
		for x := range c.width {
			c.set(x, y, row)
		}
	}
}

// staleColour is the stale marker's colour: a dim amber, visible over the
// art at any hour without drawing the eye.
var staleColour = eightBit(48, 16, 0)

// staleMarker lights the bottom-right pixel while any source is stale. It is
// the same for every source: it says to look at the logs, not which one.
func staleMarker(c *canvas, sc scene) {
	for _, h := range sc.state.Sources {
		if h.Stale(sc.t) {
			c.set(c.width-1, c.height-1, staleColour)
			return
		}
	}
}

// smoothstep rises smoothly from 0 at edge0 to 1 at edge1.
func smoothstep(edge0, edge1, x float64) float64 {
	t := math.Max(0, math.Min(1, (x-edge0)/(edge1-edge0)))
	return t * t * (3 - 2*t)
}
