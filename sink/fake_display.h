// A Display with no panels, for tests and the fake sink.

#ifndef MATRIX_DISPLAY_SINK_FAKE_DISPLAY_H
#define MATRIX_DISPLAY_SINK_FAKE_DISPLAY_H

#include <chrono>
#include <cstdint>
#include <thread>
#include <vector>

#include "display.h"

namespace sink {

class FakeDisplay : public Display {
public:
  // `refresh` is how long Show takes, standing in for the wait for vsync.
  FakeDisplay(int width, int height,
              std::chrono::microseconds refresh = std::chrono::microseconds{0})
      : width_(width), height_(height), refresh_(refresh),
        shown_(static_cast<size_t>(width) * height * 3) {}

  int width() const override { return width_; }
  int height() const override { return height_; }

  void Show(const uint8_t *rgb) override {
    shown_.assign(rgb, rgb + shown_.size());
    ++shows_;
    if (refresh_.count() > 0) {
      std::this_thread::sleep_for(refresh_);
    }
  }

  // What the last Show put on screen.
  const std::vector<uint8_t> &shown() const { return shown_; }
  int shows() const { return shows_; }

private:
  const int width_, height_;
  const std::chrono::microseconds refresh_;
  std::vector<uint8_t> shown_;
  int shows_ = 0;
};

} // namespace sink

#endif
