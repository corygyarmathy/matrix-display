{
  description = "An ambient display for a 256x64 HUB75 LED strip";

  inputs = {
    # The channel rpi01 runs (dotfiles' nixpkgs-stable), so CI builds with the
    # same toolchain the overlay will get there.
    nixpkgs.url = "github:nixos/nixpkgs/nixos-26.05";
  };

  outputs =
    { self, nixpkgs }:
    let
      inherit (nixpkgs) lib;

      systems = [
        "x86_64-linux"
        "aarch64-linux"
      ];
      forAllSystems = f: lib.genAttrs systems f;

      # This repo's own builds treat the sink's warnings as errors. Consumers
      # don't: a newer compiler on their side must not break their rebuild.
      strictSink = _final: prev: {
        matrix-display-sink = prev.matrix-display-sink.overrideAttrs (old: {
          makeFlags = old.makeFlags ++ [ "WERROR=-Werror" ];
        });
      };

      pkgsFor =
        system:
        import nixpkgs {
          inherit system;
          overlays = [
            self.overlays.default
            strictSink
          ];
        };

      # aarch64 built on x86_64, for CI: seconds rather than minutes under QEMU.
      # pkgsCross keeps pkgsFor's overlays.
      pkgsCrossFor = system: (pkgsFor system).pkgsCross.aarch64-multiplatform;
    in
    {
      # What dotfiles consumes. The sink links the `rpi-rgb-led-matrix` of the
      # pkgs this is applied to, so on rpi01 it uses dotfiles' pinned library.
      # A pkgs without one (nixpkgs has none) gets ./nix/rpi-rgb-led-matrix.nix,
      # which is also what this repo's own builds and CI use.
      overlays.default = final: _prev: {
        matrix-display = final.callPackage ./nix/matrix-display.nix { };
        matrix-display-sink = final.callPackage ./nix/matrix-display-sink.nix {
          rpi-rgb-led-matrix =
            final.rpi-rgb-led-matrix or (final.callPackage ./nix/rpi-rgb-led-matrix.nix { });
        };
      };

      packages = forAllSystems (
        system:
        let
          pkgs = pkgsFor system;
        in
        {
          inherit (pkgs) matrix-display matrix-display-sink;
          default = pkgs.matrix-display;
        }
        // lib.optionalAttrs (system == "x86_64-linux") {
          # The renderer is pure Go, so the cached native toolchain builds it
          # for arm64 with GOARCH alone. Going through pkgsCross instead builds
          # a cross Go from source, which cache.nixos.org does not hold: about
          # four minutes on every CI run. Its tests run in the native build.
          matrix-display-cross = pkgs.matrix-display.overrideAttrs (old: {
            env = old.env // {
              GOARCH = "arm64";
            };
            doCheck = false;
            # buildGoModule only does this for a pkgsCross build.
            postBuild = ''
              mv $GOPATH/bin/linux_arm64/* $GOPATH/bin/
              rmdir $GOPATH/bin/linux_arm64
            '';
          });
          # The sink needs a real cross C++ toolchain, which is cached.
          matrix-display-sink-cross = (pkgsCrossFor system).matrix-display-sink;
        }
      );

      # `nix run . -- preview …`: the renderer with ffmpeg on the PATH, for
      # the preview's videos.
      apps = forAllSystems (
        system:
        let
          pkgs = pkgsFor system;
          withFfmpeg = pkgs.writeShellApplication {
            name = "matrix-display";
            runtimeInputs = [ pkgs.ffmpeg-headless ];
            text = ''
              exec ${lib.getExe pkgs.matrix-display} "$@"
            '';
          };
        in
        {
          default = {
            type = "app";
            program = lib.getExe withFfmpeg;
            meta.description = "The renderer, with ffmpeg for the preview";
          };
        }
      );

      checks = forAllSystems (
        system:
        let
          pkgs = pkgsFor system;

          # A stand-in for the library a consumer like dotfiles provides. It
          # is only compared, never built.
          consumerLibrary = pkgs.runCommand "consumer-rpi-rgb-led-matrix" { } "mkdir $out";
          consumerPkgs = import nixpkgs {
            inherit system;
            overlays = [
              (_final: _prev: { rpi-rgb-led-matrix = consumerLibrary; })
              self.overlays.default
            ];
          };
          sinkInputs = map (d: d.outPath) consumerPkgs.matrix-display-sink.buildInputs;

          # Stock nixpkgs, which has no rpi-rgb-led-matrix of its own.
          fallbackPkgs = import nixpkgs {
            inherit system;
            overlays = [ self.overlays.default ];
          };
          fetchedLibrary = fallbackPkgs.callPackage ./nix/rpi-rgb-led-matrix.nix { };
          fallbackSinkInputs = map (d: d.outPath) fallbackPkgs.matrix-display-sink.buildInputs;

          fakeSink = pkgs.matrix-display-sink.overrideAttrs {
            pname = "matrix-display-sink-fake";
            buildFlags = [ "matrix-display-sink-fake" ];
            installTargets = [ "install-fake" ];
            doCheck = false;
          };
        in
        # Every package builds (and the renderer's tests run in its build).
        builtins.removeAttrs self.packages.${system} [ "default" ]
        // {
          overlay-uses-consumer-library =
            assert lib.assertMsg (
              consumerPkgs.rpi-rgb-led-matrix.outPath == consumerLibrary.outPath
            ) "overlays.default replaced the consumer's rpi-rgb-led-matrix";
            assert lib.assertMsg (builtins.elem consumerLibrary.outPath sinkInputs)
              "matrix-display-sink does not link the consumer's rpi-rgb-led-matrix";
            pkgs.runCommand "overlay-uses-consumer-library" { } "touch $out";

          overlay-falls-back-to-fetched-library =
            assert lib.assertMsg (
              !(fallbackPkgs ? rpi-rgb-led-matrix)
            ) "nixpkgs now has rpi-rgb-led-matrix, so this check no longer tests the fallback";
            assert lib.assertMsg (builtins.elem fetchedLibrary.outPath fallbackSinkInputs)
              "matrix-display-sink does not fall back to ./nix/rpi-rgb-led-matrix.nix";
            pkgs.runCommand "overlay-falls-back-to-fetched-library" { } "touch $out";

          # The renderer's writer against the sink's own code, built with a
          # fake display: frames sent by internal/sinkproto arrive unchanged.
          sink-roundtrip = pkgs.matrix-display.overrideAttrs (old: {
            pname = "sink-roundtrip";
            env = old.env // {
              MATRIX_DISPLAY_FAKE_SINK = lib.getExe' fakeSink "matrix-display-sink-fake";
            };
          });

          formatting =
            pkgs.runCommand "formatting"
              {
                nativeBuildInputs = [ self.formatter.${system} ];
              }
              ''
                cp -r ${self} src
                chmod -R u+w src
                cd src
                export HOME=$TMPDIR
                formatter --ci --tree-root .
                touch $out
              '';
        }
      );

      # `nix fmt` formats the tree; `nix fmt -- --ci` fails on anything it
      # would change. What runs on each file type is ./treefmt.toml; this only
      # supplies the binaries, pinned by flake.lock.
      formatter = forAllSystems (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        pkgs.writeShellApplication {
          name = "formatter";
          runtimeInputs = with pkgs; [
            clang-tools # clang-format
            gofumpt
            gotools # goimports
            nixfmt
            prettier
            treefmt
          ];
          text = ''
            exec treefmt "$@"
          '';
        }
      );

      devShells = forAllSystems (
        system:
        let
          pkgs = pkgsFor system;
        in
        {
          default = pkgs.mkShell {
            # `make -C sink` fails on warnings here, as CI does.
            WERROR = "-Werror";
            inputsFrom = [
              pkgs.matrix-display
              pkgs.matrix-display-sink
            ];
            packages = with pkgs; [
              ffmpeg-headless
              gopls
              gotools
              clang-tools
              self.formatter.${system}
            ];
          };
        }
      );
    };
}
