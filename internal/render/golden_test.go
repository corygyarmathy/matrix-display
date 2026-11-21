package render

import (
	"flag"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// update rewrites the golden images from the renderer instead of checking
// it against them: go test ./internal/render -run Golden -update. Look at
// what changed before committing them.
var update = flag.Bool("update", false, "rewrite the golden images in testdata")

// goldenTolerance is how far a value may stray from its golden image, in
// 16-bit terms: an eighth of an 8-bit step, for floating point that rounds
// differently elsewhere (fused multiply-adds on arm64), and far less than
// any change to the art.
const goldenTolerance = 32

// The art at fixed times, with no sources, matches the committed images.
func TestGolden(t *testing.T) {
	for _, c := range []struct {
		name string
		at   time.Time
	}{
		{"night", equinoxNight},
		{"dawn", equinoxDawn},
		{"noon", equinoxNoon},
		{"dusk", equinoxDusk},
	} {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join("testdata", "golden", c.name+".png")
			f := greenwich.Render(c.at, State{})
			if *update {
				if err := writeGolden(path, f); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := readGolden(path)
			if err != nil {
				t.Fatalf("%v (run with -update to write it)", err)
			}
			if want.Width != f.Width || want.Height != f.Height {
				t.Fatalf("the frame is %dx%d, the golden image %dx%d", f.Width, f.Height, want.Width, want.Height)
			}
			for i := range f.Pix {
				if d := abs(int(f.Pix[i]) - int(want.Pix[i])); d > goldenTolerance {
					p := i / 3
					t.Fatalf("pixel (%d, %d) is %v, the golden image has %v (run with -update if the change is meant)",
						p%f.Width, p/f.Width, f.At(p%f.Width, p/f.Width), want.At(p%f.Width, p/f.Width))
				}
			}
		})
	}
}

func writeGolden(path string, f Frame) error {
	img := f.RGBA64()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(out, img); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func readGolden(path string) (Frame, error) {
	in, err := os.Open(path)
	if err != nil {
		return Frame{}, err
	}
	defer in.Close()
	img, err := png.Decode(in)
	if err != nil {
		return Frame{}, fmt.Errorf("%s: %w", path, err)
	}
	b := img.Bounds()
	f := Frame{Width: b.Dx(), Height: b.Dy(), Pix: make([]uint16, 3*b.Dx()*b.Dy())}
	for y := range f.Height {
		for x := range f.Width {
			r, g, bl, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			copy(f.Pix[3*(y*f.Width+x):], []uint16{uint16(r), uint16(g), uint16(bl)})
		}
	}
	return f, nil
}
