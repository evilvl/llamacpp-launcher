package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppConfigModelDirRoundTrip(t *testing.T) {
	app = appConfig{ConfigDir: t.TempDir()}
	s := Settings{WebHost: "127.0.0.1", WebPort: 8080, ModelDir: "/models/a", LlamaServer: "/bin/llama-server"}
	if err := s.save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	got := loadSettings()
	if got.ModelDir != "/models/a" || got.LlamaServer != "/bin/llama-server" {
		t.Fatalf("app fields not persisted: %+v", got)
	}
}

func TestLoadSettingsAppDefaults(t *testing.T) {
	app = appConfig{ConfigDir: t.TempDir()}
	s := loadSettings()
	if s.WebHost != "127.0.0.1" || s.WebPort != 8080 || s.ModelDir != "" || s.LlamaServer != "" {
		t.Fatalf("expected app defaults, got %+v", s)
	}
}

func TestPersistSettingsIfMissingCreates(t *testing.T) {
	app = appConfig{ConfigDir: t.TempDir()}
	settings = Settings{WebHost: "127.0.0.1", WebPort: 8080, ModelDir: "/models", LlamaServer: "/bin/llama-server"}
	persistSettingsIfMissing()
	if !fileExists(settingsPath()) {
		t.Fatalf("settings file not created")
	}
	got := loadSettings()
	if got.ModelDir != "/models" || got.LlamaServer != "/bin/llama-server" {
		t.Fatalf("created config wrong: %+v", got)
	}
}

func TestPersistSettingsIfMissingSkipsExisting(t *testing.T) {
	app = appConfig{ConfigDir: t.TempDir()}
	if err := (Settings{WebHost: "127.0.0.1", WebPort: 8080, ModelDir: "/keep"}).save(); err != nil {
		t.Fatalf("seed save: %v", err)
	}
	settings = Settings{WebHost: "127.0.0.1", WebPort: 9090, ModelDir: "/overwrite"}
	persistSettingsIfMissing()
	if got := loadSettings(); got.ModelDir != "/keep" {
		t.Fatalf("existing config was overwritten: %+v", got)
	}
}

func TestPersistSettingsIfMissingSkipsInvalid(t *testing.T) {
	app = appConfig{ConfigDir: t.TempDir()}
	settings = Settings{WebHost: "127.0.0.1", WebPort: 70000}
	persistSettingsIfMissing()
	if fileExists(settingsPath()) {
		t.Fatalf("settings file should not be created for invalid settings")
	}
}

func TestSetConfigPathOverrides(t *testing.T) {
	dir := t.TempDir()
	app = appConfig{ConfigDir: dir}
	custom := filepath.Join(dir, "custom.json")
	orig := explicitConfigPath
	explicitConfigPath = custom
	defer func() { explicitConfigPath = orig }()
	if settingsPath() != custom {
		t.Fatalf("settingsPath = %q, want %q", settingsPath(), custom)
	}
	s := Settings{WebHost: "127.0.0.1", WebPort: 8080, ModelDir: "/m"}
	if err := s.save(); err != nil {
		t.Fatalf("save custom: %v", err)
	}
	if got := loadSettings(); got.ModelDir != "/m" {
		t.Fatalf("custom config not read: %+v", got)
	}
}

