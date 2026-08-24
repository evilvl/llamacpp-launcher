{
  description = "llamacpp-launcher — zero-dependency Go web UI to manage a llama.cpp (llama-server) instance via systemd";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs =
    { self, nixpkgs }:
    let
      lib = nixpkgs.lib;

      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];

      forAllSystems = lib.genAttrs systems;

      perSystem =
        system:
        let
          pkgs = import nixpkgs {
            inherit system;
          };

          src = self;

          pkg = pkgs.stdenv.mkDerivation {
            pname = "llamacpp-launcher";
            version = "0.2.0";

            inherit src;

            nativeBuildInputs = [
              pkgs.go
            ];

            buildPhase = ''
              export CGO_ENABLED=0
              export HOME=$TMPDIR
              export GOCACHE=$TMPDIR/go-build

              go build -o dist/llamacpp-launcher ./cmd/llamacpp-launcher
            '';

            installPhase = ''
              install -Dm0755 \
                dist/llamacpp-launcher \
                $out/bin/llamacpp-launcher
            '';
          };

          smoke = pkgs.writeShellApplication {
            name = "smoke";

            runtimeInputs = [
              pkgs.go
            ];

            text = ''
              set -uo pipefail
              export CGO_ENABLED=0

              go vet ./...
              go build -o llamacpp-launcher ./cmd/llamacpp-launcher

              echo BUILD_OK

              go test -tags=integration ./test/e2e/...

              echo SMOKE_OK
            '';
          };

          apps = {
             run = {
               type = "app";

               program = "${pkg}/bin/llamacpp-launcher";
               meta.description = "Build and run llamacpp-launcher for local testing";
             };
          };

          checks = {
            go-vet-and-tests =
              pkgs.runCommand "llamacpp-launcher-go-tests"
                {
                  nativeBuildInputs = [
                    pkgs.go
                  ];
                }
                ''
                  export CGO_ENABLED=0
                  export HOME=$TMPDIR

                  cd ${src}

                  go vet ./...
                  go test ./...

                  touch $out
                '';

            ui-i18n =
              pkgs.runCommand "llamacpp-launcher-ui-tests"
                {
                  nativeBuildInputs = [
                    pkgs.nodejs
                  ];
                }
                ''
                  cd ${src}

                  node tests/ui_i18n.test.mjs

                  touch $out
                '';

            gofmt =
              pkgs.runCommand "llamacpp-launcher-gofmt"
                {
                  nativeBuildInputs = [
                    pkgs.go
                  ];
                }
                ''
                  cd ${src}

                  unformatted=$(gofmt -l .)

                  if [ -n "$unformatted" ]; then
                    echo "files requiring gofmt:"
                    echo "$unformatted"
                    exit 1
                  fi

                  touch $out
                '';
          };
        in
        {
          packages.default = pkg;
          packages.smoke = smoke;

          inherit apps;
          inherit checks;

          devShells.default = pkgs.mkShell {
            packages = [
              pkgs.go
              pkgs.nodejs
              pkgs.git
              smoke
            ];

            shellHook = ''
              export CGO_ENABLED=0
            '';
          };

          formatter = pkgs.nixfmt;
        };
    in
    {
      packages = forAllSystems (system: (perSystem system).packages);

      checks = forAllSystems (system: (perSystem system).checks);

      devShells = forAllSystems (system: (perSystem system).devShells);

      apps = forAllSystems (system: (perSystem system).apps);

      formatter = forAllSystems (system: (perSystem system).formatter);
    };
}
