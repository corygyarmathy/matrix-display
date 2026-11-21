// matrix-display-sink-fake: the sink with a fake display instead of the
// panels, for testing the renderer's writer against the real protocol code.
// It takes its socket from systemd-style activation as the real sink does,
// and writes every frame it accepts, header included, to stdout.
//
//   matrix-display-sink-fake --width=W --height=H

#include <csignal>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <unistd.h>

#include "activation.h"
#include "fake_display.h"
#include "sink.h"

namespace {

volatile std::sig_atomic_t stop = 0;

void OnSignal(int) { stop = 1; }

bool ParseDimension(const char *arg, const char *name, int *out) {
  const size_t n = std::strlen(name);
  if (std::strncmp(arg, name, n) != 0 || arg[n] != '=') {
    return false;
  }
  *out = std::atoi(arg + n + 1);
  return *out > 0;
}

void WriteAll(const uint8_t *data, size_t length) {
  while (length > 0) {
    const ssize_t n = write(STDOUT_FILENO, data, length);
    if (n < 0) {
      std::perror("write");
      std::exit(1);
    }
    data += n;
    length -= static_cast<size_t>(n);
  }
}

} // namespace

int main(int argc, char *argv[]) {
  int width = 0, height = 0;
  for (int i = 1; i < argc; ++i) {
    if (!ParseDimension(argv[i], "--width", &width) &&
        !ParseDimension(argv[i], "--height", &height)) {
      std::fprintf(stderr, "usage: %s --width=W --height=H\n", argv[0]);
      return 2;
    }
  }
  if (width == 0 || height == 0) {
    std::fprintf(stderr, "usage: %s --width=W --height=H\n", argv[0]);
    return 2;
  }

  const int fd = sink::ListenFd();
  if (fd < 0) {
    return 1;
  }

  struct sigaction action = {};
  action.sa_handler = OnSignal;
  sigaction(SIGTERM, &action, nullptr);
  sigaction(SIGINT, &action, nullptr);

  // About the panels' refresh rate.
  sink::FakeDisplay display(width, height, std::chrono::milliseconds{10});
  sink::Sink sink(fd, &display, /*seed=*/1);
  sink.set_on_frame(WriteAll);
  sink.Run(&stop);
  return 0;
}
