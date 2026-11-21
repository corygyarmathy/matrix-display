// matrix-display-sink: the only process that touches the panels. It receives
// frames from the renderer over a unix socket that systemd passes it, and
// shows them. PROTOCOL.md describes what it receives, and
// https://github.com/corygyarmathy/matrix-display/issues/5 the design.
//
// It starts as root, for the GPIO, and the library drops privileges to
// --led-drop-priv-user once the hardware is set up. It never creates a
// socket.

#include <csignal>
#include <cstdint>
#include <cstdio>
#include <random>

#include "activation.h"
#include "led-matrix.h"
#include "signals.h"
#include "sink.h"

namespace {

// The panels, drawn off-screen and swapped in on vsync so frames never tear.
class MatrixDisplay : public sink::Display {
public:
  explicit MatrixDisplay(rgb_matrix::RGBMatrix *matrix)
      : matrix_(matrix), canvas_(matrix->CreateFrameCanvas()) {}

  // The canvas's size, after any pixel mapper, not --led-cols × --led-chain.
  int width() const override { return canvas_->width(); }
  int height() const override { return canvas_->height(); }

  void Show(const uint8_t *rgb) override {
    const int w = width(), h = height();
    for (int y = 0; y < h; ++y) {
      for (int x = 0; x < w; ++x) {
        const uint8_t *p = rgb + 3 * (static_cast<size_t>(y) * w + x);
        canvas_->SetPixel(x, y, p[0], p[1], p[2]);
      }
    }
    canvas_ = matrix_->SwapOnVSync(canvas_);
  }

private:
  rgb_matrix::RGBMatrix *const matrix_;
  rgb_matrix::FrameCanvas *canvas_;
};

int Usage(const char *program) {
  std::fprintf(stderr, "usage: %s [--led-* flags]\n", program);
  rgb_matrix::PrintMatrixFlags(stderr);
  return 2;
}

} // namespace

int main(int argc, char *argv[]) {
  rgb_matrix::RGBMatrix::Options options;
  rgb_matrix::RuntimeOptions runtime;
  if (!rgb_matrix::ParseOptionsFromFlags(&argc, &argv, &options, &runtime)) {
    return Usage(argv[0]);
  }
  if (argc > 1) {
    std::fprintf(stderr, "unexpected argument %s\n", argv[1]);
    return Usage(argv[0]);
  }
  if (runtime.daemon > 0) {
    std::fprintf(stderr, "--led-daemon is not supported: systemd runs the "
                         "sink in the foreground\n");
    return 2;
  }
  // Brightness is applied in the pixel values (PROTOCOL.md).
  if (options.brightness != 100) {
    std::fprintf(stderr,
                 "ignoring --led-brightness=%d: brightness comes "
                 "from the renderer\n",
                 options.brightness);
    options.brightness = 100;
  }

  // Before the matrix, which drops privileges.
  const int listen_fd = sink::ListenFd();
  if (listen_fd < 0) {
    return 1;
  }

  rgb_matrix::RGBMatrix *matrix =
      rgb_matrix::RGBMatrix::CreateFromOptions(options, runtime);
  if (matrix == nullptr) {
    return 1;
  }
  matrix->SetBrightness(100);

  const volatile std::sig_atomic_t *stop = sink::StopOnSignals();

  {
    MatrixDisplay display(matrix);
    std::random_device random;
    const uint64_t seed = uint64_t{random()} << 32 | random();
    sink::Sink sink(listen_fd, &display, seed);
    std::fprintf(stderr, "showing frames on %dx%d pixels\n", display.width(),
                 display.height());
    sink.Run(stop);
  }

  matrix->Clear();
  delete matrix;
  return 0;
}
