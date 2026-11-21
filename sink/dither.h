// Temporal dithering from the protocol's 16 bits per channel down to the
// library's 8. It runs once per panel refresh, so that each refresh shows a
// different rounding and the eye averages them; the spike (#2) showed that
// anything slower looks grainy.

#ifndef MATRIX_DISPLAY_SINK_DITHER_H
#define MATRIX_DISPLAY_SINK_DITHER_H

#include <cstddef>
#include <cstdint>
#include <vector>

namespace sink {

// 1.0 for Ditherer::Step's gain.
constexpr uint32_t kFullGain = 1u << 16;

class Ditherer {
public:
  // `channels` is width × height × 3. Each channel's error accumulator starts
  // at a value drawn from `seed`, so that neighbouring pixels of the same
  // colour don't step up together.
  Ditherer(size_t channels, uint64_t seed);

  // Writes one refresh's worth of 8-bit values for `in`, each scaled by
  // gain / kFullGain first (gain ≤ kFullGain). Over N calls with the same
  // input, each output averages to in × gain / kFullGain / 257 within 1/N.
  void Step(const uint16_t *in, uint32_t gain, uint8_t *out);

private:
  // Each is the remainder, in 1/257ths of an 8-bit step, not yet shown.
  // Always below 257.
  std::vector<uint16_t> error_;
};

} // namespace sink

#endif
