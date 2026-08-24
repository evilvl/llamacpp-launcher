package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersion(t *testing.T) {
	app = appConfig{ServiceName: "llama-coder"}
	rec := httptest.NewRecorder()
	handleVersion(rec, httptest.NewRequest(http.MethodGet, "/api/version", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("json: %v", err)
	}
	if m["name"] != "llamacpp-launcher" {
		t.Fatalf("name = %v", m["name"])
	}
}

func TestModelsAndConfig(t *testing.T) {
	root := t.TempDir()
	configDir := t.TempDir()
	m := filepath.Join(root, "q2.5-7b.gguf")
	if err := os.WriteFile(m, []byte("data"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	app = appConfig{ModelRoot: root, ConfigDir: configDir, WaitTimeout: 600}

	// models: один, без конфига
	rec := httptest.NewRecorder()
	handleModels(rec, httptest.NewRequest(http.MethodGet, "/api/models", nil))
	var mr struct {
		Models []ModelInfo `json:"models"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &mr)
	if len(mr.Models) != 1 || mr.Models[0].Name != "q2.5-7b.gguf" {
		t.Fatalf("unexpected models: %+v", mr.Models)
	}

	// config get — дефолты
	rec = httptest.NewRecorder()
	handleConfigGet(rec, httptest.NewRequest(http.MethodGet, "/api/config?model="+m, nil))
	var cfg Config
	_ = json.Unmarshal(rec.Body.Bytes(), &cfg)
	if cfg.Flags["--port"] != "8080" {
		t.Fatalf("default config wrong: %+v", cfg)
	}

	// config save -> get
	cfg.Model = m
	cfg.Flags["--port"] = "9999"
	cfg.Flags["--api-key"] = "k"
	b, _ := json.Marshal(cfg)
	rec = httptest.NewRecorder()
	handleConfigSave(rec, httptest.NewRequest(http.MethodPost, "/api/config", bytes.NewReader(b)))
	if rec.Code != http.StatusOK {
		t.Fatalf("save code = %d body=%s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	handleConfigGet(rec, httptest.NewRequest(http.MethodGet, "/api/config?model="+m, nil))
	_ = json.Unmarshal(rec.Body.Bytes(), &cfg)
	if cfg.Flags["--port"] != "9999" || cfg.Flags["--api-key"] != "k" {
		t.Fatalf("saved config wrong: %+v", cfg)
	}
	if !fileExists(configPathFor(m)) {
		t.Fatalf("config file not written")
	}
}

func TestIndexServesHTML(t *testing.T) {
	rec := httptest.NewRecorder()
	handleIndex(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("index code = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "<html") {
		t.Fatalf("index missing html")
	}
}

func TestSaveInvalidJSON(t *testing.T) {
	app = appConfig{ConfigDir: t.TempDir()}
	rec := httptest.NewRecorder()
	handleConfigSave(rec, httptest.NewRequest(http.MethodPost, "/api/config", strings.NewReader("{not json")))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestServiceActionWrapper(t *testing.T) {
	do := func() (map[string]any, error) {
		return map[string]any{"SubState": "active", "MainPID": "42"}, nil
	}
	rec := httptest.NewRecorder()
	serviceAction(do)(rec, httptest.NewRequest(http.MethodPost, "/api/stop", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("success code = %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"SubState":"active"`) {
		t.Fatalf("body missing payload: %s", rec.Body.String())
	}

	doErr := func() (map[string]any, error) { return nil, errors.New("boom") }
	rec = httptest.NewRecorder()
	serviceAction(doErr)(rec, httptest.NewRequest(http.MethodPost, "/api/restart", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("error code = %d, want 503", rec.Code)
	}
}

func TestConfigSaveAcceptsNumbers(t *testing.T) {
	app = appConfig{ConfigDir: t.TempDir()}
	body := `{"model":"/opt/models/m.gguf","flags":{"--port":9999,"--ctx-size":4096,"--flash-attn":"on"}}`
	rec := httptest.NewRecorder()
	handleConfigSave(rec, httptest.NewRequest(http.MethodPost, "/api/config", strings.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("save code = %d body=%s", rec.Code, rec.Body.String())
	}
	data, err := os.ReadFile(configPathFor("/opt/models/m.gguf"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, "--port=9999") || !strings.Contains(s, "--ctx-size=4096") {
		t.Fatalf("numeric flags not persisted: %s", s)
	}
	if !strings.Contains(s, `--flash-attn="on"`) {
		t.Fatalf("string flag not persisted quoted: %s", s)
	}
}

func TestFlagsHandlerReturnsSorted(t *testing.T) {
	app = appConfig{}
	appFlags = []FlagDef{
		{Canonical: "--zeta", Kind: KindValue},
		{Canonical: "--alpha", Kind: KindToggle},
		{Canonical: "--mu", Kind: KindEnum, Choices: []string{"a", "b"}},
	}
	rec := httptest.NewRecorder()
	handleFlags(rec, httptest.NewRequest(http.MethodGet, "/api/flags", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	var got []FlagDef
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(got) != 3 || got[0].Canonical != "--alpha" || got[2].Canonical != "--zeta" {
		t.Fatalf("flags not sorted: %+v", got)
	}
}

func TestConfigGetReturnsAllKnownFlags(t *testing.T) {
	app = appConfig{ConfigDir: t.TempDir()}
	appFlags = []FlagDef{
		{Canonical: "--host", Kind: KindValue, Default: "0.0.0.0"},
		{Canonical: "--port", Kind: KindValue, Default: "8080"},
		{Canonical: "--flash-attn", Kind: KindEnum, Choices: []string{"on", "off"}, Default: "auto"},
	}
	m := "/opt/models/m.gguf"
	rec := httptest.NewRecorder()
	handleConfigGet(rec, httptest.NewRequest(http.MethodGet, "/api/config?model="+m, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp configResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v", err)
	}
	if resp.Model != m {
		t.Fatalf("model = %q", resp.Model)
	}
	for _, k := range []string{"--host", "--port", "--flash-attn"} {
		if _, ok := resp.Flags[k]; !ok {
			t.Errorf("flag %q missing from response", k)
		}
	}
	if resp.Flags["--port"] != "8080" {
		t.Errorf("default port = %q, want 8080", resp.Flags["--port"])
	}
}

func TestConfigGetMissingModel(t *testing.T) {
	app = appConfig{ConfigDir: t.TempDir()}
	rec := httptest.NewRecorder()
	handleConfigGet(rec, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}
