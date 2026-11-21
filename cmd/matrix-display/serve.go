package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math"
	"time"

	"github.com/corygyarmathy/matrix-display/internal/render"
	"github.com/corygyarmathy/matrix-display/internal/sinkproto"
	"github.com/corygyarmathy/matrix-display/internal/source"
	"github.com/corygyarmathy/matrix-display/internal/sun"
)

// maxSpeed is the fastest --speed: a day a second. Faster than that, the
// palette changes faster than the frames can show it.
const maxSpeed = 86400

// The wait between attempts to reach the sink doubles from minRedial up to
// maxRedial.
const (
	minRedial = 100 * time.Millisecond
	maxRedial = 5 * time.Second
)

// serve is the display's normal mode: it renders frames and sends them to the
// sink until ctx ends.
func serve(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(out)
	socket := flags.String("socket", "", "the sink's socket (required)")
	locationFile := flags.String("location", "", "a file holding the latitude and longitude (required)")
	fps := flags.Float64("fps", defaultFPS, "frames per second")
	from := flags.String("from", "", "show this time instead of now, as 2006-01-02T15:04 local time or RFC 3339, to test the panels")
	speed := flags.Float64("speed", 1, fmt.Sprintf("simulated seconds per real second, up to %d, to test the panels (the art still moves in real time)", maxSpeed))
	art := artFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *socket == "" || *locationFile == "" {
		return errors.New("serve: --socket and --location are required")
	}
	period, err := framePeriod(*fps)
	if err != nil {
		return fmt.Errorf("serve: %w", err)
	}
	if !(*speed > 0) || *speed > maxSpeed {
		return fmt.Errorf("serve: --speed must be positive and at most %d", maxSpeed)
	}
	start := time.Now()
	if *from != "" {
		if start, err = parseTime(*from); err != nil {
			return fmt.Errorf("serve: --from: %w", err)
		}
	}
	a, err := art()
	if err != nil {
		return fmt.Errorf("serve: %w", err)
	}
	clock := time.Now
	if *from != "" || *speed != 1 {
		// The art moves with the clock, so slow it by --speed to keep its
		// real pace.
		if a, err = slowArt(a, *speed); err != nil {
			return fmt.Errorf("serve: --speed %g: %w", *speed, err)
		}
		clock = simClock(start, *speed, time.Now)
		slog.Info("showing simulated time", "from", start.Format(time.RFC3339), "speed", *speed)
	}
	loc, err := sun.LoadLocation(*locationFile)
	if err != nil {
		return fmt.Errorf("serve: reading the location: %w", err)
	}

	// The sources (#8, #9, #10) start here, each with source.Run.
	store := new(source.Store)

	stream(ctx, *socket, period, loc, a, clock, store.Snapshot)
	return nil
}

// simClock returns a clock that reads from when first called, and runs
// speed times faster than now from then on.
func simClock(from time.Time, speed float64, now func() time.Time) func() time.Time {
	var origin time.Time
	return func() time.Time {
		t := now()
		if origin.IsZero() {
			origin = t
		}
		// At most 2^63 ns, about 292 years, past from: at maxSpeed, over
		// three years of running.
		d := float64(t.Sub(origin)) * speed
		if d >= math.MaxInt64 {
			return from.Add(math.MaxInt64)
		}
		return from.Add(time.Duration(d))
	}
}

// stream renders a frame every period and sends it to the sink at socket,
// until ctx ends, for loc with art a at the time clock gives, at the size the
// sink reports. When the
// sink isn't there, or goes away, it dials again.
func stream(ctx context.Context, socket string, period time.Duration, loc sun.Location, a render.Art, clock func() time.Time, state func() render.State) {
	wait := minRedial
	reported := false
	for {
		w, err := sinkproto.Dial(socket)
		if err == nil {
			slog.Info("connected to the sink", "width", w.Width(), "height", w.Height())
			live := render.Renderer{Width: w.Width(), Height: w.Height(), Location: loc, Art: a}.Live()
			err = sendFrames(ctx, w, period, func(pix []uint16) {
				live.RenderInto(pix, clock(), state())
			})
			w.Close()
			if ctx.Err() != nil {
				return
			}
			slog.Warn("lost the sink; reconnecting", "err", err)
			wait, reported = minRedial, true
		} else {
			// Once per outage, not on every attempt.
			if !reported {
				slog.Warn("can't reach the sink; retrying", "err", err)
				reported = true
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = min(2*wait, maxRedial)
	}
}
