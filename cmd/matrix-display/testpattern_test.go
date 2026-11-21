package main

import (
	"context"
	"strings"
	"testing"
)

func TestTestPatternNeedsSocket(t *testing.T) {
	var out strings.Builder
	err := run(context.Background(), []string{"test-pattern"}, &out)
	if err == nil || !strings.Contains(err.Error(), "--socket") {
		t.Fatalf("run(test-pattern) = %v, want an error naming --socket", err)
	}
}

func TestDrawTestPattern(t *testing.T) {
	const width, height = 16, 4
	pix := make([]uint16, width*height*3)
	drawTestPattern(pix, width, height, 3, 0.03)
	at := func(x, y int) []uint16 { return pix[3*(y*width+x):][:3] }

	// The bar starts at x = frame, in the top half only.
	if at(2, 0)[0] != 0 || at(3, 0)[0] == 0 || at(6, 1)[0] == 0 || at(7, 1)[0] != 0 {
		t.Errorf("top row = %v, want a 4-pixel bar from x=3", pix[:width*3])
	}

	// The gradient runs from black to 3% of the spike's colour, never down.
	if got := at(0, height-1); got[0] != 0 || got[1] != 0 || got[2] != 0 {
		t.Errorf("gradient starts at %v, want black", got)
	}
	// 160/255 × 65535 × 3%, rounded down.
	if got, want := at(width-1, height-1)[2], uint16(1233); got != want {
		t.Errorf("gradient's blue peak = %d, want %d", got, want)
	}
	for x := 1; x < width; x++ {
		for c := range 3 {
			if at(x, height-1)[c] < at(x-1, height-1)[c] {
				t.Errorf("gradient falls at x=%d, channel %d", x, c)
			}
		}
	}
}
