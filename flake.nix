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

      # The library revision this repo fetches for its own builds and CI. Never
      # part of overlays.default.
      ciLibrary = final: _prev: {
        rpi-rgb-led-matrix = final.callPackage ./nix/rpi-rgb-led-matrix.nix { };
      };

      pkgsFor =
        system:
        import nixpkgs {
          inherit system;
          overlays = [
            ciLibrary
            self.overlays.default
          ];
        };

      # aarch64 built on x86_64, for CI: seconds rather than minutes under QEMU.
      pkgsCrossFor =
        system:
        import nixpkgs {
          localSystem = system;
          crossSystem = "aarch64-linux";
          overlays = [
            ciLibrary
            self.overlays.default
          ];
        };
    in
    {
      # What dotfiles consumes. The sink takes `rpi-rgb-led-matrix` from the
      # pkgs this is applied to, so it links against the consumer's pinned
      # library, and fails to evaluate on a pkgs without one.
      overlays.default = final: _prev: {
        matrix-display = final.callPackage ./nix/matrix-display.nix { };
        matrix-display-sink = final.callPackage ./nix/matrix-display-sink.nix { };
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
            inputsFrom = [
              pkgs.matrix-display
              pkgs.matrix-display-sink
            ];
            packages = with pkgs; [
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
