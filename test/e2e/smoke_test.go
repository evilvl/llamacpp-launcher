//go:build integration

package e2e

import (
	"bytes"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSmoke(t *testing.T) {
	bin, err := buildBinary()
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(bin) })

	root := t.TempDir()
	modelRoot := filepath.Join(root, "models")
	configDir := filepath.Join(root, "configs")
	if err := os.MkdirAll(modelRoot, 0o755); err != nil {
		t.Fatalf("mkdir model root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(modelRoot, "tiny.gguf"), []byte("GGUF"), 0o644); err != nil {
		t.Fatalf("write model: %v", err)
	}

	port, err := freePort()
	if err != nil {
		t.Fatalf("free port: %v", err)
	}

	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), serverEnv(modelRoot, configDir, port)...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	base := "http://127.0.0.1:" + strconv.Itoa(port)
	if err := waitForHTTP(base, 10*time.Second); err != nil {
		t.Fatalf("server not ready: %v\n--- server log ---\n%s", err, buf.String())
	}

	var version map[string]any
	if err := httpGetJSON(base, "/api/version", &version); err != nil {
		t.Fatalf("version: %v", err)
	}
	if version["name"] != "llamacpp-launcher" {
		t.Errorf("version name = %v, want llamacpp-launcher", version["name"])
	}

	model := filepath.Join(modelRoot, "tiny.gguf")

	saveBody := map[string]any{
		"model": model,
		"flags": map[string]any{"--port": "8090", "--api-key": "abc", "--ctx-size": "4096"},
		"extra": "--verbose --n-batch 512",
	}
	var saved map[string]any
	if err := httpPostJSON(base, "/api/config", saveBody, &saved); err != nil {
		t.Fatalf("save config: %v", err)
	}

	var got apiConfig
	if err := httpGetJSON(base, "/api/config?model="+url.QueryEscape(model), &got); err != nil {
		t.Fatalf("get config: %v", err)
	}
	if got.Flags["--port"] != "8090" || got.Flags["--api-key"] != "abc" || got.Flags["--ctx-size"] != "4096" {
		t.Errorf("flags round-trip mismatch: %+v", got.Flags)
	}
	if got.Flags["__extra__"] != "--verbose --n-batch 512" {
		t.Errorf("__extra__ = %q, want %q", got.Flags["__extra__"], "--verbose --n-batch 512")
	}

	wantFile := filepath.Join(configDir, "tiny.conf")
	data, err := os.ReadFile(wantFile)
	if err != nil {
		t.Fatalf("read config file %s: %v", wantFile, err)
	}
	for _, wantLine := range []string{"--port=8090", `--api-key="abc"`, "--ctx-size=4096", `__extra__="--verbose --n-batch 512"`} {
		if !strings.Contains(string(data), wantLine) {
			t.Errorf("config file missing %q\n---\n%s", wantLine, data)
		}
	}

	var modelsResp struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := httpGetJSON(base, "/api/models", &modelsResp); err != nil {
		t.Fatalf("models: %v", err)
	}
	found := false
	for _, m := range modelsResp.Models {
		if m.Name == "tiny.gguf" {
			found = true
		}
	}
	if !found {
		t.Errorf("tiny.gguf not listed in /api/models: %+v", modelsResp.Models)
	}

	var presets []map[string]any
	if err := httpGetJSON(base, "/api/presets", &presets); err != nil {
		t.Fatalf("presets: %v", err)
	}
	if len(presets) == 0 {
		t.Error("/api/presets returned empty")
	}

	var flags []map[string]any
	if err := httpGetJSON(base, "/api/flags", &flags); err != nil {
		t.Fatalf("flags: %v", err)
	}
}

type apiConfig struct {
	Model string            `json:"model"`
	Flags map[string]string `json:"flags"`
	Extra string            `json:"extra"`
}
