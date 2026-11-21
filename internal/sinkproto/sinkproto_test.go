package sinkproto

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// PROTOCOL.md's example, which sink/test.cc also checks.
func TestAppendFrameMatchesProtocolExample(t *testing.T) {
	want := []byte{
		0x4d, 0x44, 0x53, 0x46, 0x01, 0x00, 0x00, 0x00,
		0x05, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0xff, 0xff, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x02, 0x00, 0x03, 0x00,
	}
	got := AppendFrame(nil, 5, []uint16{65535, 0, 0, 1, 2, 3})
	if !bytes.Equal(got, want) {
		t.Errorf("AppendFrame = % x\nwant          % x", got, want)
	}
	if len(got) != MessageSize(2, 1) {
		t.Errorf("len = %d, MessageSize(2, 1) = %d", len(got), MessageSize(2, 1))
	}
}

// PROTOCOL.md's panel message example, which sink/test.cc also checks.
var panelExample = []byte{
	0x4d, 0x44, 0x53, 0x46, 0x01, 0x00, 0x00, 0x00,
	0x00, 0x01, 0x00, 0x00, 0x40, 0x00, 0x00, 0x00,
}

func TestParsePanel(t *testing.T) {
	width, height, err := ParsePanel(panelExample)
	if err != nil || width != 256 || height != 64 {
		t.Errorf("ParsePanel(example) = %d, %d, %v; want 256, 64, nil", width, height, err)
	}

	edited := func(i int, b byte) []byte {
		m := append([]byte(nil), panelExample...)
		m[i] = b
		return m
	}
	for name, msg := range map[string][]byte{
		"short":      panelExample[:PanelMessageSize-1],
		"long":       append(append([]byte(nil), panelExample...), 0),
		"bad magic":  edited(0, 'X'),
		"version 2":  edited(4, 2),
		"zero width": edited(9, 0),
	} {
		if _, _, err := ParsePanel(msg); err == nil {
			t.Errorf("ParsePanel(%s) succeeded, want an error", name)
		}
	}
}

// Dial refuses a sink that speaks another version, rather than sending it
// frames it would drop.
func TestDialRejectsOtherVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sink.sock")
	l, err := net.ListenUnix("unixpacket", &net.UnixAddr{Name: path, Net: "unixpacket"})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		conn, err := l.AcceptUnix()
		if err != nil {
			return
		}
		defer conn.Close()
		msg := append([]byte(nil), panelExample...)
		binary.LittleEndian.PutUint32(msg[4:], Version+1)
		conn.Write(msg)
		conn.Read(make([]byte, 1)) // Until the client closes.
	}()
	w, err := Dial(path)
	if err == nil {
		w.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "version") {
		t.Fatalf("Dial = %v, want an error naming the version", err)
	}
}

const fakeSinkEnv = "MATRIX_DISPLAY_FAKE_SINK"

// TestRoundTrip sends frames with Writer to the sink's own code, built with a
// fake display (sink/fake_main.cc), and checks that they arrive unchanged.
// The flake's sink-roundtrip check runs it; elsewhere, point
// MATRIX_DISPLAY_FAKE_SINK at `make -C sink matrix-display-sink-fake`.
func TestRoundTrip(t *testing.T) {
	fake := os.Getenv(fakeSinkEnv)
	if fake == "" {
		t.Skip(fakeSinkEnv + " is not set")
	}
	const width, height = 8, 4
	sink := startFakeSink(t, fake, width, height)

	w, err := Dial(sink.path)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if w.Width() != width || w.Height() != height {
		t.Fatalf("the sink reported %dx%d panels, want %dx%d", w.Width(), w.Height(), width, height)
	}
	if err := w.WriteFrame(distinctFrame(width-1, height, 0)); err == nil {
		t.Fatal("WriteFrame sent a frame of the wrong size")
	}
	for seq := range uint64(5) {
		pix := distinctFrame(width, height, seq)
		if err := w.WriteFrame(pix); err != nil {
			t.Fatalf("frame %d: %v", seq, err)
		}
		if got, want := sink.next(t, width, height), AppendFrame(nil, seq, pix); !bytes.Equal(got, want) {
			t.Fatalf("frame %d arrived as % x\nwant % x", seq, got, want)
		}
	}

	// A second client replaces the first. A message in a version the sink
	// doesn't speak, or of the wrong size, never arrives.
	// Its messages go straight to the connection, past WriteFrame's checks.
	second, err := Dial(sink.path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	bad := AppendFrame(nil, 0, distinctFrame(width, height, 10))
	binary.LittleEndian.PutUint32(bad[4:], Version+1)
	short := AppendFrame(nil, 1, distinctFrame(width-1, height, 11))
	good := AppendFrame(nil, 2, distinctFrame(width, height, 12))
	for _, m := range [][]byte{bad, short, good} {
		if _, err := second.conn.Write(m); err != nil {
			t.Fatal(err)
		}
	}
	if got := sink.next(t, width, height); !bytes.Equal(got, good) {
		t.Fatalf("after a bad message, got % x\nwant % x", got, good)
	}

	// The sink closed the first connection.
	deadline := time.Now().Add(5 * time.Second)
	for w.WriteFrame(distinctFrame(width, height, 0)) == nil {
		if time.Now().After(deadline) {
			t.Fatal("the first client can still send after the second connected")
		}
		time.Sleep(10 * time.Millisecond)
	}

	sink.stop(t)
}

// distinctFrame is a frame whose every value differs, including in its high byte.
func distinctFrame(width, height int, seq uint64) []uint16 {
	pix := make([]uint16, width*height*3)
	for i := range pix {
		pix[i] = uint16(uint64(i)*2731 + seq*40503)
	}
	return pix
}

type fakeSink struct {
	path   string
	cmd    *exec.Cmd
	stdout io.Reader
	stderr *bytes.Buffer
}

// startFakeSink runs the fake sink as systemd would: with a listening socket
// on fd 3, and LISTEN_PID set to its own pid.
func startFakeSink(t *testing.T, fake string, width, height int) *fakeSink {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sink.sock")
	l, err := net.ListenUnix("unixpacket", &net.UnixAddr{Name: path, Net: "unixpacket"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	f, err := l.File()
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	cmd := exec.Command("/bin/sh", "-c", `LISTEN_PID=$$ LISTEN_FDS=1 exec "$0" "$@"`,
		fake, "--width="+strconv.Itoa(width), "--height="+strconv.Itoa(height))
	cmd.ExtraFiles = []*os.File{f}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "LISTEN_") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	s := &fakeSink{path: path, cmd: cmd, stdout: stdout, stderr: new(bytes.Buffer)}
	cmd.Stderr = s.stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
		if t.Failed() {
			t.Logf("fake sink's log:\n%s", s.stderr)
		}
	})
	return s
}

// next reads the next frame the sink accepted.
func (s *fakeSink) next(t *testing.T, width, height int) []byte {
	t.Helper()
	got := make(chan error, 1)
	buf := make([]byte, MessageSize(width, height))
	go func() {
		_, err := io.ReadFull(s.stdout, buf)
		got <- err
	}()
	select {
	case err := <-got:
		if err != nil {
			t.Fatalf("reading the sink's frames: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no frame from the sink within 5s")
	}
	return buf
}

func (s *fakeSink) stop(t *testing.T) {
	t.Helper()
	if err := s.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if err := s.cmd.Wait(); err != nil {
		t.Fatalf("fake sink: %v", err)
	}
}
