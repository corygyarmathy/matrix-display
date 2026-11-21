package render

import "math"

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
