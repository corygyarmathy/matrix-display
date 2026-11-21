package main

import (
	"context"
	"encoding/binary"
	"math"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/corygyarmathy/matrix-display/internal/render"
	"github.com/corygyarmathy/matrix-display/internal/sinkproto"
	"github.com/corygyarmathy/matrix-display/internal/sun"
)

func TestServeNeedsFlags(t *testing.T) {
	var out strings.Builder
	for _, args := range [][]string{{"serve"}, {"serve", "--socket", "s"}, {"serve", "--location", "l"}} {
		if err := run(context.Background(), args, &out); err == nil || !strings.Contains(err.Error(), "required") {
			t.Errorf("run(%q) = %v, want an error about required flags", args, err)
		}
	}
}

// Bad art flags fail up front, before the location is read.
func TestServeRejectsArt(t *testing.T) {
	for _, flag := range [][]string{{"--east", "up"}, {"--ceiling", "-0.1"}, {"--reshape", "-1s"}} {
		args := append([]string{"serve", "--socket", "s", "--location", "/nonexistent"}, flag...)
		if err := run(context.Background(), args, new(strings.Builder)); err == nil || strings.Contains(err.Error(), "location") {
			t.Errorf("run(%q) = %v, want an error about the art", args, err)
		}
	}
}

// An --fps whose period isn't positive fails up front, rather than
// panicking in time.NewTicker once the sink is reached.
func TestFramePeriod(t *testing.T) {
	if p, err := framePeriod(100); err != nil || p != 10*time.Millisecond {
		t.Errorf("framePeriod(100) = %s, %v; want 10ms", p, err)
	}
	for _, fps := range []float64{0, -1, math.NaN(), math.Inf(1), 2e9} {
		if p, err := framePeriod(fps); err == nil {
			t.Errorf("framePeriod(%v) = %s, want an error", fps, p)
		}
	}
	for _, cmd := range []string{"serve", "test-pattern"} {
		args := []string{cmd, "--socket", "s", "--location", "l", "--fps", "2e9"}
		if cmd == "test-pattern" {
			args = []string{cmd, "--socket", "s", "--fps", "2e9"}
		}
		if err := run(context.Background(), args, new(strings.Builder)); err == nil || !strings.Contains(err.Error(), "--fps") {
			t.Errorf("run(%q) = %v, want an error about --fps", args, err)
		}
	}
}

// A bad location file's error doesn't quote it: it comes from a secret.
func TestServeHidesLocation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "location")
	if err := os.WriteFile(path, []byte("12.3456 not-a-longitude\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	err := run(context.Background(), []string{"serve", "--socket", "s", "--location", path}, &out)
	if err == nil || strings.Contains(err.Error(), "12.3456") || strings.Contains(out.String(), "12.3456") {
		t.Errorf("run(serve) = %v, want an error that doesn't quote the file", err)
	}
}

// Bad --from and --speed fail up front, before the location is read.
func TestServeRejectsClock(t *testing.T) {
	for _, flag := range [][]string{
		{"--from", "noon"}, {"--speed", "0"}, {"--speed", "-1"}, {"--speed", "NaN"}, {"--speed", "1e6"},
	} {
		args := append([]string{"serve", "--socket", "s", "--location", "/nonexistent"}, flag...)
		if err := run(context.Background(), args, new(strings.Builder)); err == nil || !strings.Contains(err.Error(), flag[0]) {
			t.Errorf("run(%q) = %v, want an error about %s", args, err, flag[0])
		}
	}
}

// The simulated clock starts at from on its first reading, and runs speed
// times faster than real time from then on.
func TestSimClock(t *testing.T) {
	from := time.Date(2025, 6, 21, 5, 0, 0, 0, time.UTC)
	real := time.Date(2026, 11, 21, 9, 0, 0, 0, time.UTC)
	clock := simClock(from, 720, func() time.Time { return real })
	if got := clock(); !got.Equal(from) {
		t.Errorf("first reading = %s, want %s", got, from)
	}
	real = real.Add(5 * time.Second)
	if got, want := clock(), from.Add(time.Hour); !got.Equal(want) {
		t.Errorf("after 5s at 720×, the clock reads %s, want %s", got, want)
	}
	// Past the largest Duration, it stops rather than wrapping back.
	real = real.Add(4 * 365 * 24 * time.Hour)
	clock = simClock(from, maxSpeed, func() time.Time { return real })
	clock()
	real = real.Add(4 * 365 * 24 * time.Hour)
	if got := clock(); !got.Equal(from.Add(math.MaxInt64)) {
		t.Errorf("after 4 years at %d×, the clock reads %s, want %s", maxSpeed, got, from.Add(math.MaxInt64))
	}
}

// stream sends frames rendered at the time its clock gives, and reconnects
// when the sink restarts.
func TestStreamReconnects(t *testing.T) {
	const width, height = 4, 2
	path := filepath.Join(t.TempDir(), "sink.sock")
	l, err := net.ListenUnix("unixpacket", &net.UnixAddr{Name: path, Net: "unixpacket"})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	// Art that barely moves, so the frames can be checked against one.
	art := render.Art{Reshape: 1000 * time.Hour}
	noon := time.Date(2025, 6, 21, 12, 0, 0, 0, time.UTC)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		stream(ctx, path, time.Millisecond, sun.Greenwich, art, func() time.Time { return noon }, func() render.State { return render.State{} })
	}()

	want := render.Renderer{Width: width, Height: height, Location: sun.Greenwich, Art: art}.Render(noon, render.State{})
	for conn := range 2 {
		c, err := l.AcceptUnix()
		if err != nil {
			t.Fatal(err)
		}
		panel := append(sinkproto.Magic[:], make([]byte, 12)...)
		binary.LittleEndian.PutUint32(panel[4:], sinkproto.Version)
		binary.LittleEndian.PutUint32(panel[8:], width)
		binary.LittleEndian.PutUint32(panel[12:], height)
		if _, err := c.Write(panel); err != nil {
			t.Fatal(err)
		}
		for seq := range uint64(3) {
			msg := make([]byte, sinkproto.MessageSize(width, height)+1)
			c.SetReadDeadline(time.Now().Add(5 * time.Second))
			n, err := c.Read(msg)
			if err != nil {
				t.Fatalf("connection %d, frame %d: %v", conn, seq, err)
			}
			if n != sinkproto.MessageSize(width, height) || binary.LittleEndian.Uint64(msg[8:]) != seq {
				t.Fatalf("connection %d: got a %d-byte message with sequence %d, want %d bytes, sequence %d",
					conn, n, binary.LittleEndian.Uint64(msg[8:]), sinkproto.MessageSize(width, height), seq)
			}
			if got := binary.LittleEndian.Uint16(msg[sinkproto.HeaderSize:]); got != want.Pix[0] {
				t.Errorf("connection %d: first value %d, want about %d", conn, got, want.Pix[0])
			}
		}
		// The sink restarts.
		c.Close()
	}

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("stream didn't return after its context ended")
	}
}
