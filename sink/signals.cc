#include "signals.h"

namespace sink {
namespace {

volatile std::sig_atomic_t stop = 0;

void OnSignal(int) { stop = 1; }

} // namespace

const volatile std::sig_atomic_t *StopOnSignals() {
  struct sigaction action = {};
  action.sa_handler = OnSignal;
  sigaction(SIGTERM, &action, nullptr);
  sigaction(SIGINT, &action, nullptr);
  return &stop;
}

} // namespace sink
