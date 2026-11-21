#include "protocol.h"

#include <cstring>

namespace sink {
namespace {

uint32_t ReadU32(const uint8_t *p) {
  return uint32_t{p[0]} | uint32_t{p[1]} << 8 | uint32_t{p[2]} << 16 |
         uint32_t{p[3]} << 24;
}

uint64_t ReadU64(const uint8_t *p) {
  return uint64_t{ReadU32(p)} | uint64_t{ReadU32(p + 4)} << 32;
}

} // namespace

const char *Describe(ParseResult result) {
  switch (result) {
  case ParseResult::kOk:
    return "ok";
  case ParseResult::kShort:
    return "shorter than the header";
  case ParseResult::kBadMagic:
    return "bad magic";
  case ParseResult::kBadVersion:
    return "unsupported version";
  case ParseResult::kBadSize:
    return "wrong size for the panels";
  }
  return "unknown";
}

ParseResult ParseFrame(const uint8_t *message, size_t length, int width,
                       int height, Header *header) {
  if (length < kHeaderSize) {
    return ParseResult::kShort;
  }
  if (std::memcmp(message, kMagic, sizeof kMagic) != 0) {
    return ParseResult::kBadMagic;
  }
  header->version = ReadU32(message + 4);
  header->sequence = ReadU64(message + 8);
  if (header->version != kVersion) {
    return ParseResult::kBadVersion;
  }
  if (length != MessageSize(width, height)) {
    return ParseResult::kBadSize;
  }
  return ParseResult::kOk;
}

void DecodePixels(const uint8_t *bytes, size_t count, uint16_t *out) {
  for (size_t i = 0; i < count; ++i) {
    out[i] = static_cast<uint16_t>(bytes[2 * i] | bytes[2 * i + 1] << 8);
  }
}

} // namespace sink
