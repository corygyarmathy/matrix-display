package sinkproto

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"time"
)

const (
	// Version is the protocol version this package speaks.
	Version = 1
	// HeaderSize is the size of a frame message's header, before the pixels.
	HeaderSize = 16
	// PanelMessageSize is the size of the panel message the sink sends on
	// connecting.
	PanelMessageSize = 16
)

// panelTimeout is how long Dial waits for the panel message.
const panelTimeout = 5 * time.Second

// Magic starts every message.
var Magic = [4]byte{'M', 'D', 'S', 'F'}

// MessageSize is the size of a whole message for a width × height frame.
func MessageSize(width, height int) int {
	return HeaderSize + width*height*3*2
}

// AppendFrame appends the message for one frame to dst and returns it. pix
// holds width × height × 3 values: rows top to bottom, each row left to
// right, each pixel red, green, blue, with 65535 as full brightness.
func AppendFrame(dst []byte, sequence uint64, pix []uint16) []byte {
	dst = append(dst, Magic[:]...)
	dst = binary.LittleEndian.AppendUint32(dst, Version)
	dst = binary.LittleEndian.AppendUint64(dst, sequence)
	for _, v := range pix {
		dst = binary.LittleEndian.AppendUint16(dst, v)
	}
	return dst
}

// ParsePanel parses the panel message the sink sends on connecting, and
// returns the panels' size.
func ParsePanel(msg []byte) (width, height int, err error) {
	if len(msg) != PanelMessageSize {
		return 0, 0, fmt.Errorf("panel message is %d bytes, want %d", len(msg), PanelMessageSize)
	}
	if !bytes.Equal(msg[:4], Magic[:]) {
		return 0, 0, fmt.Errorf("panel message has magic % x, want % x", msg[:4], Magic[:])
	}
	if v := binary.LittleEndian.Uint32(msg[4:]); v != Version {
		return 0, 0, fmt.Errorf("the sink speaks protocol version %d, this writer %d", v, Version)
	}
	width = int(binary.LittleEndian.Uint32(msg[8:]))
	height = int(binary.LittleEndian.Uint32(msg[12:]))
	if width <= 0 || height <= 0 {
		return 0, 0, fmt.Errorf("panel message gives a %dx%d display", width, height)
	}
	return width, height, nil
}

// Writer sends frames to the sink over one connection. It does not
// reconnect: when the sink restarts, WriteFrame fails and the caller dials
// again.
type Writer struct {
	conn          *net.UnixConn
	width, height int
	sequence      uint64
	buf           []byte
}

// Dial connects to the sink's socket at path, and waits for the sink to say
// the panels' size.
func Dial(path string) (*Writer, error) {
	conn, err := net.DialUnix("unixpacket", nil, &net.UnixAddr{Name: path, Net: "unixpacket"})
	if err != nil {
		return nil, err
	}
	// One byte more than the message, so a longer one isn't read as truncated
	// to the right size.
	msg := make([]byte, PanelMessageSize+1)
	if err := conn.SetReadDeadline(time.Now().Add(panelTimeout)); err != nil {
		conn.Close()
		return nil, err
	}
	n, err := conn.Read(msg)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("waiting for the sink's panel message: %w", err)
	}
	width, height, err := ParsePanel(msg[:n])
	if err != nil {
		conn.Close()
		return nil, err
	}
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		conn.Close()
		return nil, err
	}
	return &Writer{conn: conn, width: width, height: height}, nil
}

// Width is the panels' width in pixels, as the sink reported it.
func (w *Writer) Width() int { return w.width }

// Height is the panels' height in pixels, as the sink reported it.
func (w *Writer) Height() int { return w.height }

// WriteFrame sends one frame (see AppendFrame for pix's layout), blocking
// while the sink's queue is full. pix must hold Width() × Height() × 3
// values.
func (w *Writer) WriteFrame(pix []uint16) error {
	if want := w.width * w.height * 3; len(pix) != want {
		return fmt.Errorf("frame has %d values, want %d for %dx%d panels", len(pix), want, w.width, w.height)
	}
	w.buf = AppendFrame(w.buf[:0], w.sequence, pix)
	n, err := w.conn.Write(w.buf)
	if err != nil {
		return err
	}
	if n != len(w.buf) {
		return fmt.Errorf("sent %d of %d bytes", n, len(w.buf))
	}
	w.sequence++
	return nil
}

// Close closes the connection; the sink then fades the panels to black.
func (w *Writer) Close() error {
	return w.conn.Close()
}
