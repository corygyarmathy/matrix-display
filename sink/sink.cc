#include "sink.h"

#include <algorithm>
#include <cerrno>
#include <cinttypes>
#include <cstdio>
#include <cstring>
#include <poll.h>
#include <sys/socket.h>
#include <unistd.h>

#include "protocol.h"

namespace sink {
namespace {

constexpr std::chrono::seconds kDropLogInterval{5};
constexpr std::chrono::seconds kIdleWait{1};

} // namespace

Sink::Sink(int listen_fd, Display *display, uint64_t seed, Config config)
    : listen_fd_(listen_fd), display_(display), config_(config),
      received_(MessageSize(display->width(), display->height()) + 1),
      latest_(received_.size()),
      frame_(static_cast<size_t>(display->width()) * display->height() * 3),
      out_(frame_.size()), ditherer_(frame_.size(), seed) {}

Sink::~Sink() {
  if (client_fd_ >= 0) {
    close(client_fd_);
  }
  close(listen_fd_);
}

void Sink::Run(const volatile std::sig_atomic_t *stop) {
  while (!*stop) {
    if (Idle()) {
      WaitForInput(kIdleWait);
    }
    Refresh(Clock::now());
  }
}

void Sink::Refresh(Clock::time_point now) {
  Accept();
  Read(now);
  LogDrops(now);

  const uint32_t gain = has_frame_ ? Gain(now) : 0;
  if (gain == 0) {
    has_frame_ = false;
    if (!blank_shown_) {
      std::fill(out_.begin(), out_.end(), 0);
      display_->Show(out_.data());
      blank_shown_ = true;
    }
    return;
  }
  blank_shown_ = false;
  ditherer_.Step(frame_.data(), gain, out_.data());
  display_->Show(out_.data());
}

void Sink::WaitForInput(std::chrono::milliseconds timeout) const {
  pollfd fds[2] = {{listen_fd_, POLLIN, 0}, {client_fd_, POLLIN, 0}};
  // poll ignores a negative fd, so this also works with no client.
  poll(fds, 2, static_cast<int>(timeout.count()));
}

void Sink::Accept() {
  for (;;) {
    const int fd =
        accept4(listen_fd_, nullptr, nullptr, SOCK_NONBLOCK | SOCK_CLOEXEC);
    if (fd < 0) {
      if (errno == EINTR || errno == ECONNABORTED) {
        continue;
      }
      if (errno != EAGAIN && errno != EWOULDBLOCK) {
        std::fprintf(stderr, "accept: %s\n", std::strerror(errno));
      }
      return;
    }
    // Tell the client the panels' size before anything else. The connection
    // is new, so its buffer has room and this doesn't block. If the client has
    // already gone, this fails, but whatever it sent is still read.
    uint8_t panel[kPanelMessageSize];
    EncodePanel(display_->width(), display_->height(), panel);
    if (send(fd, panel, sizeof panel, MSG_DONTWAIT | MSG_NOSIGNAL) !=
        static_cast<ssize_t>(sizeof panel)) {
      std::fprintf(stderr, "sending the panel size: %s\n",
                   std::strerror(errno));
    }
    if (client_fd_ >= 0) {
      std::fprintf(stderr, "a new client replaces the current one\n");
      close(client_fd_);
    } else {
      std::fprintf(stderr, "client connected\n");
    }
    client_fd_ = fd;
    sequence_known_ = false;
  }
}

void Sink::Read(Clock::time_point now) {
  if (client_fd_ < 0) {
    return;
  }
  const int width = display_->width();
  const int height = display_->height();
  bool got_frame = false;
  bool disconnected = false;
  for (;;) {
    // MSG_TRUNC makes recv return the message's real length, even when it
    // didn't fit.
    const ssize_t n = recv(client_fd_, received_.data(), received_.size(),
                           MSG_DONTWAIT | MSG_TRUNC);
    if (n < 0) {
      if (errno == EINTR) {
        continue;
      }
      if (errno != EAGAIN && errno != EWOULDBLOCK) {
        Disconnect(std::strerror(errno));
        disconnected = true;
      }
      break;
    }
    if (n == 0) {
      // The end of the connection. (A zero-length message reads the same,
      // and isn't a frame either.)
      Disconnect("client disconnected");
      disconnected = true;
      break;
    }
    const size_t length = static_cast<size_t>(n);
    Header header;
    const ParseResult result =
        ParseFrame(received_.data(), length, width, height, &header);
    if (result != ParseResult::kOk) {
      ++dropped_;
      last_drop_why_ = Describe(result);
      last_drop_length_ = length;
      continue;
    }
    if (sequence_known_ && header.sequence != next_sequence_) {
      std::fprintf(stderr, "frame %" PRIu64 " arrived, expected %" PRIu64 "\n",
                   header.sequence, next_sequence_);
    }
    sequence_known_ = true;
    next_sequence_ = header.sequence + 1;
    if (on_frame_) {
      on_frame_(received_.data(), length);
    }
    std::swap(received_, latest_);
    got_frame = true;
  }

  if (got_frame) {
    DecodePixels(latest_.data() + kHeaderSize, frame_.size(), frame_.data());
    has_frame_ = true;
    fade_start_ = now + config_.timeout;
  }
  // After the frame, which may have been the client's last.
  if (disconnected) {
    fade_start_ = std::min(fade_start_, now);
  }
}

void Sink::Disconnect(const char *why) {
  std::fprintf(stderr, "%s\n", why);
  close(client_fd_);
  client_fd_ = -1;
}

void Sink::LogDrops(Clock::time_point now) {
  if (dropped_ == 0 || now < next_drop_log_) {
    return;
  }
  std::fprintf(stderr,
               "dropped %zu bad message(s); the latest was %s (%zu bytes)\n",
               dropped_, last_drop_why_, last_drop_length_);
  dropped_ = 0;
  next_drop_log_ = now + kDropLogInterval;
}

uint32_t Sink::Gain(Clock::time_point now) const {
  if (now <= fade_start_) {
    return kFullGain;
  }
  const auto elapsed = now - fade_start_;
  const auto fade = std::chrono::duration_cast<Clock::duration>(config_.fade);
  if (elapsed >= fade) {
    return 0;
  }
  // Linear in the pixel values, which are already perceptual.
  return static_cast<uint32_t>(kFullGain -
                               kFullGain * elapsed.count() / fade.count());
}

} // namespace sink
