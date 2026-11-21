package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/corygyarmathy/matrix-display/internal/sinkproto"
)

// The spike's (#2) gradient colour, in 8-bit values.
var gradientColour = [3]float64{60, 30, 160}

// testPattern sends a pattern to the sink, at the size the sink reports, until
// ctx ends, for checking the panels by eye (#5):
//
//   - the top half has a bar sweeping left to right, whose edges tear if
//     frames aren't swapped on vsync;
//   - the bottom half is a gradient from black to the spike's indigo at
//     --brightness percent, which shows banding if dithering isn't working.
//
// When it stops, the sink should fade the panels to black.
func testPattern(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("test-pattern", flag.ContinueOnError)
	flags.SetOutput(out)
	socket := flags.String("socket", "", "the sink's socket (required)")
	brightness := flags.Float64("brightness", 3, "the gradient's peak, in percent")
	fps := flags.Float64("fps", defaultFPS, "frames per second")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *socket == "" {
		return errors.New("test-pattern: --socket is required")
	}
	period, err := framePeriod(*fps)
	if err != nil {
		return fmt.Errorf("test-pattern: %w", err)
	}

	w, err := sinkproto.Dial(*socket)
	if err != nil {
		return fmt.Errorf("test-pattern: %w", err)
	}
	defer w.Close()
	width, height := w.Width(), w.Height()
	fmt.Fprintf(out, "sending a %dx%d test pattern to %s; interrupt to stop\n", width, height, *socket)

	frame := 0
	err = sendFrames(ctx, w, period, func(pix []uint16) {
		drawTestPattern(pix, width, height, frame, *brightness/100)
		frame++
	})
	if err != nil {
		return fmt.Errorf("test-pattern: %w", err)
	}
	return nil
}

// drawTestPattern fills pix for the given frame number. level is the
// gradient's peak, from 0 to 1.
func drawTestPattern(pix []uint16, width, height, frame int, level float64) {
	const barWidth = 4
	bar := frame % width
	for y := range height {
		for x := range width {
			p := pix[3*(y*width+x):][:3]
			if y < height/2 {
				// A quarter-brightness white bar, so it doesn't dazzle.
				v := uint16(0)
				if d := x - bar; d >= 0 && d < barWidth {
					v = 65535 / 4
				}
				p[0], p[1], p[2] = v, v, v
				continue
			}
			t := 0.0
			if width > 1 {
				t = float64(x) / float64(width-1)
			}
			for c := range 3 {
				p[c] = uint16(gradientColour[c] / 255 * 65535 * level * t)
			}
		}
	}
}
