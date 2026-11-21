package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/png"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/corygyarmathy/matrix-display/internal/render"
	"github.com/corygyarmathy/matrix-display/internal/sun"
)

// The preview's panel size: rpi01's, which the sink reports in serve.
const previewWidth, previewHeight = 256, 64

// maxPreviewFrames bounds a video, so that a mistyped --speed fails rather
// than rendering for hours: a million frames is over nine hours at 30 fps.
const maxPreviewFrames = 1_000_000

// previewOptions is what preview renders.
type previewOptions struct {
	from, to   time.Time
	speed, fps float64
	// motion is how many times faster than real time the art moves in a
	// video, which is slower than --speed: at a day a minute, the art
	// moving with simulated time would reshape twice a frame, and flicker.
	motion     float64
	scale, gap int
	location   sun.Location
	art        render.Art
	state      render.State
}

// preview renders simulated time to a video or an image, with fixture state
// and no sink, upscaled with gaps between the pixels so it reads like the
// panels. A monitor can't show how the panels render dark gradients, which
// are near black and dithered by the sink: check night palettes on them.
func preview(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("preview", flag.ContinueOnError)
	flags.SetOutput(out)
	from := flags.String("from", "", "the first frame's time, as 2006-01-02T15:04 local time or RFC 3339 (default: today's midnight)")
	to := flags.String("to", "", "the last frame's time, in the same form (default: 24 hours after --from)")
	speed := flags.Float64("speed", 1440, "simulated seconds per second of video")
	fps := flags.Float64("fps", 30, "the video's frames per second")
	scale := flags.Int("scale", 4, "output pixels per panel pixel")
	gap := flags.Int("gap", 1, "dark output pixels between panel pixels")
	locationFile := flags.String("location", "", "a file holding the latitude and longitude (default: Greenwich)")
	motion := flags.Float64("motion", 0, "how many times faster than real time the art moves in a video (default: --speed / 60, at least 1)")
	art := artFlags(flags)
	stale := flags.Bool("stale", false, "show a data source as stale throughout, to preview the marker")
	outPath := flags.String("out", "", "the file to write: .mp4 or .gif (with ffmpeg), or .png for one frame at --from (required)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *outPath == "" {
		return errors.New("preview: --out is required")
	}
	if !(*speed > 0) || !(*fps > 0) {
		return errors.New("preview: --speed and --fps must be positive")
	}
	if *motion == 0 {
		*motion = max(1, *speed/60)
	}
	if !(*motion > 0) || math.IsInf(*motion, 0) {
		return errors.New("preview: --motion must be positive")
	}
	if *scale < 1 || *gap < 0 || *gap >= *scale {
		return errors.New("preview: --scale must be at least 1, and --gap less than it")
	}

	opts := previewOptions{speed: *speed, fps: *fps, motion: *motion, scale: *scale, gap: *gap, location: sun.Greenwich}
	var err error
	if opts.art, err = art(); err != nil {
		return fmt.Errorf("preview: %w", err)
	}
	if _, err := opts.videoRenderer(); err != nil {
		return fmt.Errorf("preview: %w", err)
	}
	now := time.Now()
	if *from == "" {
		opts.from = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	} else if opts.from, err = parseTime(*from); err != nil {
		return fmt.Errorf("preview: --from: %w", err)
	}
	if *to == "" {
		opts.to = opts.from.Add(24 * time.Hour)
	} else if opts.to, err = parseTime(*to); err != nil {
		return fmt.Errorf("preview: --to: %w", err)
	}
	if !opts.to.After(opts.from) {
		return errors.New("preview: --to must be after --from")
	}
	if step := opts.step(); step <= 0 || opts.to.Sub(opts.from)/step >= maxPreviewFrames {
		return fmt.Errorf("preview: that would be over %d frames; raise --speed or lower --fps", maxPreviewFrames)
	}
	if *locationFile != "" {
		if opts.location, err = sun.LoadLocation(*locationFile); err != nil {
			return fmt.Errorf("preview: reading the location: %w", err)
		}
	}

	if *stale {
		// A source that has never fetched is stale at any time.
		opts.state.Sources = map[string]render.Health{"fixture": {StaleAfter: 5 * time.Minute}}
	}

	switch ext := strings.ToLower(filepath.Ext(*outPath)); ext {
	case ".png":
		if err := writePreviewPNG(*outPath, opts); err != nil {
			return fmt.Errorf("preview: %w", err)
		}
		return nil
	case ".mp4", ".gif":
		start := time.Now()
		n, err := writePreviewVideo(ctx, *outPath, opts)
		if err != nil {
			return fmt.Errorf("preview: %w", err)
		}
		fmt.Fprintf(out, "wrote %d frames to %s in %s\n", n, *outPath, time.Since(start).Round(100*time.Millisecond))
		return nil
	default:
		return fmt.Errorf("preview: can't write %q files; use .mp4, .gif or .png", ext)
	}
}

