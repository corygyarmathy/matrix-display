package main

import (
	"flag"
	"fmt"
	"math"
	"time"

	"github.com/corygyarmathy/matrix-display/internal/render"
)

// artFlags adds the art's flags to flags, for serve and preview. Call the
// function it returns after parsing them.
func artFlags(flags *flag.FlagSet) func() (render.Art, error) {
	d := render.DefaultArt
	ceiling := flags.Float64("ceiling", d.Ceiling, "the brightest the art gets, at noon, as a fraction of full brightness (below about 0.16, the day is dimmer than the night)")
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

// slowArt returns a with its motion slowed by a factor of by, for a clock
// running by times faster than real time, so that the art still moves at its
// own pace while the sun hurries. It fails when the art can't be drawn that
// slowly.
func slowArt(a render.Art, by float64) (render.Art, error) {
	// float64(math.MaxInt64) is 2^63, one past the largest Duration.
	if reshape := float64(a.Reshape) * by; reshape >= math.MaxInt64 {
		a.Reshape = math.MaxInt64
	} else {
		a.Reshape = time.Duration(reshape)
	}
	a.Drift /= by
	if err := a.Validate(); err != nil {
		return render.Art{}, err
	}
	return a, nil
}
