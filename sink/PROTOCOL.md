# Frame protocol

How the renderer (`matrix-display`) sends frames to the sink
(`matrix-display-sink`). The two ends live in `internal/sinkproto` (Go) and
`sink/protocol.h` (C++); change all three together.

## Transport

A unix socket of type `SOCK_SEQPACKET`, so each message is one frame, and
messages arrive whole and in order. systemd creates the socket (socket
activation) and decides its path, owner and mode; the sink never creates or
binds a socket itself.

The sink serves one client. A new connection replaces the current one, which
the sink closes.

All integers are little-endian.

## Panel message

As soon as it accepts a connection, the sink sends the client one 16-byte
message with the panels' size. It is the only message the sink sends.

| Offset | Size | Field   | Value                         |
| -----: | ---: | ------- | ----------------------------- |
|      0 |    4 | magic   | the bytes `MDSF`              |
|      4 |    4 | version | `1`                           |
|      8 |    4 | width   | the panels' width, in pixels  |
|     12 |    4 | height  | the panels' height, in pixels |

`width` and `height` are the size of the sink's canvas, as its `--led-*` flags
configure it after any pixel mapper: 256 × 64 on rpi01. A client that gets a
version it doesn't speak should close the connection rather than send frames
the sink would drop.

A client reads the panel message before it closes the connection. If it
closes with the panel message unread, the sink sees a reset instead of its
last frames.

### Example

256 × 64:

```text
4d 44 53 46  01 00 00 00  00 01 00 00  40 00 00 00
```

Both ends test against these bytes.

## Frame message

One message per frame: a 16-byte header, then the pixels.

| Offset | Size | Field    | Value                                     |
| -----: | ---: | -------- | ----------------------------------------- |
|      0 |    4 | magic    | the bytes `MDSF`                          |
|      4 |    4 | version  | `1`                                       |
|      8 |    8 | sequence | counts up from 0 on each new connection   |
|     16 |    — | pixels   | `width × height × 3` unsigned 16-bit ints |

The pixels are rows top to bottom, each row left to right, each pixel red,
green, blue. `width` and `height` are the ones the panel message gave (so on
rpi01 a frame message is 98,320 bytes). They are not in the header, and the
sink rejects a message of any other size.

A value of 65535 is full brightness. The values are what the library's 8-bit
input would be, scaled by 257, so brightness and colour correction are already
applied in them: the library's CIE1931 correction still runs after the sink
dithers to 8 bits, and the sink always runs the panels at
`SetBrightness(100)`.

The sequence number is for logs only: the sink notes a gap, and shows the frame
anyway.

### Example

A 2 × 1 frame, sequence 5, with a red pixel `(65535, 0, 0)` and a dim grey one
`(1, 2, 3)`:

```text
4d 44 53 46  01 00 00 00  05 00 00 00 00 00 00 00
ff ff 00 00 00 00  01 00 02 00 03 00
```

Both ends test against these bytes.

## What the sink does with it

- A message with the wrong magic, version or size is dropped and logged, and
  never shown. The connection stays open.
- Frames arrive at whatever rate the renderer chooses. On every panel refresh,
  the sink temporally dithers the latest frame down to 8 bits and swaps it in
  on vsync.
- When the client disconnects, or sends nothing for 3 seconds, the sink fades
  the last frame to black over 2 seconds. A new frame cancels the fade.
- With no frame, the panels are black.

## Limits

A unix `SOCK_SEQPACKET` message must fit in the sender's send buffer
(`net.core.wmem_default`, 212,992 bytes by default). A larger panel than
rpi01's would need a bigger buffer, or the renderer's `write` fails with
`EMSGSIZE`.
