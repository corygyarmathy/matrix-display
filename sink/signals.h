// Stopping the sink on SIGTERM or SIGINT, for both mains.

#ifndef MATRIX_DISPLAY_SINK_SIGNALS_H
#define MATRIX_DISPLAY_SINK_SIGNALS_H

#include <csignal>

namespace sink {

// Installs handlers for SIGTERM and SIGINT, and returns the flag they set,
// for Sink::Run.
const volatile std::sig_atomic_t *StopOnSignals();

} // namespace sink

#endif
