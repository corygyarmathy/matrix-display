// Unit tests for the sink, run by `make check`. The Go test in
// internal/sinkproto covers the other end, against matrix-display-sink-fake.

#include <cerrno>
#include <cmath>
#include <cstdint>
#include <cstdio>
#include <cstring>
#include <fcntl.h>
#include <random>
#include <string>
#include <sys/socket.h>
#include <sys/un.h>
#include <unistd.h>
#include <vector>

#include "dither.h"
#include "fake_display.h"
#include "protocol.h"
#include "sink.h"

namespace {

int failures = 0;

#define CHECK(cond)                                                            \
  do {                                                                         \
    if (!(cond)) {                                                             \
      std::fprintf(stderr, "%s:%d: CHECK(%s) failed\n", __FILE__, __LINE__,    \
                   #cond);                                                     \
      ++failures;                                                              \
    }                                                                          \
  } while (0)

using sink::ParseResult;

// PROTOCOL.md's example: 2 × 1, sequence 5, (65535, 0, 0) and (1, 2, 3).
const std::vector<uint8_t> kExample = {
    0x4d, 0x44, 0x53, 0x46, 0x01, 0x00, 0x00, 0x00, 0x05, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xff, 0xff, 0x00, 0x00,
    0x00, 0x00, 0x01, 0x00, 0x02, 0x00, 0x03, 0x00,
};

ParseResult Parse(const std::vector<uint8_t> &message, int width, int height,
                  sink::Header *header) {
  return sink::ParseFrame(message.data(), message.size(), width, height,
                          header);
}

// A whole message, as the Go writer would build it.
std::vector<uint8_t> Encode(uint64_t sequence,
                            const std::vector<uint16_t> &pixels,
                            uint32_t version = sink::kVersion) {
  std::vector<uint8_t> out(sink::kMagic, sink::kMagic + 4);
  for (int i = 0; i < 4; ++i) {
    out.push_back(static_cast<uint8_t>(version >> (8 * i)));
  }
  for (int i = 0; i < 8; ++i) {
    out.push_back(static_cast<uint8_t>(sequence >> (8 * i)));
  }
  for (uint16_t v : pixels) {
    out.push_back(static_cast<uint8_t>(v));
    out.push_back(static_cast<uint8_t>(v >> 8));
  }
  return out;
}

void TestParseGoodFrame() {
  sink::Header header{};
  CHECK(Parse(kExample, 2, 1, &header) == ParseResult::kOk);
  CHECK(header.version == 1);
  CHECK(header.sequence == 5);
  uint16_t pixels[6];
  sink::DecodePixels(kExample.data() + sink::kHeaderSize, 6, pixels);
  const uint16_t want[6] = {65535, 0, 0, 1, 2, 3};
  CHECK(std::memcmp(pixels, want, sizeof want) == 0);
  CHECK(Encode(5, {65535, 0, 0, 1, 2, 3}) == kExample);
}

void TestParseShortFrame() {
  sink::Header header{};
  for (size_t n : {size_t{0}, size_t{4}, sink::kHeaderSize - 1}) {
    const std::vector<uint8_t> message(kExample.begin(), kExample.begin() + n);
    CHECK(Parse(message, 2, 1, &header) == ParseResult::kShort);
  }
}

void TestParseBadMagic() {
  sink::Header header{};
  std::vector<uint8_t> message = kExample;
  message[0] = 'X';
  CHECK(Parse(message, 2, 1, &header) == ParseResult::kBadMagic);
}

void TestParseWrongVersion() {
  sink::Header header{};
  for (uint32_t version : {0u, 2u, 0x01000000u}) {
    const auto message = Encode(5, {65535, 0, 0, 1, 2, 3}, version);
    CHECK(Parse(message, 2, 1, &header) == ParseResult::kBadVersion);
    CHECK(header.version == version);
  }
}

void TestParseWrongSize() {
  sink::Header header{};
  std::vector<uint8_t> shorter(kExample.begin(), kExample.end() - 1);
  CHECK(Parse(shorter, 2, 1, &header) == ParseResult::kBadSize);
  std::vector<uint8_t> longer = kExample;
  longer.push_back(0);
  CHECK(Parse(longer, 2, 1, &header) == ParseResult::kBadSize);
  std::vector<uint8_t> header_only(kExample.begin(),
                                   kExample.begin() + sink::kHeaderSize);
  CHECK(Parse(header_only, 2, 1, &header) == ParseResult::kBadSize);
  // The right size for 1 × 2, the wrong panels.
  CHECK(Parse(kExample, 1, 1, &header) == ParseResult::kBadSize);
  CHECK(Parse(kExample, 1, 2, &header) == ParseResult::kOk);
}

// Over N refreshes, each output averages to its input within 1/N.
void TestDitherAverages() {
  std::mt19937 random(42);
  std::vector<uint16_t> in(3000);
  for (uint16_t &v : in) {
    v = static_cast<uint16_t>(random());
  }
  in[0] = 0;
  in[1] = 65535;
  in[2] = 1;
  in[3] = 256;
  for (int n : {1, 2, 7, 100, 1000}) {
    sink::Ditherer ditherer(in.size(), /*seed=*/n);
    std::vector<uint64_t> sums(in.size());
    std::vector<uint8_t> out(in.size());
    for (int step = 0; step < n; ++step) {
      ditherer.Step(in.data(), sink::kFullGain, out.data());
      for (size_t i = 0; i < in.size(); ++i) {
        sums[i] += out[i];
      }
    }
    int bad = 0;
    for (size_t i = 0; i < in.size(); ++i) {
      const double average = static_cast<double>(sums[i]) / n;
      if (std::fabs(average - in[i] / 257.0) >= 1.0 / n) {
        ++bad;
      }
    }
    CHECK(bad == 0);
  }
}

// A value that 8 bits can show exactly is shown exactly, every refresh.
void TestDitherExactValues() {
  std::vector<uint16_t> in(256 * 3);
  for (size_t i = 0; i < in.size(); ++i) {
    in[i] = static_cast<uint16_t>((i % 256) * 257);
  }
  for (uint64_t seed : {0ull, 1ull, 12345ull}) {
    sink::Ditherer ditherer(in.size(), seed);
    std::vector<uint8_t> out(in.size());
    int bad = 0;
    for (int step = 0; step < 50; ++step) {
      ditherer.Step(in.data(), sink::kFullGain, out.data());
      for (size_t i = 0; i < in.size(); ++i) {
        bad += out[i] != i % 256;
      }
    }
    CHECK(bad == 0);
  }
}

// Different pixels of the same colour don't all step on the same refresh.
void TestDitherPhasesDiffer() {
  const std::vector<uint16_t> in(300, 128); // Half of one step.
  sink::Ditherer ditherer(in.size(), /*seed=*/7);
  std::vector<uint8_t> out(in.size());
  ditherer.Step(in.data(), sink::kFullGain, out.data());
  int lit = 0;
  for (uint8_t v : out) {
    lit += v;
  }
  CHECK(lit > 50 && lit < 250);
}

void TestDitherGain() {
  const std::vector<uint16_t> in(30, 65535);
  sink::Ditherer ditherer(in.size(), /*seed=*/3);
  std::vector<uint8_t> out(in.size());
  ditherer.Step(in.data(), 0, out.data());
  CHECK(out == std::vector<uint8_t>(in.size(), 0));

  // At half gain, 65535 averages to 127.5.
  const int n = 100;
  uint64_t sum = 0;
  for (int step = 0; step < n; ++step) {
    ditherer.Step(in.data(), sink::kFullGain / 2, out.data());
    sum += out[0];
  }
  CHECK(std::fabs(static_cast<double>(sum) / n - 127.5) < 0.02);
}

// --- The refresh loop, over real sockets with a fake display.

using Clock = sink::Sink::Clock;
using std::chrono::milliseconds;

constexpr int kWidth = 4, kHeight = 2;

// A listening socket in the abstract namespace, so nothing touches the disk.
int Listen(sockaddr_un *address) {
  static int counter = 0;
  *address = {};
  address->sun_family = AF_UNIX;
  const std::string name = "matrix-display-sink-test-" +
                           std::to_string(getpid()) + "-" +
                           std::to_string(counter++);
  std::memcpy(address->sun_path + 1, name.data(), name.size());
  const int fd = socket(AF_UNIX, SOCK_SEQPACKET | SOCK_NONBLOCK, 0);
  const socklen_t length =
      static_cast<socklen_t>(offsetof(sockaddr_un, sun_path) + 1 + name.size());
  if (fd < 0 || bind(fd, reinterpret_cast<sockaddr *>(address), length) != 0 ||
      listen(fd, 4) != 0) {
    std::perror("listen");
    std::exit(1);
  }
  return fd;
}

int Connect(const sockaddr_un &address) {
  const int fd = socket(AF_UNIX, SOCK_SEQPACKET, 0);
  const size_t name_length = std::strlen(address.sun_path + 1);
  const socklen_t length =
      static_cast<socklen_t>(offsetof(sockaddr_un, sun_path) + 1 + name_length);
  if (fd < 0 ||
      connect(fd, reinterpret_cast<const sockaddr *>(&address), length) != 0) {
    std::perror("connect");
    std::exit(1);
  }
  return fd;
}

void Send(int fd, const std::vector<uint8_t> &message) {
  if (send(fd, message.data(), message.size(), 0) !=
      static_cast<ssize_t>(message.size())) {
    std::perror("send");
    std::exit(1);
  }
}

// A frame of one 8-bit-exact colour, so the dithered output is exact too.
std::vector<uint8_t> Solid(uint64_t sequence, uint8_t level) {
  return Encode(sequence,
                std::vector<uint16_t>(kWidth * kHeight * 3,
                                      static_cast<uint16_t>(level * 257)));
}

bool Shows(const sink::FakeDisplay &display, uint8_t level) {
  for (uint8_t v : display.shown()) {
    if (v != level) {
      return false;
    }
  }
  return true;
}

double MeanShown(const sink::FakeDisplay &display) {
  double sum = 0;
  for (uint8_t v : display.shown()) {
    sum += v;
  }
  return sum / display.shown().size();
}

struct Fixture {
  sockaddr_un address;
  sink::FakeDisplay display{kWidth, kHeight};
  sink::Sink sink;
  Clock::time_point t0 = Clock::now();
  std::vector<std::vector<uint8_t>> observed;

  Fixture() : sink(Listen(&address), &display, /*seed=*/9) {
    sink.set_on_frame([this](const uint8_t *message, size_t length) {
      observed.emplace_back(message, message + length);
    });
  }
};

void TestSinkStartsBlank() {
  Fixture f;
  CHECK(!f.sink.Idle());
  f.sink.Refresh(f.t0);
  CHECK(f.display.shows() == 1);
  CHECK(Shows(f.display, 0));
  CHECK(f.sink.Idle());
  f.sink.Refresh(f.t0);
  CHECK(f.display.shows() == 1); // Nothing new to show.
}

void TestSinkShowsLatestFrame() {
  Fixture f;
  const int client = Connect(f.address);
  Send(client, Solid(0, 10));
  Send(client, Solid(1, 200));
  f.sink.Refresh(f.t0);
  CHECK(Shows(f.display, 200));
  CHECK(f.observed.size() == 2);
  CHECK(!f.observed.empty() && f.observed.back() == Solid(1, 200));
  // The same frame keeps showing, refresh after refresh.
  f.sink.Refresh(f.t0 + milliseconds{10});
  f.sink.Refresh(f.t0 + milliseconds{20});
  CHECK(f.display.shows() == 3);
  CHECK(Shows(f.display, 200));
  close(client);
}

void TestSinkDropsBadMessages() {
  Fixture f;
  const int client = Connect(f.address);
  Send(client, Solid(0, 50));
  f.sink.Refresh(f.t0);
  Send(client, Encode(1, std::vector<uint16_t>(kWidth * kHeight * 3, 65535),
                      /*version=*/2));
  Send(client,
       Encode(2, std::vector<uint16_t>(kWidth * kHeight * 3 + 3, 65535)));
  Send(client, std::vector<uint8_t>(8, 0xff));
  f.sink.Refresh(f.t0 + milliseconds{10});
  CHECK(Shows(f.display, 50));
  CHECK(f.observed.size() == 1);
  // An oversized message is dropped whole, not read as a truncated frame.
  std::vector<uint8_t> oversized = Solid(3, 99);
  oversized.resize(oversized.size() + 100);
  Send(client, oversized);
  f.sink.Refresh(f.t0 + milliseconds{20});
  CHECK(Shows(f.display, 50));
  // The connection survives them.
  Send(client, Solid(4, 60));
  f.sink.Refresh(f.t0 + milliseconds{30});
  CHECK(Shows(f.display, 60));
  close(client);
}

void TestSinkFadesOnDisconnect() {
  sink::Config config;
  Fixture f;
  const int client = Connect(f.address);
  Send(client, Solid(0, 200));
  f.sink.Refresh(f.t0);
  close(client);
  const auto gone = f.t0 + milliseconds{100};
  f.sink.Refresh(gone);
  CHECK(Shows(f.display, 200));
  CHECK(!f.sink.Idle());

  // Halfway through the fade, about half as bright.
  f.sink.Refresh(gone + config.fade / 2);
  const double mean = MeanShown(f.display);
  CHECK(mean > 99 && mean < 101);

  f.sink.Refresh(gone + config.fade);
  CHECK(Shows(f.display, 0));
  CHECK(f.sink.Idle());
}

void TestSinkFadesWhenClientStopsSending() {
  sink::Config config;
  Fixture f;
  const int client = Connect(f.address);
  Send(client, Solid(0, 200));
  f.sink.Refresh(f.t0);
  f.sink.Refresh(f.t0 + config.timeout);
  CHECK(Shows(f.display, 200));
  f.sink.Refresh(f.t0 + config.timeout + config.fade / 4);
  const double mean = MeanShown(f.display);
  CHECK(mean > 149 && mean < 151);

  // A new frame ends the fade.
  Send(client, Solid(1, 120));
  f.sink.Refresh(f.t0 + config.timeout + config.fade / 2);
  CHECK(Shows(f.display, 120));

  f.sink.Refresh(f.t0 + 3 * config.timeout + config.fade);
  CHECK(Shows(f.display, 0));
  CHECK(f.sink.Idle());
  close(client);
}

void TestSinkNewClientReplacesOld() {
  Fixture f;
  const int first = Connect(f.address);
  Send(first, Solid(0, 30));
  f.sink.Refresh(f.t0);
  const int second = Connect(f.address);
  f.sink.Refresh(f.t0 + milliseconds{10});

  // The sink closed the first connection.
  uint8_t byte;
  CHECK(recv(first, &byte, 1, MSG_DONTWAIT) == 0);

  // The old frame stays up until the new client sends one.
  CHECK(Shows(f.display, 30));
  Send(second, Solid(0, 40));
  f.sink.Refresh(f.t0 + milliseconds{20});
  CHECK(Shows(f.display, 40));
  close(first);
  close(second);
}

} // namespace

int main() {
  TestParseGoodFrame();
  TestParseShortFrame();
  TestParseBadMagic();
  TestParseWrongVersion();
  TestParseWrongSize();
  TestDitherAverages();
  TestDitherExactValues();
  TestDitherPhasesDiffer();
  TestDitherGain();
  TestSinkStartsBlank();
  TestSinkShowsLatestFrame();
  TestSinkDropsBadMessages();
  TestSinkFadesOnDisconnect();
  TestSinkFadesWhenClientStopsSending();
  TestSinkNewClientReplacesOld();
  if (failures > 0) {
    std::fprintf(stderr, "%d check(s) failed\n", failures);
    return 1;
  }
  std::fprintf(stderr, "all sink tests passed\n");
  return 0;
}
