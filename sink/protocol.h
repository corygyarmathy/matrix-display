// The frame protocol between the renderer and the sink. PROTOCOL.md is the
// specification; internal/sinkproto is the Go end.

#ifndef MATRIX_DISPLAY_SINK_PROTOCOL_H
#define MATRIX_DISPLAY_SINK_PROTOCOL_H

#include <cstddef>
#include <cstdint>

namespace sink {

constexpr uint8_t kMagic[4] = {'M', 'D', 'S', 'F'};
constexpr uint32_t kVersion = 1;
constexpr size_t kHeaderSize = 16;

// The panel message: what the sink sends a client as soon as it connects.
constexpr size_t kPanelMessageSize = 16;

// Writes the panel message for a width × height display to `out`.
void EncodePanel(int width, int height, uint8_t out[kPanelMessageSize]);

// The size of a whole message for a width × height frame.
constexpr size_t MessageSize(int width, int height) {
  return kHeaderSize + static_cast<size_t>(width) * height * 3 * 2;
}

enum class ParseResult {
  kOk,
  kShort,      // Shorter than the header.
  kBadMagic,   // Not a frame at all.
  kBadVersion, // A frame, but not one this sink speaks.
  kBadSize,    // The right header, but not width × height pixels.
};

const char *Describe(ParseResult result);

struct Header {
  uint32_t version;
  uint64_t sequence;
};

// Checks a whole message of `length` bytes against a width × height frame.
// `header` is filled in whenever the header itself parsed (kOk, kBadVersion
// and kBadSize), so the caller can log it.
ParseResult ParseFrame(const uint8_t *message, size_t length, int width,
                       int height, Header *header);

// Decodes `count` little-endian 16-bit values from `bytes`.
void DecodePixels(const uint8_t *bytes, size_t count, uint16_t *out);

} // namespace sink

#endif
