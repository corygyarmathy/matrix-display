package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/corygyarmathy/matrix-display/internal/render"
	"github.com/corygyarmathy/matrix-display/internal/sinkproto"
	"github.com/corygyarmathy/matrix-display/internal/source"
	"github.com/corygyarmathy/matrix-display/internal/sun"
)

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
	r := render.Renderer{}
	if r.Art, err = art(); err != nil {
		return fmt.Errorf("serve: %w", err)
	}
	if r.Location, err = sun.LoadLocation(*locationFile); err != nil {
		return fmt.Errorf("serve: reading the location: %w", err)
	}

	// The sources (#8, #9, #10) start here, each with source.Run.
	store := new(source.Store)

	stream(ctx, *socket, period, r, store.Snapshot)
	return nil
}

// stream renders a frame every period and sends it to the sink at socket,
// until ctx ends, with r's location and art at the size the sink reports.
// When the sink isn't there, or goes away, it dials again.
func stream(ctx context.Context, socket string, period time.Duration, r render.Renderer, state func() render.State) {
	wait := minRedial
	reported := false
	for {
		w, err := sinkproto.Dial(socket)
		if err == nil {
			slog.Info("connected to the sink", "width", w.Width(), "height", w.Height())
			r.Width, r.Height = w.Width(), w.Height()
			live := r.Live()
			err = sendFrames(ctx, w, period, func(pix []uint16) {
				live.RenderInto(pix, time.Now(), state())
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
