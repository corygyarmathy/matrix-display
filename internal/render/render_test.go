package render

import (
	"reflect"
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

// The art brightens as the sun rises, and is never black.
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

// Live draws what Render draws, frame after frame, and allocates nothing per
// frame: at 98 fps on the Pi, garbage means GC pauses, which show as jitter.
func TestLiveMatchesRenderWithoutAllocating(t *testing.T) {
	live := greenwich.Live()
	pix := make([]uint16, width*height*3)
	stale := State{Sources: map[string]Health{"a": {LastOK: noon.Add(-10 * time.Minute), StaleAfter: 5 * time.Minute}}}
	for _, c := range []struct {
		at time.Time
		s  State
	}{{noon, stale}, {midnight, State{}}, {dusk, stale}} {
		live.RenderInto(pix, c.at, c.s)
		if !slices.Equal(pix, greenwich.Render(c.at, c.s).Pix) {
			t.Errorf("at %s, Live drew a different frame from Render", c.at.Format(time.Kitchen))
		}
	}
	if n := testing.AllocsPerRun(10, func() { live.RenderInto(pix, noon, stale) }); n != 0 {
		t.Errorf("RenderInto allocated %v times per frame, want 0", n)
	}
}

// Clone must copy every field of State that holds memory, so that a snapshot
// shares nothing with the sources. This fails when State gains such a field
// that Clone doesn't copy, or one nested deeper than this test can check.
func TestStateCloneSharesNothing(t *testing.T) {
	var s State
	v := reflect.ValueOf(&s).Elem()
	var copied []int
	for i := range v.NumField() {
		f := v.Type().Field(i)
		switch f.Type.Kind() {
		case reflect.Map:
			if sharesMemory(f.Type.Key()) || sharesMemory(f.Type.Elem()) {
				t.Errorf("State.%s holds memory inside its values: copy them in Clone, and teach this test to check it", f.Name)
				continue
			}
			m := reflect.MakeMap(f.Type)
			m.SetMapIndex(reflect.Zero(f.Type.Key()), reflect.Zero(f.Type.Elem()))
			v.Field(i).Set(m)
		case reflect.Slice:
			if sharesMemory(f.Type.Elem()) {
				t.Errorf("State.%s holds memory inside its elements: copy them in Clone, and teach this test to check it", f.Name)
				continue
			}
			v.Field(i).Set(reflect.MakeSlice(f.Type, 1, 1))
		default:
			if sharesMemory(f.Type) {
				t.Errorf("State.%s holds memory: copy it in Clone, and teach this test to check it", f.Name)
			}
			continue
		}
		copied = append(copied, i)
	}
	c := reflect.ValueOf(s.Clone())
	for _, i := range copied {
		if c.Field(i).UnsafePointer() == v.Field(i).UnsafePointer() {
			t.Errorf("Clone shares State.%s with the original", v.Type().Field(i).Name)
		}
	}
}

// sharesMemory reports whether a copy of a value of type t can share memory
// with the original. time.Time counts as a plain value: it points only to its
// zone, which nothing changes.
func sharesMemory(t reflect.Type) bool {
	if t == reflect.TypeFor[time.Time]() {
		return false
	}
	switch t.Kind() {
	case reflect.Map, reflect.Slice, reflect.Pointer, reflect.Interface, reflect.Chan, reflect.Func, reflect.UnsafePointer:
		return true
	case reflect.Array:
		return sharesMemory(t.Elem())
	case reflect.Struct:
		for i := range t.NumField() {
			if sharesMemory(t.Field(i).Type) {
				return true
			}
		}
	}
	return false
}

func meanLevel(f Frame) float64 {
	var sum float64
	for _, v := range f.Pix {
		sum += float64(v)
	}
	return sum / float64(len(f.Pix))
}
