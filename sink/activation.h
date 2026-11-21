// Receiving the listening socket from systemd.

#ifndef MATRIX_DISPLAY_SINK_ACTIVATION_H
#define MATRIX_DISPLAY_SINK_ACTIVATION_H

namespace sink {

// Returns the one socket systemd passed (LISTEN_FDS=1 for this process), after
// checking that it is a listening unix SOCK_SEQPACKET socket, or -1 after
// writing why to stderr. Unsets the LISTEN_* variables.
//
// This is sd_listen_fds(3), written out so the sink links nothing but the
// panel library.
int ListenFd();

} // namespace sink

#endif
