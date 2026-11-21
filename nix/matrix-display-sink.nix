# The sink: the only process that touches the panels. `rpi-rgb-led-matrix` is
# whatever the caller's pkgs provides - see overlays.default in flake.nix.
{
  lib,
  stdenv,
  pkg-config,
  rpi-rgb-led-matrix,
}:

stdenv.mkDerivation {
  pname = "matrix-display-sink";
  version = "0-unstable";

  src = lib.fileset.toSource {
    root = ../sink;
    fileset = lib.fileset.unions [
      ../sink/Makefile
      (lib.fileset.fileFilter (f: f.hasExt "cc" || f.hasExt "h") ../sink)
    ];
  };

  nativeBuildInputs = [ pkg-config ];
  buildInputs = [ rpi-rgb-led-matrix ];

  makeFlags = [ "PREFIX=${placeholder "out"}" ];
  enableParallelBuilding = true;

  meta = {
    description = "Puts frames from matrix-display on HUB75 LED panels";
    homepage = "https://github.com/corygyarmathy/matrix-display";
    # Links rpi-rgb-led-matrix, which is GPL-2.0-only.
    license = lib.licenses.gpl2Only;
    platforms = lib.platforms.linux;
    mainProgram = "matrix-display-sink";
  };
}
