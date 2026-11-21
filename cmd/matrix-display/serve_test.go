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

// stream sends rendered frames, and reconnects when the sink restarts.
func TestStreamReconnects(t *testing.T) {
	const width, height = 4, 2
	path := filepath.Join(t.TempDir(), "sink.sock")
	l, err := net.ListenUnix("unixpacket", &net.UnixAddr{Name: path, Net: "unixpacket"})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		stream(ctx, path, time.Millisecond, sun.Greenwich, func() render.State { return render.State{} })
	}()

	want := render.Renderer{Width: width, Height: height, Location: sun.Greenwich}.Render(time.Now(), render.State{})
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
			// The sun barely moves during the test: a few steps of 65535 at most.
			if got := int(binary.LittleEndian.Uint16(msg[sinkproto.HeaderSize:])); got < int(want.Pix[0])-64 || got > int(want.Pix[0])+64 {
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
