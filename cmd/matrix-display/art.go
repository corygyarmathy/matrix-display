package main

import (
	"flag"
	"fmt"

	"github.com/corygyarmathy/matrix-display/internal/render"
)

// artFlags adds the art's flags to flags, for serve and preview. Call the
// function it returns after parsing them.
func artFlags(flags *flag.FlagSet) func() (render.Art, error) {
	d := render.DefaultArt
	ceiling := flags.Float64("ceiling", d.Ceiling, "the brightest the art gets, at noon, as a fraction of full brightness")
	east := flags.String("east", "left", "the end of the strip that faces east: left or right")
	reshape := flags.Duration("reshape", d.Reshape, "how long the art's noise takes to change shape")
	drift := flags.Float64("drift", d.Drift, "how fast the art drifts sideways, in pixels per second")
	return func() (render.Art, error) {
		a := render.Art{Ceiling: *ceiling, Reshape: *reshape, Drift: *drift}
		switch *east {
		case "left":
		case "right":
			a.EastRight = true
		default:
			return render.Art{}, fmt.Errorf("--east must be left or right, not %q", *east)
		}
		if err := a.Validate(); err != nil {
			return render.Art{}, err
		}
		return a, nil
	}
}
