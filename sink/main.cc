// matrix-display-sink: the only process that touches the panels. It receives
// frames from the renderer over a unix socket and shows them. See
// https://github.com/corygyarmathy/matrix-display/issues/5 for the design.
//
// For now this only parses the library's --led-* flags, which proves the
// build links against rpi-rgb-led-matrix. It does not open the hardware.

#include <cstdio>

#include "led-matrix.h"

int main(int argc, char *argv[]) {
  rgb_matrix::RGBMatrix::Options options;
  rgb_matrix::RuntimeOptions runtime;
  if (!rgb_matrix::ParseOptionsFromFlags(&argc, &argv, &options, &runtime)) {
    std::fprintf(stderr, "usage: %s [--led-* flags]\n", argv[0]);
    rgb_matrix::PrintMatrixFlags(stderr);
    return 1;
  }
  std::fprintf(
      stderr,
      "matrix-display-sink: %dx%d panels, %d chained; not implemented yet\n",
      options.cols, options.rows, options.chain_length);
  return 0;
}
