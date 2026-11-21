package main

import (
	"context"
	"image/color"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/corygyarmathy/matrix-display/internal/render"
)

func TestUpscale(t *testing.T) {
	f := render.Frame{Width: 2, Height: 1, Pix: []uint16{1000, 2000, 3000, 4000, 5000, 6000}}
	img := upscale(nil, f, 3, 1)
	if b := img.Bounds(); b.Dx() != 6 || b.Dy() != 3 {
		t.Fatalf("image is %dx%d, want 6x3", b.Dx(), b.Dy())
	}
	first := color.RGBA64{1000, 2000, 3000, 0xffff}
	second := color.RGBA64{4000, 5000, 6000, 0xffff}
	gap := color.RGBA64{0, 0, 0, 0xffff}
	for _, c := range []struct {
		x, y int
		want color.RGBA64
	}{{0, 0, first}, {1, 1, first}, {2, 0, gap}, {0, 2, gap}, {3, 0, second}, {4, 1, second}, {5, 1, gap}} {
		if got := img.RGBA64At(c.x, c.y); got != c.want {
			t.Errorf("(%d, %d) = %v, want %v", c.x, c.y, got, c.want)
		}
	}
}

func TestFrameTimes(t *testing.T) {
	from := time.Date(2025, 6, 21, 0, 0, 0, 0, time.UTC)
	opts := previewOptions{from: from, to: from.Add(24 * time.Hour), speed: 1440, fps: 30}
	times := opts.frameTimes()
	// A minute of video at 30 fps, and the last frame.
	if len(times) != 1801 || !times[0].Equal(opts.from) || !times[1800].Equal(opts.to) {
		t.Errorf("%d frames from %s to %s, want 1801 from %s to %s",
			len(times), times[0], times[len(times)-1], opts.from, opts.to)
	}
}

func TestPreviewPNG(t *testing.T) {
	path := filepath.Join(t.TempDir(), "noon.png")
	var out strings.Builder
	err := run(context.Background(), []string{"preview", "--from", "2025-06-21T12:00:00Z", "--scale", "2", "--out", path}, &out)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 512 || b.Dy() != 128 {
		t.Errorf("image is %dx%d, want 512x128", b.Dx(), b.Dy())
	}
}

// --stale lights the stale marker in the bottom-right panel pixel.
func TestPreviewStale(t *testing.T) {
	corner := func(args ...string) color.RGBA64 {
		t.Helper()
		path := filepath.Join(t.TempDir(), "noon.png")
		args = append([]string{"preview", "--from", "2025-06-21T12:00:00Z", "--scale", "1", "--gap", "0", "--out", path}, args...)
		if err := run(context.Background(), args, new(strings.Builder)); err != nil {
			t.Fatal(err)
		}
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		img, err := png.Decode(f)
		if err != nil {
			t.Fatal(err)
		}
		return color.RGBA64Model.Convert(img.At(previewWidth-1, previewHeight-1)).(color.RGBA64)
	}
	marker := color.RGBA64{48 * 257, 16 * 257, 0, 0xffff}
	if got := corner("--stale"); got != marker {
		t.Errorf("with --stale, the corner is %v, want the marker %v", got, marker)
	}
	if got := corner(); got == marker {
		t.Error("without --stale, the corner shows the marker")
	}
}

// The acceptance criterion from #6: a 24-hour MP4 in under a minute on a
// laptop. It takes about 6s there; the margin absorbs slower CI machines.
func TestPreviewDayUnderAMinute(t *testing.T) {
	if testing.Short() || raceDetector {
		t.Skip("renders a whole day, which takes too long under -short or -race")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("no ffmpeg")
	}
	path := filepath.Join(t.TempDir(), "day.mp4")
	start := time.Now()
	if err := run(context.Background(), []string{"preview", "--from", "2025-06-21", "--out", path}, new(strings.Builder)); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took >= time.Minute {
		t.Errorf("a 24-hour MP4 took %s, want under a minute", took.Round(time.Second))
	}
}

