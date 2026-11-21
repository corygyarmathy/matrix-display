// Package sinkproto is the renderer's end of the frame protocol to
// matrix-display-sink. sink/PROTOCOL.md is the specification, and
// sink/protocol.h the sink's end; change all three together.
package sinkproto

import (
	"encoding/binary"
	"fmt"
	"net"
)

const (
	// Version is the protocol version this package speaks.
	Version = 1
	// HeaderSize is the size of a message's header, before the pixels.
	HeaderSize = 16
)

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

// Writer sends frames to the sink over one connection. It does not
// reconnect: when the sink restarts, WriteFrame fails and the caller dials
// again.
type Writer struct {
	conn     *net.UnixConn
	sequence uint64
	buf      []byte
}

// Dial connects to the sink's socket at path.
func Dial(path string) (*Writer, error) {
	conn, err := net.DialUnix("unixpacket", nil, &net.UnixAddr{Name: path, Net: "unixpacket"})
	if err != nil {
		return nil, err
	}
	return &Writer{conn: conn}, nil
}

// WriteFrame sends one frame (see AppendFrame for pix's layout), blocking
// while the sink's queue is full. The sink drops a frame that isn't the size
// of its panels, so a wrong length here shows up only in the sink's log.
func (w *Writer) WriteFrame(pix []uint16) error {
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
