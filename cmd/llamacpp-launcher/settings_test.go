package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestSettingsRoundTrip(t *testing.T) {
	app = appConfig{ConfigDir: t.TempDir()}
	s := Settings{WebHost: "127.0.0.1", WebPort: 9090}
	if err := s.save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	got := loadSettings()
	if got.WebHost != s.WebHost || got.WebPort != s.WebPort {
		t.Fatalf("roundtrip mismatch: %+v vs %+v", got, s)
	}
}

func TestSettingsDefaultsWhenMissing(t *testing.T) {
	app = appConfig{ConfigDir: t.TempDir()}
	s := loadSettings()
	if s.WebHost != "127.0.0.1" || s.WebPort != 8080 {
		t.Fatalf("expected defaults, got %+v", s)
	}
}

func TestSettingsInvalidStoredIgnored(t *testing.T) {
	app = appConfig{ConfigDir: t.TempDir()}
	_ = (Settings{WebHost: "127.0.0.1", WebPort: 70000}).save()
	if s := loadSettings(); s.WebPort != 8080 {
		t.Fatalf("invalid port not reset: %+v", s)
	}
}

func TestSettingsValidate(t *testing.T) {
	bad := []Settings{
		{WebHost: "", WebPort: 8080},
		{WebHost: "127.0.0.1", WebPort: 0},
		{WebHost: "127.0.0.1", WebPort: 70000},
		{WebHost: "a/b", WebPort: 1},
	}
	for _, s := range bad {
		if err := s.validate(); err == nil {
			t.Errorf("expected error for %+v", s)
		}
	}
	if err := (Settings{WebHost: "127.0.0.1", WebPort: 65535}).validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSettingsGetHandler(t *testing.T) {
	app = appConfig{ConfigDir: t.TempDir()}
	settings = Settings{WebHost: "127.0.0.1", WebPort: 8080}
	rec := httptest.NewRecorder()
	handleSettingsGet(rec, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("json: %v", err)
	}
	if m["webPort"] != "8080" {
		t.Fatalf("webPort = %v", m["webPort"])
	}
	if m["webHost"] != "127.0.0.1" {
		t.Fatalf("webHost = %v", m["webHost"])
	}
}

func TestSettingsSaveHandlerPersistsAndTriggersReexec(t *testing.T) {
	app = appConfig{ConfigDir: t.TempDir()}
	settings = Settings{WebHost: "127.0.0.1", WebPort: 8080}

	var gotHost, gotPort string
	orig := applyReexec
	applyReexec = func(h, p string) { gotHost, gotPort = h, p }
	defer func() { applyReexec = orig }()

	rec := httptest.NewRecorder()
	handleSettingsSave(rec, httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(`{"webHost":"127.0.0.1","webPort":"9090"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d body=%s", rec.Code, rec.Body.String())
	}
	if gotHost != "127.0.0.1" || gotPort != "9090" {
		t.Fatalf("reexec not scheduled: %q %q", gotHost, gotPort)
	}
	if settings.WebPort != 9090 {
		t.Fatalf("settings not updated: %+v", settings)
	}
	if !fileExists(settingsPath()) {
		t.Fatalf("settings file not written")
	}
}

func TestSettingsSaveNoOpDoesNotReexec(t *testing.T) {
	app = appConfig{ConfigDir: t.TempDir()}
	settings = Settings{WebHost: "127.0.0.1", WebPort: 8080}

	reexecCalled := false
	orig := applyReexec
	applyReexec = func(h, p string) { reexecCalled = true }
	defer func() { applyReexec = orig }()

	rec := httptest.NewRecorder()
	handleSettingsSave(rec, httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(`{"webHost":"127.0.0.1","webPort":"8080"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	if reexecCalled {
		t.Fatalf("reexec should not fire when nothing changed")
	}
}

func TestSettingsSaveHandlerRejectsBadPort(t *testing.T) {
	app = appConfig{ConfigDir: t.TempDir()}
	settings = Settings{WebHost: "127.0.0.1", WebPort: 8080}

	reexecCalled := false
	orig := applyReexec
	applyReexec = func(h, p string) { reexecCalled = true }
	defer func() { applyReexec = orig }()

	rec := httptest.NewRecorder()
	handleSettingsSave(rec, httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(`{"webHost":"127.0.0.1","webPort":"0"}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if reexecCalled {
		t.Fatalf("reexec must not fire on invalid input")
	}
	if settings.WebPort != 8080 {
		t.Fatalf("settings must be unchanged after rejection: %+v", settings)
	}
}

func TestSettingsSaveRejectsBadJSON(t *testing.T) {
	app = appConfig{ConfigDir: t.TempDir()}
	settings = Settings{WebHost: "127.0.0.1", WebPort: 8080}
	rec := httptest.NewRecorder()
	handleSettingsSave(rec, httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader("{not json")))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestReexecCommandDropsWebFlags(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = append([]string{"llamacpp-launcher"}, "--web-host", "127.0.0.1", "--web-port", "8080", "-v", "--extra", "x")

	cmd, err := reexecCommand("127.0.0.1", "9090")
	if err != nil {
		t.Fatalf("reexecCommand: %v", err)
	}
	if got, want := strings.Join(cmd.Args[1:], " "), "-v --extra x"; got != want {
		t.Fatalf("args = %q, want %q", got, want)
	}
	var host, port string
	for _, e := range cmd.Env {
		if strings.HasPrefix(e, "LLAMA_WEB_HOST=") {
			host = e
		}
		if strings.HasPrefix(e, "LLAMA_WEB_PORT=") {
			port = e
		}
	}
	if host != "LLAMA_WEB_HOST=127.0.0.1" || port != "LLAMA_WEB_PORT=9090" {
		t.Fatalf("env = host %q, port %q", host, port)
	}
}

func TestSetEnvReplacesExisting(t *testing.T) {
	env := setEnv([]string{"A=1", "LLAMA_WEB_PORT=1", "B=2"}, "LLAMA_WEB_PORT", "9")
	if len(env) != 3 {
		t.Fatalf("expected 3 entries, got %v", env)
	}
	found := 0
	for _, e := range env {
		if e == "LLAMA_WEB_PORT=9" {
			found++
		}
	}
	if found != 1 {
		t.Fatalf("setEnv did not replace cleanly: %v", env)
	}
}

func TestListenWithFallbackPicksFreePort(t *testing.T) {
	// occupy an arbitrary port, then request the same address — it should fall back to a free one.
	blocker, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("blocker: %v", err)
	}
	addr := blocker.Addr().String()
	defer blocker.Close()

	ln, actual, err := listenWithFallback(addr)
	if err != nil {
		t.Fatalf("listenWithFallback: %v", err)
	}
	defer ln.Close()
	if actual == addr {
		t.Fatalf("expected a different (free) address, got %s", actual)
	}
}
