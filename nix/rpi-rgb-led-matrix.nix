# hzeller/rpi-rgb-led-matrix, the library only, for this repo's own builds and
# CI. Consumers never get this: overlays.default takes the library from the
# pkgs it is applied to, so rpi01 links against the one dotfiles pins
# (packages/rpi-rgb-led-matrix there, which also builds the demos and tools).
#
# Keep `rev` near dotfiles' pin so CI compiles against the API rpi01 has.
{
  lib,
  stdenv,
  fetchFromGitHub,
}:

stdenv.mkDerivation {
  pname = "rpi-rgb-led-matrix";
  version = "0-unstable-2026-11-17";

  src = fetchFromGitHub {
    owner = "hzeller";
    repo = "rpi-rgb-led-matrix";
    rev = "a8dd6854580b7e2b18d0f697cec94415dd8824cd";
    hash = "sha256-nkmvOTcYRRlVjeLr4+hPNxqckrFYBE33pcbLuLehDN8=";
  };

  makeFlags = [
    # config.mk adds -march=native unless the compiler is named
    # aarch64-linux-gnu-*, which would tune for the builder.
    "CPU_ARCH_FLAGS="
    # The library builds with -flto, so it needs the LTO-aware archiver, and
    # the bare `gcc-ar` is not on PATH under a cross compiler.
    "AR=${stdenv.cc.targetPrefix}gcc-ar"
    "PREFIX=${placeholder "out"}"
  ];

  enableParallelBuilding = true;

  buildPhase = ''
    runHook preBuild
    make -C lib -j$NIX_BUILD_CORES $makeFlags
    runHook postBuild
  '';

  installPhase = ''
    runHook preInstall
    make -C lib $makeFlags install
    runHook postInstall
  '';

  meta = {
    description = "Library for driving HUB75 RGB LED matrix panels from a Raspberry Pi";
    homepage = "https://github.com/hzeller/rpi-rgb-led-matrix";
    license = lib.licenses.gpl2Only;
    platforms = lib.platforms.linux;
  };
}
