package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/png"
	"io"
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

// previewOptions is what preview renders.
type previewOptions struct {
	from, to   time.Time
	speed, fps float64
	scale, gap int
	location   sun.Location
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
	outPath := flags.String("out", "", "the file to write: .mp4 or .gif (with ffmpeg), or .png for one frame at --from (required)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *outPath == "" {
		return errors.New("preview: --out is required")
	}
	if *speed <= 0 || *fps <= 0 {
		return errors.New("preview: --speed and --fps must be positive")
	}
	if *scale < 1 || *gap < 0 || *gap >= *scale {
		return errors.New("preview: --scale must be at least 1, and --gap less than it")
	}

	opts := previewOptions{speed: *speed, fps: *fps, scale: *scale, gap: *gap, location: sun.Greenwich}
	var err error
	now := time.Now()
	if *from == "" {
		opts.from = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	} else if opts.from, err = parsePreviewTime(*from); err != nil {
		return fmt.Errorf("preview: --from: %w", err)
	}
	if *to == "" {
		opts.to = opts.from.Add(24 * time.Hour)
	} else if opts.to, err = parsePreviewTime(*to); err != nil {
		return fmt.Errorf("preview: --to: %w", err)
	}
	if !opts.to.After(opts.from) {
		return errors.New("preview: --to must be after --from")
	}
	if *locationFile != "" {
		if opts.location, err = sun.LoadLocation(*locationFile); err != nil {
			return fmt.Errorf("preview: reading the location: %w", err)
		}
	}

	switch ext := strings.ToLower(filepath.Ext(*outPath)); ext {
	case ".png":
		return writePreviewPNG(*outPath, opts)
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

// parsePreviewTime parses RFC 3339, or a date and time with no zone, which is
// local time.
func parsePreviewTime(s string) (time.Time, error) {
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

// frameTimes returns the simulated time of every video frame, from opts.from
// to opts.to inclusive.
func (opts previewOptions) frameTimes() []time.Time {
	step := time.Duration(opts.speed / opts.fps * float64(time.Second))
	var times []time.Time
	for t := opts.from; !t.After(opts.to); t = t.Add(step) {
		times = append(times, t)
	}
	return times
}

func (opts previewOptions) renderer() render.Renderer {
	return render.Renderer{Width: previewWidth, Height: previewHeight, Location: opts.location}
}

func writePreviewPNG(path string, opts previewOptions) error {
	img := upscale(nil, opts.renderer().Render(opts.from, opts.state), opts.scale, opts.gap)
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("preview: %w", err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return fmt.Errorf("preview: %w", err)
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

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return 0, err
	}
	if err := cmd.Start(); err != nil {
		return 0, err
	}

	r := opts.renderer()
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
	for y := range f.Height {
		for x := range f.Width {
			p := f.At(x, y)
			for dy := range scale {
				row := dst.Pix[(y*scale+dy)*dst.Stride:]
				for dx := range scale {
					v := p
					if dx >= scale-gap || dy >= scale-gap {
						v = [3]uint16{}
					}
					putRGBA64(row[(x*scale+dx)*8:][:8], v)
				}
			}
		}
	}
	return dst
}

// putRGBA64 writes an opaque pixel in image.RGBA64's big-endian layout.
func putRGBA64(o []byte, p [3]uint16) {
	o[0], o[1] = byte(p[0]>>8), byte(p[0])
	o[2], o[3] = byte(p[1]>>8), byte(p[1])
	o[4], o[5] = byte(p[2]>>8), byte(p[2])
	o[6], o[7] = 0xff, 0xff
}
