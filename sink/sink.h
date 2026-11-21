// The sink's refresh loop: take frames from one client, dither the latest onto
// the display at every refresh, and fade to black when the client goes away.

#ifndef MATRIX_DISPLAY_SINK_SINK_H
#define MATRIX_DISPLAY_SINK_SINK_H

#include <chrono>
#include <csignal>
#include <cstddef>
#include <cstdint>
#include <functional>
#include <vector>

#include "display.h"
#include "dither.h"

namespace sink {

struct Config {
  // How long the client may send nothing before the fade starts.
  std::chrono::milliseconds timeout{3000};
  // How long the fade to black takes.
  std::chrono::milliseconds fade{2000};
};

class Sink {
public:
  using Clock = std::chrono::steady_clock;
  // Sees every well-formed message, header included, as it arrives.
  using FrameObserver = std::function<void(const uint8_t *message, size_t)>;

  // Takes ownership of `listen_fd`, a listening, non-blocking unix
  // SOCK_SEQPACKET socket. `display` must outlive the sink.
  Sink(int listen_fd, Display *display, uint64_t seed, Config config = {});
  ~Sink();
  Sink(const Sink &) = delete;
  Sink &operator=(const Sink &) = delete;

  void set_on_frame(FrameObserver on_frame) { on_frame_ = std::move(on_frame); }

  // Refreshes until *stop is set (from a signal handler).
  void Run(const volatile std::sig_atomic_t *stop);

  // One step of Run: accepts any new client, reads whatever it sent, and
  // shows the latest frame, dithered and faded as of `now`. Blocks in
  // Display::Show unless Idle() was already true and nothing arrived.
  void Refresh(Clock::time_point now);

  // True when the panels are black and stay black until a client sends a
  // frame, so there's no need to refresh.
  bool Idle() const { return !has_frame_ && blank_shown_; }

  // Blocks until a client connects or sends, or `timeout` passes, or a signal
  // arrives.
  void WaitForInput(std::chrono::milliseconds timeout) const;

private:
  void Accept();
  void Read(Clock::time_point now);
  void Disconnect(Clock::time_point now, const char *why);
  void LogDrop(Clock::time_point now, const char *why, size_t length);
  uint32_t Gain(Clock::time_point now) const;

  const int listen_fd_;
  int client_fd_ = -1;
  Display *const display_;
  const Config config_;
  FrameObserver on_frame_;

  // One byte longer than a valid message, so a longer one shows up as
  // oversized rather than as truncated to the right size.
  std::vector<uint8_t> received_;
  std::vector<uint8_t> latest_;
  std::vector<uint16_t> frame_;
  std::vector<uint8_t> out_;
  Ditherer ditherer_;

  bool has_frame_ = false;
  bool blank_shown_ = false;
  // When the fade to black begins (or began).
  Clock::time_point fade_start_;

  // The sequence number the client should send next, once it has sent one.
  bool sequence_known_ = false;
  uint64_t next_sequence_ = 0;

  // Bad messages are logged at most every few seconds, with a count.
  size_t dropped_ = 0;
  Clock::time_point next_drop_log_;
};

} // namespace sink

#endif
