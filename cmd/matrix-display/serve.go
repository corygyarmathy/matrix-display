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

// maxRedial caps the wait between attempts to reach the sink.
const maxRedial = 5 * time.Second

// serve is the display's normal mode: it renders frames and sends them to the
// sink until ctx ends.
func serve(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(out)
	socket := flags.String("socket", "", "the sink's socket (required)")
	locationFile := flags.String("location", "", "a file holding the latitude and longitude (required)")
	fps := flags.Float64("fps", 98, "frames per second")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *socket == "" || *locationFile == "" {
		return errors.New("serve: --socket and --location are required")
	}
	if *fps <= 0 {
		return errors.New("serve: --fps must be positive")
	}
	loc, err := sun.LoadLocation(*locationFile)
	if err != nil {
		return fmt.Errorf("serve: reading the location: %w", err)
	}

	// The sources (#8, #9, #10) start here, each with source.Run.
	store := new(source.Store)

	stream(ctx, *socket, time.Duration(float64(time.Second) / *fps), loc, store.Snapshot)
	return nil
}

// stream renders a frame every period and sends it to the sink at socket,
// until ctx ends. When the sink isn't there, or goes away, it dials again.
func stream(ctx context.Context, socket string, period time.Duration, loc sun.Location, state func() render.State) {
	wait := 100 * time.Millisecond
	reported := false
	for {
		w, err := sinkproto.Dial(socket)
		if err == nil {
			slog.Info("connected to the sink", "width", w.Width(), "height", w.Height())
			err = sendFrames(ctx, w, period, loc, state)
			w.Close()
			if ctx.Err() != nil {
				return
			}
			slog.Warn("lost the sink; reconnecting", "err", err)
			wait, reported = 100*time.Millisecond, true
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

// sendFrames sends frames over w until ctx ends or a write fails.
func sendFrames(ctx context.Context, w *sinkproto.Writer, period time.Duration, loc sun.Location, state func() render.State) error {
	r := render.Renderer{Width: w.Width(), Height: w.Height(), Location: loc}
	ticker := time.NewTicker(period)
	defer ticker.Stop()
	for {
		if err := w.WriteFrame(r.Render(time.Now(), state()).Pix); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
