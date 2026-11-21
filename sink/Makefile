# Builds matrix-display-sink against rpi-rgb-led-matrix, found through
# pkg-config. CXX and PKG_CONFIG come from the environment, so a cross stdenv's
# prefixed tools are used when Nix sets them.

PKG_CONFIG ?= pkg-config
PREFIX ?= /usr/local

CXXFLAGS ?= -O2
# Off by default, so a newer compiler's warnings never break a consumer's
# build; this repo's own builds, CI and dev shell set WERROR=-Werror.
WERROR ?=
# The library's headers are included as system headers: they trip -Wextra
# (unused parameters), and -Werror is for this code, not upstream's.
CXXFLAGS += -std=c++17 -Wall -Wextra $(WERROR) \
	$(patsubst -I%,-isystem %,$(shell $(PKG_CONFIG) --cflags rgbmatrix))
LDLIBS += $(shell $(PKG_CONFIG) --libs rgbmatrix)

matrix-display-sink: main.o
	$(CXX) $(LDFLAGS) -o $@ $^ $(LDLIBS)

install: matrix-display-sink
	install -D -m 755 matrix-display-sink $(DESTDIR)$(PREFIX)/bin/matrix-display-sink

clean:
	rm -f matrix-display-sink *.o

.PHONY: install clean
