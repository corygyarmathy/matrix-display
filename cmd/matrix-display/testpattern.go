package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/corygyarmathy/matrix-display/internal/sinkproto"
)

// The spike's (#2) gradient colour, in 8-bit values.
var gradientColour = [3]float64{60, 30, 160}

// testPattern sends a pattern to the sink until ctx ends, for checking the
// panels by eye (#5):
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
	width := flags.Int("width", 256, "panel width in pixels")
	height := flags.Int("height", 64, "panel height in pixels")
	brightness := flags.Float64("brightness", 3, "the gradient's peak, in percent")
	fps := flags.Float64("fps", 98, "frames per second")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *socket == "" {
		return errors.New("test-pattern: --socket is required")
	}
	if *width <= 0 || *height <= 0 || *fps <= 0 {
		return errors.New("test-pattern: --width, --height and --fps must be positive")
	}

	w, err := sinkproto.Dial(*socket)
	if err != nil {
		return fmt.Errorf("test-pattern: %w", err)
	}
	defer w.Close()
	fmt.Fprintf(out, "sending a %dx%d test pattern to %s; interrupt to stop\n", *width, *height, *socket)

	ticker := time.NewTicker(time.Duration(float64(time.Second) / *fps))
	defer ticker.Stop()
	pix := make([]uint16, *width**height*3)
	for frame := 0; ; frame++ {
		drawTestPattern(pix, *width, *height, frame, *brightness/100)
		if err := w.WriteFrame(pix); err != nil {
			return fmt.Errorf("test-pattern: %w", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
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
