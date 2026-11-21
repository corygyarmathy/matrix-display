#include "activation.h"

#include <cerrno>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <fcntl.h>
#include <sys/socket.h>
#include <unistd.h>

namespace sink {
namespace {

// SD_LISTEN_FDS_START.
constexpr int kFirstFd = 3;

bool ParseLong(const char *s, long *out) {
  if (s == nullptr || *s == '\0') {
    return false;
  }
  char *end;
  errno = 0;
  *out = std::strtol(s, &end, 10);
  return errno == 0 && *end == '\0';
}

bool SocketOption(int fd, int option, int *value) {
  socklen_t length = sizeof *value;
  return getsockopt(fd, SOL_SOCKET, option, value, &length) == 0;
}

} // namespace

int ListenFd() {
  long pid, fds;
  const bool for_us = ParseLong(std::getenv("LISTEN_PID"), &pid) &&
                      pid == static_cast<long>(getpid());
  const bool counted = ParseLong(std::getenv("LISTEN_FDS"), &fds);
  unsetenv("LISTEN_PID");
  unsetenv("LISTEN_FDS");
  unsetenv("LISTEN_FDNAMES");

  if (!for_us || !counted) {
    std::fprintf(stderr, "no socket from systemd: run this from a socket "
                         "unit with ListenSequentialPacket=\n");
    return -1;
  }
  if (fds != 1) {
    std::fprintf(stderr, "systemd passed %ld sockets, want 1\n", fds);
    return -1;
  }

  const int fd = kFirstFd;
  int domain, type, listening;
  if (!SocketOption(fd, SO_DOMAIN, &domain) ||
      !SocketOption(fd, SO_TYPE, &type) ||
      !SocketOption(fd, SO_ACCEPTCONN, &listening)) {
    std::fprintf(stderr, "fd %d from systemd is not a socket: %s\n", fd,
                 std::strerror(errno));
    return -1;
  }
  if (domain != AF_UNIX || type != SOCK_SEQPACKET || !listening) {
    std::fprintf(stderr,
                 "fd %d from systemd is not a listening unix "
                 "SOCK_SEQPACKET socket\n",
                 fd);
    return -1;
  }

  const int flags = fcntl(fd, F_GETFL);
  if (fcntl(fd, F_SETFD, FD_CLOEXEC) != 0 ||
      fcntl(fd, F_SETFL, flags | O_NONBLOCK) != 0) {
    std::fprintf(stderr, "fd %d from systemd: %s\n", fd, std::strerror(errno));
    return -1;
  }
  return fd;
}

} // namespace sink
