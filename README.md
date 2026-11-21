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
  receives frames from the renderer over a unix socket.

The design, and the map of the work, is
[#1](https://github.com/corygyarmathy/matrix-display/issues/1).

## Running the preview

The preview renders simulated time to a video or image, with no panels needed.
It lands with [#6](https://github.com/corygyarmathy/matrix-display/issues/6):

```sh
nix run . -- preview --from … --to … --speed … --out day.mp4
```

## Building

```sh
nix build .#matrix-display
nix build .#matrix-display-sink
nix flake check   # what CI runs
nix develop       # Go, a C++ toolchain and the library
nix fmt
```

Consumers use `overlays.default`, which adds both packages and links the sink
against the consumer's own `rpi-rgb-led-matrix`.

## Licence

GPL-2.0-only (see [LICENSE](LICENSE)), because the sink links
rpi-rgb-led-matrix.
