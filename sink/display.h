// What the sink draws on: the panels in production, a fake in tests.

#ifndef MATRIX_DISPLAY_SINK_DISPLAY_H
#define MATRIX_DISPLAY_SINK_DISPLAY_H

#include <cstdint>

namespace sink {

class Display {
public:
  virtual ~Display() = default;
  virtual int width() const = 0;
  virtual int height() const = 0;

  // Shows width × height × 3 bytes of RGB, rows top to bottom, at the next
  // refresh, and returns once it is on screen. The panels refresh at about
  // 100 Hz, so this is what paces the sink.
  virtual void Show(const uint8_t *rgb) = 0;
};

} // namespace sink

#endif
