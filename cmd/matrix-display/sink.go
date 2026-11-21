package main

import (
	"context"
	"errors"
	"time"

	"github.com/corygyarmathy/matrix-display/internal/sinkproto"
)

// defaultFPS is the frame rate to the sink unless --fps says otherwise. The
// spike (#2) found motion very smooth at about 98 fps.
const defaultFPS = 98

// framePeriod is the time between frames at fps frames per second. It fails
// unless the period is positive: past 1e9 fps it rounds to nothing.
func framePeriod(fps float64) (time.Duration, error) {
	period := time.Duration(float64(time.Second) / fps)
	if !(fps > 0) || period <= 0 {
		return 0, errors.New("--fps must be positive, and at most 1e9")
	}
	return period, nil
}

// sendFrames sends a frame over w every period until ctx ends, returning nil,
// or until a write fails. draw fills each frame, which is w.Width() ×
// w.Height() pixels in the protocol's layout, before it is sent.
func sendFrames(ctx context.Context, w *sinkproto.Writer, period time.Duration, draw func(pix []uint16)) error {
	pix := make([]uint16, w.Width()*w.Height()*3)
	ticker := time.NewTicker(period)
	defer ticker.Stop()
	for {
		draw(pix)
		if err := w.WriteFrame(pix); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
