package render

import (
	"slices"
	"testing"
	"time"

	"github.com/corygyarmathy/matrix-display/internal/sun"
)

const width, height = 256, 64

var greenwich = Renderer{Width: width, Height: height, Location: sun.Greenwich}

// Times at Greenwich, on the 2025 June solstice.
var (
	midnight = time.Date(2025, 6, 21, 0, 0, 0, 0, time.UTC)
	dusk     = time.Date(2025, 6, 21, 20, 30, 0, 0, time.UTC) // About -2°.
	noon     = time.Date(2025, 6, 21, 12, 0, 0, 0, time.UTC)
)

func TestRenderIsPure(t *testing.T) {
	a, b := greenwich.Render(dusk, State{}), greenwich.Render(dusk, State{})
	if a.Width != width || a.Height != height || len(a.Pix) != width*height*3 {
		t.Fatalf("frame is %dx%d with %d values, want %dx%d with %d",
			a.Width, a.Height, len(a.Pix), width, height, width*height*3)
	}
	if !slices.Equal(a.Pix, b.Pix) {
		t.Error("two renders of the same time and state differ")
	}
}

// The placeholder brightens as the sun rises, and is never black.
func TestRenderFollowsTheSun(t *testing.T) {
	var last float64
	for _, at := range []time.Time{midnight, dusk, noon} {
		f := greenwich.Render(at, State{})
		mean := meanLevel(f)
		if mean <= last {
			t.Errorf("at %s, mean level %.1f, want more than %.1f before it", at.Format(time.Kitchen), mean, last)
		}
		last = mean
		for x := range width {
			if p := f.At(x, height-1); p == [3]uint16{} {
				t.Fatalf("at %s, pixel (%d, %d) is black", at.Format(time.Kitchen), x, height-1)
			}
		}
	}
}

// Layers keep the full precision: a gradient in the dark must not be
// quantised to the library's 8-bit steps (multiples of 257).
func TestRenderKeepsPrecision(t *testing.T) {
	f := greenwich.Render(midnight, State{})
	for _, v := range f.Pix {
		if v%257 != 0 {
			return
		}
	}
	t.Error("every value is a multiple of 257, as if rounded to 8 bits")
}

func TestStaleMarker(t *testing.T) {
	stale := Health{LastOK: noon.Add(-10 * time.Minute), StaleAfter: 5 * time.Minute}
	fresh := Health{LastOK: noon.Add(-time.Minute), StaleAfter: 5 * time.Minute}
	marker := greenwich.Render(noon, State{Sources: map[string]Health{"a": stale}}).At(width-1, height-1)
	want := [3]uint16{48 * 257, 16 * 257, 0}
	if marker != want {
		t.Errorf("a stale source's marker is %v, want %v", marker, want)
	}

	for name, s := range map[string]State{
		"no sources": {},
		"fresh":      {Sources: map[string]Health{"a": fresh}},
		"one stale":  {Sources: map[string]Health{"a": fresh, "b": stale}},
	} {
		f := greenwich.Render(noon, s)
		got := f.At(width-1, height-1)
		if (got == want) != (name == "one stale") {
			t.Errorf("%s: the corner is %v", name, got)
		}
		if f.At(width-2, height-1) == want || f.At(width-1, height-2) == want {
			t.Errorf("%s: the marker spread beyond the corner", name)
		}
	}
}

func TestStateClone(t *testing.T) {
	s := State{Sources: map[string]Health{"a": {}}}
	c := s.Clone()
	c.Sources["b"] = Health{}
	if len(s.Sources) != 1 {
		t.Error("changing a clone's Sources changed the original's")
	}
}

func meanLevel(f Frame) float64 {
	var sum float64
	for _, v := range f.Pix {
		sum += float64(v)
	}
	return sum / float64(len(f.Pix))
}
