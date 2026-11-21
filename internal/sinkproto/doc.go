// Package sinkproto is the renderer's end of the frame protocol to
// matrix-display-sink. sink/PROTOCOL.md is the specification, and
// sink/protocol.h the sink's end; change all three together.
//
// The connection is a unix SOCK_SEQPACKET socket, so each message arrives
// whole. All integers are little-endian.
//
// On connecting, the client receives one panel message from the sink:
//
//	offset  size  field
//	     0     4  magic, "MDSF"
//	     4     4  version, 1
//	     8     4  width, in pixels
//	    12     4  height, in pixels
//
// After that, the client sends one frame message per frame:
//
//	offset  size  field
//	     0     4  magic, "MDSF"
//	     4     4  version, 1
//	     8     8  sequence, counting up from 0 on each connection
//	    16     —  width × height × 3 uint16 pixel values (see AppendFrame)
//
// The sink drops a frame message of the wrong magic, version or size, and
// fades to black when the client disconnects or sends nothing for 3 seconds.
package sinkproto
