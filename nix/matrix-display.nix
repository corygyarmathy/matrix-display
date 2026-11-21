# The renderer. Unprivileged; everything except putting frames on the panels.
{
  lib,
  buildGoModule,
  ffmpeg-headless,
}:

let
  root = ../.;
in
buildGoModule {
  pname = "matrix-display";
  version = "0-unstable";

  src = lib.fileset.toSource {
    inherit root;
    fileset = lib.fileset.unions [
      (root + "/go.mod")
      (lib.fileset.maybeMissing (root + "/go.sum"))
      (root + "/cmd")
      (lib.fileset.maybeMissing (root + "/internal"))
    ];
  };

  # No dependencies yet. Set to a hash once go.sum exists.
  vendorHash = null;

  # No subPackages: it would also limit the tests to those packages. Only
  # cmd/matrix-display is a main package, so it is still the only binary.

  env.CGO_ENABLED = 0;

  # For the preview's video tests. At run time, the preview finds ffmpeg on
  # the PATH, so it stays out of the closure the Pi installs; the flake's
  # default app adds it.
  nativeCheckInputs = [ ffmpeg-headless ];

  # The tests are a correctness gate for every build of the package, including
  # a consumer's aarch64 build under QEMU. A wall-clock bound is not
  # correctness, and emulation misses it, so -short skips the preview's
  # timing test here. The flake's `preview-speed` check runs it natively.
  checkFlags = [ "-short" ];

  meta = {
    description = "Renders an ambient display for a 256x64 LED strip";
    homepage = "https://github.com/corygyarmathy/matrix-display";
    license = lib.licenses.gpl2Only;
    platforms = lib.platforms.linux;
    mainProgram = "matrix-display";
  };
}