// parseTime parses RFC 3339, or a date and time with no zone, which is
// local time.
func parseTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	for _, layout := range []string{"2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("can't read %q: use 2006-01-02T15:04 or RFC 3339", s)
}

// step is the simulated time between video frames. It can round to nothing
// when --speed is tiny against --fps, so preview checks it.
func (opts previewOptions) step() time.Duration {
	return time.Duration(opts.speed / opts.fps * float64(time.Second))
}

// frameTimes returns the simulated time of every video frame, from opts.from
// to opts.to inclusive.
func (opts previewOptions) frameTimes() []time.Time {
	step := opts.step()
	var times []time.Time
	for t := opts.from; !t.After(opts.to); t = t.Add(step) {
		times = append(times, t)
	}
	return times
}

// renderer draws a still: the art as it is at that moment.
func (opts previewOptions) renderer() render.Renderer {
	return render.Renderer{Width: previewWidth, Height: previewHeight, Location: opts.location, Art: opts.art}
}

// videoRenderer draws a video's frames, with the art moving opts.motion
// times faster than real time in the video, not opts.speed times. It fails
// when --motion is so far from --speed that the art can't be drawn.
func (opts previewOptions) videoRenderer() (render.Renderer, error) {
	r := opts.renderer()
	var err error
	if r.Art, err = slowArt(r.Art, opts.speed/opts.motion); err != nil {
		return render.Renderer{}, fmt.Errorf("--motion %g at --speed %g: %w", opts.motion, opts.speed, err)
	}
	return r, nil
}

func writePreviewPNG(path string, opts previewOptions) error {
	img := upscale(nil, opts.renderer().Render(opts.from, opts.state), opts.scale, opts.gap)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// writePreviewVideo pipes 16-bit frames to ffmpeg, which picks the format
// from the file's extension. It returns the number of frames.
func writePreviewVideo(ctx context.Context, path string, opts previewOptions) (int, error) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return 0, errors.New("writing video needs ffmpeg on the PATH (`nix run .` provides it)")
	}
	w, h := previewWidth*opts.scale, previewHeight*opts.scale
	args := []string{
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "rawvideo", "-pix_fmt", "rgba64be", "-s", fmt.Sprintf("%dx%d", w, h),
		"-framerate", strconv.FormatFloat(opts.fps, 'f', -1, 64), "-i", "-",
	}
	if strings.EqualFold(filepath.Ext(path), ".gif") {
		args = append(args, "-filter_complex", "split[a][b];[a]palettegen=stats_mode=diff[p];[b][p]paletteuse")
	} else {
		args = append(args, "-c:v", "libx264", "-preset", "fast", "-crf", "18", "-pix_fmt", "yuv420p", "-movflags", "+faststart")
	}
	args = append(args, path)

	r, err := opts.videoRenderer()
	if err != nil {
		return 0, err
	}
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return 0, err
	}
	if err := cmd.Start(); err != nil {
		return 0, err
	}

	var img *image.RGBA64
	times := opts.frameTimes()
	var writeErr error
	for _, t := range times {
		img = upscale(img, r.Render(t, opts.state), opts.scale, opts.gap)
		if _, writeErr = stdin.Write(img.Pix); writeErr != nil {
			break
		}
	}
	stdin.Close()
	if err := cmd.Wait(); err != nil {
		return 0, fmt.Errorf("ffmpeg: %w", err)
	}
	if writeErr != nil {
		return 0, writeErr
	}
	return len(times), nil
}

// upscale draws f into dst (allocating it if nil), each panel pixel a
// scale × scale square with gap dark pixels on its right and bottom, at the
// frame's full 16 bits.
func upscale(dst *image.RGBA64, f render.Frame, scale, gap int) *image.RGBA64 {
	if dst == nil {
		dst = image.NewRGBA64(image.Rect(0, 0, f.Width*scale, f.Height*scale))
	}
	src := f.RGBA64()
	dark := []byte{0, 0, 0, 0, 0, 0, 0xff, 0xff}
	for y := range f.Height {
		for x := range f.Width {
			p := src.Pix[src.PixOffset(x, y):][:8]
			for dy := range scale {
				row := dst.Pix[(y*scale+dy)*dst.Stride:]
				for dx := range scale {
					v := p
					if dx >= scale-gap || dy >= scale-gap {
						v = dark
					}
					copy(row[(x*scale+dx)*8:][:8], v)
				}
			}
		}
	}
	return dst
}
