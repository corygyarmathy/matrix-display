# Builds matrix-display-sink against rpi-rgb-led-matrix, found through
# pkg-config. CXX and PKG_CONFIG come from the environment, so a cross stdenv's
# prefixed tools are used when Nix sets them.
#
#   make                          the sink
#   make check                    the unit tests
#   make matrix-display-sink-fake the sink with a fake display, for the Go
#                                 round-trip test (needs no library)

PKG_CONFIG ?= pkg-config
PREFIX ?= /usr/local

CXXFLAGS ?= -O2
# Off by default, so a newer compiler's warnings never break a consumer's
# build; this repo's own builds, CI and dev shell set WERROR=-Werror.
WERROR ?=
CXXFLAGS += -std=c++17 -Wall -Wextra $(WERROR)
# The library's headers are included as system headers: they trip -Wextra
# (unused parameters), and -Werror is for this code, not upstream's. Only
# main.o includes them; nothing else needs the library.
MATRIX_CFLAGS = $(patsubst -I%,-isystem %,$(shell $(PKG_CONFIG) --cflags rgbmatrix))
MATRIX_LIBS = $(shell $(PKG_CONFIG) --libs rgbmatrix)

CORE = activation.o dither.o protocol.o signals.o sink.o

matrix-display-sink: main.o $(CORE)
	$(CXX) $(LDFLAGS) -o $@ $^ $(MATRIX_LIBS) $(LDLIBS)

main.o: main.cc
	$(CXX) $(CPPFLAGS) $(CXXFLAGS) $(MATRIX_CFLAGS) -c -o $@ $<

matrix-display-sink-fake: fake_main.o $(CORE)
	$(CXX) $(LDFLAGS) -o $@ $^ $(LDLIBS)

sink-test: test.o $(CORE)
	$(CXX) $(LDFLAGS) -o $@ $^ $(LDLIBS)

check: sink-test
	./sink-test

install: matrix-display-sink
	install -D -m 755 matrix-display-sink $(DESTDIR)$(PREFIX)/bin/matrix-display-sink

install-fake: matrix-display-sink-fake
	install -D -m 755 matrix-display-sink-fake $(DESTDIR)$(PREFIX)/bin/matrix-display-sink-fake

clean:
	rm -f matrix-display-sink matrix-display-sink-fake sink-test *.o *.d

# Header dependencies, so editing a header rebuilds what includes it.
CXXFLAGS += -MMD -MP
-include $(wildcard *.d)

.PHONY: check install install-fake clean
