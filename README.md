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

The preview renders simulated time to a video or image, with fixture state and
no panels, upscaled with gaps between the pixels so it reads like the strip:

```sh
nix run . -- preview --out day.mp4                    # today, 24 hours in a minute
nix run . -- preview --from 2025-12-21 --speed 720 --out winter.gif
nix run . -- preview --from 2025-06-21T21:00 --out dusk.png  # one frame
```

Times without a zone are local. It uses Greenwich unless `--location` names a
file, and `--stale` shows a data source as stale throughout, to preview the
marker. A monitor can't show how the panels render dark gradients, which are
near black and dithered by the sink, so check night palettes on the panels.

In a video, the art moves `--motion` times faster than real time (by default
`--speed / 60`): at a day a minute, the noise reshapes about once a second
rather than twice a frame. That is not a fair test of how fast it drifts, so
judge that in real time, at `--speed 1`, or on the panels.

## The art

The art is two colours of the hour mixed by soft noise that drifts across the
whole strip, warmer and brighter at the sun's end. Its palette follows the
sun's elevation from indigo at night, through rose and violet in twilight and
amber in golden hour, to a soft blue by day. The art direction is
[#4](https://github.com/corygyarmathy/matrix-display/issues/4). `serve` and
`preview` take the same flags for it:

- `--east left|right`: the end of the strip that faces east (default `left`).
- `--ceiling 0.4`: the brightest it gets, at noon, as a fraction of full
  brightness, as the library's `--led-brightness` would set it. The night
  field doesn't change with it, so below about 0.16, the night field's
  brightest, the day is dimmer than the night.
- `--reshape 24s` and `--drift 1`: how long the noise takes to change shape,
  and how many pixels a second it moves sideways.

Golden images of the art at dawn, noon, dusk and night are in
`internal/render/testdata/golden`, and the tests check against them. After a
change to the art that is meant, look at the change, then rewrite them:

```sh
go test ./internal/render -run Golden -update
```

## Running on the panels

```sh
matrix-display serve --socket /run/led-matrix/frames.sock --location "$CREDENTIALS_DIRECTORY/location"
```

`--location` is a file holding the latitude and longitude, such as
`51.4769, -0.0005`. On the host it comes from a secret (`LoadCredential=`):
real coordinates never go in this repository, and the renderer never logs
them. `serve` reconnects whenever the sink restarts. While any data source
has been unreachable for a few minutes, the bottom-right pixel glows dim amber.

To see another time of day on the panels, `--from` shows that time instead of
now, and `--speed` runs the clock faster, up to a day a second. The sun and the
palette follow the simulated clock, and the art still moves in real time. Stop
the service first, and start it again afterwards, so that the sink is showing
the real time again:

```sh
matrix-display serve --socket /run/led-matrix/frames.sock --location "$LOCATION" --from 2026-06-21T12:00           # noon, held
matrix-display serve --socket /run/led-matrix/frames.sock --location "$LOCATION" --from 2026-06-21T03:00 --speed 720  # a day in two minutes
```

## Checking the panels

With the sink running, `test-pattern` sends a sweeping bar (which tears if
frames aren't swapped on vsync) above a dark gradient (which bands if dithering
isn't working), at the size the sink reports. Interrupting it should fade the
panels to black. Stop the renderer first: the sink serves one client, and each
new connection replaces the last.

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
