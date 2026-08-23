package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnquoteShell(t *testing.T) {
	cases := map[string]string{
		`"--flash-attn on"`:  "--flash-attn on",
		`'simple'`:           "simple",
		`plain`:              "plain",
		`a\ b`:               "a b",
		`"with \"q\""`:       `with "q"`,
	}
	for in, want := range cases {
		if got := unquoteShell(in); got != want {
			t.Errorf("unquoteShell(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseSaveRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	app = appConfig{ConfigDir: tmp, WaitTimeout: 120, ModelRoot: "/opt/models"}

	model := "/opt/models/qwen2.5-7b-instruct-q4_k_m.gguf"
	path := configPathFor(model)

	// сначала сохраним дефолтный конфиг
	orig := defaultConfig(model)
	orig.Flags["--host"] = "0.0.0.0"
	orig.Flags["--port"] = "8080"
	orig.Flags["--flash-attn"] = "on"
	orig.Flags["--api-key"] = "secret-key"
	if err := orig.save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	if !fileExists(path) {
		t.Fatalf("config file not created: %s", path)
	}

	// прочитаем обратно
	got, err := parseConfigFile(path, model)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Flags["--api-key"] != "secret-key" || got.Flags["--port"] != "8080" || got.Flags["--flash-attn"] != "on" {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
}

func TestLoadActiveConfig(t *testing.T) {
	tmp := t.TempDir()
	app = appConfig{ConfigDir: tmp, WaitTimeout: 120, ModelRoot: "/opt/models"}

	model := "/opt/models/qwen2.5-7b-instruct-q4_k_m.gguf"

	// No config on disk -> pure defaults.
	got := loadActiveConfig(model)
	if got.Flags["--port"] != "8080" || got.Flags["--host"] != "0.0.0.0" {
		t.Fatalf("expected defaults, got %+v", got.Flags)
	}

	// Persist a config, then load it back.
	orig := defaultConfig(model)
	orig.Flags["--port"] = "9999"
	orig.Flags["--api-key"] = "secret"
	if err := orig.save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	got = loadActiveConfig(model)
	if got.Flags["--port"] != "9999" || got.Flags["--api-key"] != "secret" {
		t.Fatalf("loaded config mismatch: %+v", got.Flags)
	}
}

func TestDiscoverModels(t *testing.T) {
	root := t.TempDir()
	// создаем .gguf и лишний .part (должен игнорироваться)
	m1 := filepath.Join(root, "a.gguf")
	m2 := filepath.Join(root, "sub", "b.gguf")
	partial := filepath.Join(root, "c.gguf.part")
	os.WriteFile(m1, []byte("x"), 0o644)
	os.MkdirAll(filepath.Dir(m2), 0o755)
	os.WriteFile(m2, []byte("y"), 0o644)
	os.WriteFile(partial, []byte("z"), 0o644)

	app = appConfig{ModelRoot: root, ConfigDir: t.TempDir()}
	models, err := discoverModels()
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("want 2 models, got %d: %+v", len(models), models)
	}
	for _, m := range models {
		if strings.HasSuffix(m.Path, ".part") {
			t.Fatalf("partial model leaked: %s", m.Path)
		}
	}
	if models[0].Name != "a.gguf" {
		t.Fatalf("sort order wrong: %s", models[0].Name)
	}
	if models[0].Size != 1 || models[1].Size != 1 {
		t.Fatalf("size wrong: %+v", models)
	}
}

func TestGenerateUnit(t *testing.T) {
	app = appConfig{LlamaServer: "/opt/llama-bin/llama-server", ServiceName: "llama-coder"}
	cfg := defaultConfig("/opt/models/m.gguf")
	cfg.Flags["--port"] = "8080"
	cfg.Flags["--ctx-size"] = "131072"
	cfg.Flags["--api-key"] = "testkey"
	unit := generateUnit(cfg)

	for _, need := range []string{
		"[Unit]", "[Service]", "ExecStart=",
		"-m /opt/models/m.gguf",
		"--port 8080",
		"--ctx-size 131072",
		"--api-key testkey",
		"WantedBy=multi-user.target",
		// NVIDIA power-management is best-effort and portable:
		"ExecStartPre=-/bin/sh -c",          // non-fatal
		"nvidia-smi not found",              // reports when absent
		"LLAMA_INSTALL_NVIDIA_TOOLS",        // optional auto-install hook
	} {
		if !strings.Contains(unit, need) {
			t.Errorf("unit missing %q\n---\n%s", need, unit)
		}
	}
}