func TestPreviewVideo(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("no ffmpeg")
	}
	for _, ext := range []string{".mp4", ".gif"} {
		path := filepath.Join(t.TempDir(), "preview"+ext)
		var out strings.Builder
		args := []string{"preview", "--from", "2025-06-21T12:00:00Z", "--to", "2025-06-21T12:01:00Z", "--speed", "60", "--fps", "5", "--out", path}
		if err := run(context.Background(), args, &out); err != nil {
			t.Fatalf("%s: %v", ext, err)
		}
		if !strings.Contains(out.String(), "wrote 6 frames") {
			t.Errorf("%s: preview said %q, want 6 frames", ext, out.String())
		}
		if st, err := os.Stat(path); err != nil || st.Size() == 0 {
			t.Errorf("%s: no video written: %v", ext, err)
		}
	}
}

func TestPreviewRejects(t *testing.T) {
	for _, args := range [][]string{
		{"preview"},
		{"preview", "--out", "x.webm"},
		{"preview", "--out", "x.png", "--from", "yesterday"},
		{"preview", "--out", "x.mp4", "--from", "2025-06-21", "--to", "2025-06-20"},
		{"preview", "--out", "x.png", "--gap", "4"},
		{"preview", "--out", "x.mp4", "--speed", "NaN"},
		{"preview", "--out", "x.mp4", "--motion", "-1"},
		// So fast the reshape time rounds to nothing.
		{"preview", "--out", "x.mp4", "--speed", "1", "--motion", "1e11"},
		{"preview", "--out", "x.png", "--east", "north"},
		{"preview", "--out", "x.png", "--ceiling", "0"},
		{"preview", "--out", "x.png", "--ceiling", "1.5"},
		{"preview", "--out", "x.png", "--reshape", "0s"},
		{"preview", "--out", "x.png", "--drift", "Inf"},
		// Steps that round to nothing, or too many frames, which would
		// render forever.
		{"preview", "--out", "x.mp4", "--speed", "1e-9"},
		{"preview", "--out", "x.mp4", "--speed", "0.001"},
	} {
		var out strings.Builder
		if err := run(context.Background(), args, &out); err == nil {
			t.Errorf("run(%q) succeeded, want an error", args)
		}
	}
}

// A video's art moves --motion times faster than real time, not --speed
// times: at a day a minute, once per second of video rather than twice a
// frame.
func TestPreviewMotion(t *testing.T) {
	opts := previewOptions{speed: 1440, motion: 24, art: render.DefaultArt}
	r, err := opts.videoRenderer()
	if err != nil {
		t.Fatal(err)
	}
	art := r.Art
	if art.Reshape != 60*render.DefaultArt.Reshape || art.Drift != render.DefaultArt.Drift/60 {
		t.Errorf("at --speed 1440 and --motion 24, the art reshapes every %s and drifts %g px/s, want %s and %g",
			art.Reshape, art.Drift, 60*render.DefaultArt.Reshape, render.DefaultArt.Drift/60)
	}
	if opts.renderer().Art != render.DefaultArt {
		t.Error("a still's art is not the real art")
	}
}

// A reshape time too long for a Duration is the longest one, not one that
// wraps negative.
func TestPreviewMotionClamps(t *testing.T) {
	r, err := previewOptions{speed: 1e12, motion: 1, art: render.DefaultArt}.videoRenderer()
	if err != nil || r.Art.Reshape != math.MaxInt64 {
		t.Errorf("at --speed 1e12 and --motion 1, the art reshapes every %s (%v), want %s",
			r.Art.Reshape, err, time.Duration(math.MaxInt64))
	}
}

func TestParsePreviewTime(t *testing.T) {
	want := time.Date(2025, 6, 21, 21, 30, 0, 0, time.Local)
	for _, s := range []string{"2025-06-21T21:30", "2025-06-21T21:30:00", want.Format(time.RFC3339)} {
		if got, err := parsePreviewTime(s); err != nil || !got.Equal(want) {
			t.Errorf("parsePreviewTime(%q) = %s, %v; want %s", s, got, err, want)
		}
	}
}
