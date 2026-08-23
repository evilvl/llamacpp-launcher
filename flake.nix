{
  description = "llama-cpp-webui — zero-dependency Go web UI to manage a llama.cpp (llama-server) instance via systemd";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable"; # go here is >= 1.26, matching go.mod's `go 1.26`
  };

  outputs = {
    self,
    nixpkgs,
    ...
  }: let
    # Single-target for now; this project is developed on x86_64-linux.
    # To add a target, change `system` below (or generalize to lib.systems).
    system = "x86_64-linux";
    pkgs = import nixpkgs {inherit system;};
    src = self;

    # End-to-end smoke test: build, run, exercise the save/get round-trip.
    # Needs a live llama-server binary + systemd, so it is NOT part of
    # `nix flake check`; run it via `nix develop .#default -c smoke`.
    smoke = pkgs.writeShellApplication {
      name = "smoke";
      runtimeInputs = [pkgs.go pkgs.curl pkgs.python3];
      text = ''
          #!/usr/bin/env bash
          # shellcheck disable=SC2086
          # End-to-end smoke test: build, run, exercise the save/get round-trip with the
        # current API payload (flags map), and verify it lands on disk.
        set -uo pipefail
        go vet ./... && go build -o llama-cpp-webui . && echo BUILD_OK
        D=$(mktemp -d)
        mkdir -p "$D/models"
        printf 'GGUF' > "$D/models/tiny.gguf"
        export LLAMA_MODEL_ROOT="$D/models"
        export LLAMA_CONFIG_DIR="$D/configs"
        export LLAMA_WEB_HOST=127.0.0.1
        export LLAMA_WEB_PORT=18079
        ./llama-cpp-webui >/tmp/llama_smoke.log 2>&1 &
        P=$!
        sleep 1
          echo "=== save (flags map + extra) ==="
          body='{"model":"'"$D"'/models/tiny.gguf","flags":{"--port":"8090","--api-key":"abc","--ctx-size":"4096"},"extra":"--verbose --n-batch 512"}'
          curl -s -XPOST localhost:18079/api/config -H 'Content-Type: application/json' \
            -d "$body"
        echo
        echo "=== config after save (GET) ==="
        curl -s "localhost:18079/api/config?model=$D/models/tiny.gguf" | python3 -m json.tool
        echo "=== file on disk ==="
        cat "$D/configs/tiny.conf"
        kill "$P" 2>/dev/null
        wait "$P" 2>/dev/null
        rm -rf "$D"
        echo "=== server log ==="
        cat /tmp/llama_smoke.log
        echo "smoke done"
      '';
    };
  in {
    packages.${system} = {
      # Default package: the static web-server binary (stdlib only, no cgo).
      # No third-party modules → no vendor dir, no go.sum needed.
      default = pkgs.stdenv.mkDerivation {
        pname = "llama-cpp-webui";
        version = "0.1.0";
        inherit src;
        nativeBuildInputs = [pkgs.go];
        buildPhase = ''
          export CGO_ENABLED=0 HOME=$TMPDIR GOCACHE=$TMPDIR/go-build
          go build -o dist/llama-cpp-webui .
        '';
        installPhase = ''
          install -Dm0755 dist/llama-cpp-webui $out/bin/llama-cpp-webui
        '';
      };

      # End-to-end round-trip (manual, needs a live llama-server + systemd).
      smoke = smoke;
    };

    checks.${system} = {
      go-vet-and-tests =
        pkgs.runCommand "llama-cpp-webui-go-tests" {
          nativeBuildInputs = [pkgs.go];
        } ''
          export CGO_ENABLED=0 HOME=$TMPDIR
          cd ${src}
          go vet ./...
          go test ./...
          : > $out
        '';

      ui-i18n =
        pkgs.runCommand "llama-cpp-webui-ui-tests" {
          nativeBuildInputs = [pkgs.nodejs];
        } ''
          cd ${src}
          node tests/ui_i18n.test.mjs
          : > $out
        '';

      gofmt =
        pkgs.runCommand "llama-cpp-webui-gofmt" {
          nativeBuildInputs = [pkgs.go];
        } ''
          cd ${src}
          unformatted=$(gofmt -l .)
          if [ -n "$unformatted" ]; then
            echo "files requiring gofmt:"
            echo "$unformatted"
            exit 1
          fi
          : > $out
        '';
    };

    # Dev environment: go + nodejs. CGO is disabled by default (no gcc needed).
    devShells.${system}.default = pkgs.mkShell {
      env = {CGO_ENABLED = "0";};
      packages = [
        pkgs.go
        pkgs.nodejs
        pkgs.git
        smoke
      ];
    };

    # `nix fmt` formats the Nix files. Go formatting is enforced by checks.gofmt.
    formatter.${system} = pkgs.alejandra;
  };
}