func TestHandleModelDirSetUpdatesGlobal(t *testing.T) {
	root := t.TempDir()
	app = appConfig{ModelRoot: "/old", ConfigDir: t.TempDir()}
	settings = Settings{WebHost: "127.0.0.1", WebPort: 8080}

	rec := httptest.NewRecorder()
	handleModelDirSet(rec, httptest.NewRequest(http.MethodPost, "/api/app/model-dir", strings.NewReader(`{"modelDir":"`+root+`"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d body=%s", rec.Code, rec.Body.String())
	}
	if app.ModelRoot != root {
		t.Fatalf("app.ModelRoot not updated: %q", app.ModelRoot)
	}
	if settings.ModelDir != root {
		t.Fatalf("settings.ModelDir not updated: %q", settings.ModelDir)
	}
	if !fileExists(settingsPath()) {
		t.Fatalf("settings not persisted")
	}
}

func TestHandleModelDirSetRejectsMissing(t *testing.T) {
	app = appConfig{ConfigDir: t.TempDir()}
	settings = Settings{WebHost: "127.0.0.1", WebPort: 8080}
	rec := httptest.NewRecorder()
	handleModelDirSet(rec, httptest.NewRequest(http.MethodPost, "/api/app/model-dir", strings.NewReader(`{"modelDir":"/no/such/dir"}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleModelDirSetRejectsFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	app = appConfig{ConfigDir: t.TempDir()}
	settings = Settings{WebHost: "127.0.0.1", WebPort: 8080}
	rec := httptest.NewRecorder()
	handleModelDirSet(rec, httptest.NewRequest(http.MethodPost, "/api/app/model-dir", strings.NewReader(`{"modelDir":"`+file+`"}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a file, got %d", rec.Code)
	}
}

func TestHandleModelDirSetRejectsEmpty(t *testing.T) {
	app = appConfig{ConfigDir: t.TempDir()}
	settings = Settings{WebHost: "127.0.0.1", WebPort: 8080}
	rec := httptest.NewRecorder()
	handleModelDirSet(rec, httptest.NewRequest(http.MethodPost, "/api/app/model-dir", strings.NewReader(`{"modelDir":"  "}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleModelDirSetNoOp(t *testing.T) {
	root := t.TempDir()
	app = appConfig{ModelRoot: root, ConfigDir: t.TempDir()}
	settings = Settings{WebHost: "127.0.0.1", WebPort: 8080, ModelDir: root}
	rec := httptest.NewRecorder()
	handleModelDirSet(rec, httptest.NewRequest(http.MethodPost, "/api/app/model-dir", strings.NewReader(`{"modelDir":"`+root+`"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d body=%s", rec.Code, rec.Body.String())
	}
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("json: %v", err)
	}
	if m["restarted"] != false {
		t.Fatalf("expected restarted=false")
	}
}

func TestHandleLlamaServerSet(t *testing.T) {
	app = appConfig{LlamaServer: "/old", ConfigDir: t.TempDir()}
	settings = Settings{WebHost: "127.0.0.1", WebPort: 8080}
	rec := httptest.NewRecorder()
	handleLlamaServerSet(rec, httptest.NewRequest(http.MethodPost, "/api/app/llama-server", strings.NewReader(`{"llamaServer":"/new/llama-server"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d body=%s", rec.Code, rec.Body.String())
	}
	if app.LlamaServer != "/new/llama-server" {
		t.Fatalf("app.LlamaServer not updated: %q", app.LlamaServer)
	}
	if settings.LlamaServer != "/new/llama-server" {
		t.Fatalf("settings.LlamaServer not updated: %q", settings.LlamaServer)
	}
}

func TestHandleLlamaServerSetRejectsEmpty(t *testing.T) {
	app = appConfig{ConfigDir: t.TempDir()}
	settings = Settings{WebHost: "127.0.0.1", WebPort: 8080}
	rec := httptest.NewRecorder()
	handleLlamaServerSet(rec, httptest.NewRequest(http.MethodPost, "/api/app/llama-server", strings.NewReader(`{"llamaServer":"  "}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleSettingsGetIncludesAppFields(t *testing.T) {
	app = appConfig{ConfigDir: t.TempDir()}
	settings = Settings{WebHost: "127.0.0.1", WebPort: 8080, ModelDir: "/m", LlamaServer: "/s"}
	rec := httptest.NewRecorder()
	handleSettingsGet(rec, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("json: %v", err)
	}
	if m["modelDir"] != "/m" || m["llamaServer"] != "/s" {
		t.Fatalf("app fields missing: %v", m)
	}
}

func TestHandleSettingsSavePreservesAppFields(t *testing.T) {
	app = appConfig{ConfigDir: t.TempDir()}
	settings = Settings{WebHost: "127.0.0.1", WebPort: 8080, ModelDir: "/m", LlamaServer: "/s"}

	origReexec := applyReexec
	applyReexec = func(host, port string) {}
	defer func() { applyReexec = origReexec }()

	rec := httptest.NewRecorder()
	handleSettingsSave(rec, httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(`{"webHost":"127.0.0.1","webPort":"9090"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d body=%s", rec.Code, rec.Body.String())
	}
	got := loadSettings()
	if got.ModelDir != "/m" || got.LlamaServer != "/s" {
		t.Fatalf("app fields clobbered on save: %+v", got)
	}
}

func TestHandleSettingsSavePersistsLangAndNoRestart(t *testing.T) {
	app = appConfig{ConfigDir: t.TempDir()}
	settings = Settings{WebHost: "127.0.0.1", WebPort: 8080, Lang: "en"}

	origReexec := applyReexec
	applyReexec = func(host, port string) {}
	defer func() { applyReexec = origReexec }()

	rec := httptest.NewRecorder()
	handleSettingsSave(rec, httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(`{"webHost":"127.0.0.1","webPort":"8080","lang":"ru"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d body=%s", rec.Code, rec.Body.String())
	}
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("json: %v", err)
	}
	if m["lang"] != "ru" {
		t.Fatalf("lang not echoed: %v", m["lang"])
	}
	if m["restarted"] != false {
		t.Fatalf("lang-only change should not restart: %v", m["restarted"])
	}
	if settings.Lang != "ru" {
		t.Fatalf("settings.Lang not updated: %q", settings.Lang)
	}
	if got := loadSettings(); got.Lang != "ru" {
		t.Fatalf("lang not persisted: %q", got.Lang)
	}
}

func TestHandleSettingsGetIncludesLang(t *testing.T) {
	app = appConfig{ConfigDir: t.TempDir()}
	settings = Settings{WebHost: "127.0.0.1", WebPort: 8080, Lang: "ru"}
	rec := httptest.NewRecorder()
	handleSettingsGet(rec, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("json: %v", err)
	}
	if m["lang"] != "ru" {
		t.Fatalf("lang missing: %v", m["lang"])
	}
}

func TestHandleSettingsSaveIgnoresInvalidLang(t *testing.T) {
	app = appConfig{ConfigDir: t.TempDir()}
	settings = Settings{WebHost: "127.0.0.1", WebPort: 8080, Lang: "ru"}
	rec := httptest.NewRecorder()
	handleSettingsSave(rec, httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(`{"webHost":"127.0.0.1","webPort":"8080","lang":"fr"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d body=%s", rec.Code, rec.Body.String())
	}
	if settings.Lang != "ru" {
		t.Fatalf("invalid lang should be ignored, kept %q", settings.Lang)
	}
}

func TestTranslateErrLocalizesRootError(t *testing.T) {
	cases := map[string]string{
		"en": errServiceRequiresRootMsg["en"],
		"ru": errServiceRequiresRootMsg["ru"],
	}
	for lang, want := range cases {
		settings = Settings{Lang: lang}
		if got := translateErr(errServiceRequiresRoot); got != want {
			t.Fatalf("lang=%s translateErr = %q, want %q", lang, got, want)
		}
	}

	// Unknown language falls back to English.
	settings = Settings{Lang: "de"}
	if got := translateErr(errServiceRequiresRoot); got != errServiceRequiresRootMsg["en"] {
		t.Fatalf("invalid lang fallback = %q, want English", got)
	}

	// Non-sentinel errors are returned verbatim.
	settings = Settings{Lang: "ru"}
	sent := errors.New("boom")
	if got := translateErr(sent); got != "boom" {
		t.Fatalf("non-sentinel error changed: %q", got)
	}
}
