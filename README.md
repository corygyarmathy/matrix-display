# matrix-display

An ambient display for four 64×64 HUB75 LED panels in a row above a desk,
driven by a Raspberry Pi 4. It shows a slow gradient that follows the sun, and
shows homelab alerts and upcoming calendar events only when they need
attention.

It is two programs:

- `matrix-display` (`cmd/matrix-display`, Go): the renderer. It runs
  unprivileged, reads from the network and computes every frame.
- `matrix-display-sink` (`sink/`, C++): the only process that touches the
  panels, through
  [rpi-rgb-led-matrix](https://github.com/hzeller/rpi-rgb-led-matrix). It
  receives frames from the renderer over a unix socket, in the protocol that
  [sink/PROTOCOL.md](sink/PROTOCOL.md) describes.

The design, and the map of the work, is
[#1](https://github.com/corygyarmathy/matrix-display/issues/1).

## Running the preview

The preview renders simulated time to a video or image, with no panels needed.
It lands with [#6](https://github.com/corygyarmathy/matrix-display/issues/6):

```sh
nix run . -- preview --from … --to … --speed … --out day.mp4
```

## Checking the panels

With the sink running, `test-pattern` sends a sweeping bar (which tears if
frames aren't swapped on vsync) above a dark gradient (which bands if dithering
isn't working). Interrupting it should fade the panels to black.

```sh
matrix-display test-pattern --socket /run/led-matrix/frames.sock --brightness 3
```

## Building

```sh
nix build .#matrix-display
nix build .#matrix-display-sink
nix flake check   # what CI runs
nix develop       # Go, a C++ toolchain and the library
make -C sink check  # the sink's unit tests, in the dev shell
nix fmt
```

Consumers use `overlays.default`, which adds both packages and links the sink
against the consumer's own `rpi-rgb-led-matrix`, or this repo's pinned copy
when the consumer has none.

## Licence

GPL-2.0-only (see [LICENSE](LICENSE)), because the sink links
rpi-rgb-led-matrix.
