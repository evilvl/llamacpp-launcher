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
            version = "0.1.0";

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
              pkgs.curl
              pkgs.python3
            ];

            text = ''
              set -uo pipefail
              export CGO_ENABLED=0

              go vet ./...
              go build -o llamacpp-launcher ./cmd/llamacpp-launcher

              echo BUILD_OK

              D=$(mktemp -d)

              cleanup() {
                if [ -n "''${P:-}" ]; then
                  kill "$P" 2>/dev/null || true
                  wait "$P" 2>/dev/null || true
                fi

                rm -rf "$D"
              }

              trap cleanup EXIT

              mkdir -p "$D/models"

              printf 'GGUF' > "$D/models/tiny.gguf"

              export LLAMA_MODEL_ROOT="$D/models"
              export LLAMA_CONFIG_DIR="$D/configs"
              export LLAMA_WEB_HOST=127.0.0.1
              export LLAMA_WEB_PORT=18079

              ./llamacpp-launcher >/tmp/llama_smoke.log 2>&1 &
              P=$!

              sleep 1

              echo "=== save (flags map + extra) ==="

              body='{"model":"'"$D"'/models/tiny.gguf","flags":{"--port":"8090","--api-key":"abc","--ctx-size":"4096"},"extra":"--verbose --n-batch 512"}'

              curl -s \
                -XPOST \
                localhost:18079/api/config \
                -H 'Content-Type: application/json' \
                -d "$body"

              echo

              echo "=== config after save (GET) ==="

              curl -s \
                "localhost:18079/api/config?model=$D/models/tiny.gguf" |
                python3 -m json.tool

              echo "=== file on disk ==="

              cat "$D/configs/tiny.conf"

              echo "=== server log ==="

              cat /tmp/llama_smoke.log

              echo "smoke done"
            '';
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

      formatter = forAllSystems (system: (perSystem system).formatter);
    };
}
