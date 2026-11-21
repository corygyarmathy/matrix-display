#include "dither.h"

namespace sink {
namespace {

// 65535 / 255: one 8-bit step in 16-bit units.
constexpr uint32_t kStep = 257;

// splitmix64: small, and good enough to spread the starting phases.
uint64_t SplitMix64(uint64_t *state) {
  uint64_t z = (*state += 0x9e3779b97f4a7c15);
  z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9;
  z = (z ^ (z >> 27)) * 0x94d049bb133111eb;
  return z ^ (z >> 31);
}

} // namespace

Ditherer::Ditherer(size_t channels, uint64_t seed) : error_(channels) {
  for (uint16_t &e : error_) {
    e = static_cast<uint16_t>(SplitMix64(&seed) % kStep);
  }
}

void Ditherer::Step(const uint16_t *in, uint32_t gain, uint8_t *out) {
  const size_t n = error_.size();
  for (size_t i = 0; i < n; ++i) {
    // At most 65535 + 256, so the quotient is at most 255.
    const uint32_t value = (in[i] * gain) >> 16;
    const uint32_t total = value + error_[i];
    const uint32_t level = total / kStep;
    out[i] = static_cast<uint8_t>(level);
    error_[i] = static_cast<uint16_t>(total - level * kStep);
  }
}

} // namespace sink
