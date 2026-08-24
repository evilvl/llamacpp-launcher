package main

import (
	"errors"
	"flag"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// withRunCmd replaces the systemctl/journalctl runner with a stub that returns
// the given output and error, restoring the original on test cleanup.
func withRunCmd(t *testing.T, out string, err error) {
	t.Helper()
	orig := runCmd
	runCmd = func(name string, args ...string) (string, error) { return out, err }
	t.Cleanup(func() { runCmd = orig })
}

func TestHandleStatus(t *testing.T) {
	app = appConfig{ServiceName: "llama-coder"}
	// `systemctl show … --value` prints one value per property, in order.
	withRunCmd(t, "active\nloaded\n42", nil)
	rec := httptest.NewRecorder()
	handleStatus(rec, httptest.NewRequest(http.MethodGet, "/api/status", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"ActiveState":"active"`) {
		t.Fatalf("body missing active: %s", rec.Body.String())
	}
}

func TestHandleStartSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	model := "/opt/models/m.gguf"
	app = appConfig{ConfigDir: t.TempDir(), ServiceName: "llama-coder",
		ServiceFile: filepath.Join(t.TempDir(), "m.service"), WaitTimeout: 5, LlamaServer: "/opt/llama-bin/llama-server"}
	cfg := defaultConfig(model)
	cfg.Flags["--port"] = strconv.Itoa(portFromURL(srv.URL))
	if err := cfg.save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	withRunCmd(t, "", nil)

	rec := httptest.NewRecorder()
	handleStart(rec, httptest.NewRequest(http.MethodPost, "/api/start?model="+model, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "started_model") {
		t.Fatalf("missing started_model: %s", rec.Body.String())
	}
}

func TestHandleStartMissingModel(t *testing.T) {
	withRunCmd(t, "", nil)
	rec := httptest.NewRecorder()
	handleStart(rec, httptest.NewRequest(http.MethodPost, "/api/start", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleStartServiceError(t *testing.T) {
	model := "/opt/models/m.gguf"
	app = appConfig{ConfigDir: t.TempDir(), ServiceName: "llama-coder",
		ServiceFile: filepath.Join(t.TempDir(), "m.service"), WaitTimeout: 1, LlamaServer: "/opt/llama-bin/llama-server"}
	withRunCmd(t, "", errors.New("systemctl failed")) // systemctl fails
	rec := httptest.NewRecorder()
	handleStart(rec, httptest.NewRequest(http.MethodPost, "/api/start?model="+model, nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestHandleLogs(t *testing.T) {
	app = appConfig{ServiceName: "llama-coder"}
	withRunCmd(t, "line1\nline2\nline3", nil)
	rec := httptest.NewRecorder()
	handleLogs(rec, httptest.NewRequest(http.MethodGet, "/api/logs?lines=2", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "line1") || !strings.Contains(rec.Body.String(), "line3") {
		t.Fatalf("logs wrong: %s", rec.Body.String())
	}
}

func TestHandleHealthTestSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"LLAMA_OK"}}]}`))
	}))
	defer srv.Close()

	model := "/opt/models/m.gguf"
	app = appConfig{ConfigDir: t.TempDir(), WaitTimeout: 5, LlamaServer: "/x"}
	cfg := defaultConfig(model)
	cfg.Flags["--port"] = strconv.Itoa(portFromURL(srv.URL))
	if err := cfg.save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	rec := httptest.NewRecorder()
	handleHealthTest(rec, httptest.NewRequest(http.MethodPost, "/api/health-test?model="+model, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "LLAMA_OK") {
		t.Fatalf("missing LLAMA_OK: %s", rec.Body.String())
	}
}

func TestHandleIndexNotFound(t *testing.T) {
	rec := httptest.NewRecorder()
	handleIndex(rec, httptest.NewRequest(http.MethodGet, "/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestHandleModelsError(t *testing.T) {
	file := filepath.Join(t.TempDir(), "notadir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	app = appConfig{ModelRoot: file, ConfigDir: t.TempDir()}
	rec := httptest.NewRecorder()
	handleModels(rec, httptest.NewRequest(http.MethodGet, "/api/models", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestHumanSize(t *testing.T) {
	cases := map[int64]string{
		500:           "500 B",
		1024:          "1.0 KB",
		1 << 20:       "1.0 MB",
		1 << 30:       "1.0 GB",
		5 * (1 << 40): "5.0 TB",
	}
	for in, want := range cases {
		if got := humanSize(in); got != want {
			t.Errorf("humanSize(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestDiscoverModelsNotDir(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	app = appConfig{ModelRoot: file, ConfigDir: t.TempDir()}
	_, err := discoverModels()
	if err != errNotDir {
		t.Fatalf("expected errNotDir, got %v", err)
	}
}

func TestWriteUnitAndActions(t *testing.T) {
	app = appConfig{LlamaServer: "/opt/llama-bin/llama-server", ServiceName: "llama-coder",
		ServiceFile: filepath.Join(t.TempDir(), "unit.service")}
	generateUnit(defaultConfig("/opt/models/m.gguf"))

	withRunCmd(t, "", nil)
	st, err := stopService()
	if err != nil || st == nil {
		t.Fatalf("stopService: %v %+v", err, st)
	}
	if _, err := restartService(); err != nil {
		t.Fatalf("restartService: %v", err)
	}
}

func TestServiceStatusParsing(t *testing.T) {
	app = appConfig{ServiceName: "llama-coder"}
	// `systemctl show … --value` prints one value per property, in order.
	withRunCmd(t, "active\nrunning\nenabled\nloaded\n42\n3", nil)
	m, err := serviceStatus()
	if err != nil {
		t.Fatalf("serviceStatus: %v", err)
	}
	if m["ActiveState"] != "active" {
		t.Fatalf("ActiveState = %q", m["ActiveState"])
	}
}

func TestStatusSafeOnErr(t *testing.T) {
	app = appConfig{ServiceName: "llama-coder"}
	withRunCmd(t, "", errors.New("service command failed"))
	m := status()
	if _, ok := m["error"]; !ok {
		t.Fatalf("expected error key, got %+v", m)
	}
}

func TestReadLogs(t *testing.T) {
	app = appConfig{ServiceName: "llama-coder"}
	withRunCmd(t, "hello logs", nil)
	out, err := readLogs(10)
	if err != nil || out != "hello logs" {
		t.Fatalf("readLogs = %q, %v", out, err)
	}
}

func TestNullStr(t *testing.T) {
	if nullStr(nil) != nil {
		t.Fatalf("nil error should yield nil")
	}
	if nullStr(errNotDir) != errNotDir.Error() {
		t.Fatalf("expected error string")
	}
}

func TestWaitForReady(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	hostport := strings.TrimPrefix(srv.URL, "http://")
	_, port, _ := net.SplitHostPort(hostport)
	if !waitForReady("127.0.0.1:"+port, 2*time.Second) {
		t.Fatalf("waitForReady should return true")
	}
	if waitForReady("127.0.0.1:1", 500*time.Millisecond) {
		t.Fatalf("waitForReady should time out on a dead port")
	}
}

func TestSettingsPathEmptyDir(t *testing.T) {
	app = appConfig{ConfigDir: ""}
	if got := settingsPath(); got != settingsFileName {
		t.Fatalf("settingsPath with empty dir = %q, want %q", got, settingsFileName)
	}
}

func TestLoadSettingsInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	app = appConfig{ConfigDir: dir}
	if err := os.WriteFile(settingsPath(), []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if s := loadSettings(); s.WebPort != 8080 {
		t.Fatalf("invalid settings not reset: %+v", s)
	}
}

func TestHandleSettingsSaveInvalidHost(t *testing.T) {
	app = appConfig{ConfigDir: t.TempDir()}
	settings = Settings{WebHost: "127.0.0.1", WebPort: 8080}
	rec := httptest.NewRecorder()
	handleSettingsSave(rec, httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(`{"webHost":"a/b","webPort":"9090"}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleSettingsSaveInvalidPort(t *testing.T) {
	app = appConfig{ConfigDir: t.TempDir()}
	settings = Settings{WebHost: "127.0.0.1", WebPort: 8080}
	rec := httptest.NewRecorder()
	handleSettingsSave(rec, httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(`{"webHost":"127.0.0.1","webPort":"0"}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleConfigSaveRejectsExtraInvalid(t *testing.T) {
	app = appConfig{ConfigDir: t.TempDir()}
	rec := httptest.NewRecorder()
	// valid json but model missing
	handleConfigSave(rec, httptest.NewRequest(http.MethodPost, "/api/config", strings.NewReader(`{"flags":{"--port":"9090"}}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing model, got %d", rec.Code)
	}
}

func TestHandleConfigGetRejectsMissingModel(t *testing.T) {
	app = appConfig{ConfigDir: t.TempDir()}
	rec := httptest.NewRecorder()
	handleConfigGet(rec, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestFormatFlagValueDefault(t *testing.T) {
	if got := formatFlagValue([]string{"x"}); got != "[x]" {
		t.Fatalf("formatFlagValue fallback = %q, want [x]", got)
	}
}

func TestRunBindsAndWires(t *testing.T) {
	origSettings, origApp, origBound := settings, app, boundAddr
	t.Cleanup(func() { settings, app, boundAddr = origSettings, origApp, origBound })

	ln, srv, err := run("127.0.0.1", 0)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	defer ln.Close()
	if srv == nil {
		t.Fatalf("run returned nil server")
	}
	if boundAddr == "" {
		t.Fatalf("boundAddr not set")
	}
	// The bound address must be a real loopback address with a port.
	if !strings.HasPrefix(boundAddr, "127.0.0.1:") {
		t.Fatalf("boundAddr = %q", boundAddr)
	}
}

func TestMainBootstrap(t *testing.T) {
	origServe := serveFunc
	serveFunc = func(_ *http.Server, _ net.Listener) error { return nil }
	origFlags := flag.CommandLine
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	flag.CommandLine.SetOutput(io.Discard)
	origSettings, origApp, origBound := settings, app, boundAddr
	origEnv, haveEnv := os.LookupEnv("LLAMA_WEB_PORT")
	os.Setenv("LLAMA_WEB_PORT", "0") // bind a free port, avoid leaking on 8080
	t.Cleanup(func() {
		serveFunc = origServe
		flag.CommandLine = origFlags
		settings, app, boundAddr = origSettings, origApp, origBound
		if haveEnv {
			os.Setenv("LLAMA_WEB_PORT", origEnv)
		} else {
			os.Unsetenv("LLAMA_WEB_PORT")
		}
	})

	main()
	if boundAddr == "" {
		t.Fatalf("main did not set boundAddr")
	}
	if app.ServiceName == "" {
		t.Fatalf("main did not set app.ServiceName")
	}
}

func TestEnvHelpers(t *testing.T) {
	orig := os.Environ()
	t.Cleanup(func() {
		os.Clearenv()
		for _, e := range orig {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				os.Setenv(parts[0], parts[1])
			}
		}
	})

	os.Clearenv()
	if got := envOr("LLAMA_TEST_STR", "def"); got != "def" {
		t.Fatalf("envOr miss = %q, want def", got)
	}
	if got := envOr("LLAMA_TEST_STR", "def"); got != "def" {
		t.Fatalf("envOr = %q, want def", got)
	}
	os.Setenv("LLAMA_TEST_STR", "set")
	if got := envOr("LLAMA_TEST_STR", "def"); got != "set" {
		t.Fatalf("envOr hit = %q, want set", got)
	}
	if got := envInt("LLAMA_TEST_INT", 7); got != 7 {
		t.Fatalf("envInt miss = %d, want 7", got)
	}
	os.Setenv("LLAMA_TEST_INT", "42")
	if got := envInt("LLAMA_TEST_INT", 7); got != 42 {
		t.Fatalf("envInt hit = %d, want 42", got)
	}
	os.Setenv("LLAMA_TEST_INT", "notanint")
	if got := envInt("LLAMA_TEST_INT", 7); got != 7 {
		t.Fatalf("envInt bad value = %d, want 7", got)
	}
}

func TestLoadFlags(t *testing.T) {
	// Missing binary -> nil flag list.
	app = appConfig{LlamaServer: "/no/such/llama-server"}
	if f := loadFlags(); f != nil {
		t.Fatalf("expected nil flags for missing binary, got %v", f)
	}

	// A fake --help output parses into toggle + value flags.
	bin := filepath.Join(t.TempDir(), "llama-server")
	script := "#!/bin/sh\n" +
		`echo "-t,    --threads N   number of CPU threads ... (default: -1)"` + "\n" +
		`echo "--perf, --no-perf    whether to enable ... (default: false)"` + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}
	app = appConfig{LlamaServer: bin}
	f := loadFlags()
	if len(f) != 2 {
		t.Fatalf("want 2 flags, got %d: %+v", len(f), f)
	}

	// Non-executable / broken binary -> nil.
	bad := filepath.Join(t.TempDir(), "llama-server")
	if err := os.WriteFile(bad, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	app = appConfig{LlamaServer: bad}
	if f := loadFlags(); f != nil {
		t.Fatalf("expected nil for broken binary, got %v", f)
	}
}

func TestPortHostFromConfigFallbacks(t *testing.T) {
	cfg := defaultConfig("/opt/models/m.gguf")
	// Missing --port / --host fall back to defaults.
	if p := portFromConfig(cfg); p != 8080 {
		t.Fatalf("portFromConfig default = %d", p)
	}
	if h := hostFromConfig(cfg); h != "0.0.0.0" {
		t.Fatalf("hostFromConfig default = %q", h)
	}
	// Non-numeric --port falls back to 8080.
	cfg.Flags["--port"] = "abc"
	if p := portFromConfig(cfg); p != 8080 {
		t.Fatalf("portFromConfig bad = %d, want 8080", p)
	}
	cfg.Flags["--host"] = "10.0.0.1"
	if h := hostFromConfig(cfg); h != "10.0.0.1" {
		t.Fatalf("hostFromConfig = %q, want 10.0.0.1", h)
	}
}

func TestDiscoverModelsNonRegular(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "real.gguf")
	if err := os.WriteFile(target, []byte("data"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	link := filepath.Join(root, "link.gguf")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	app = appConfig{ModelRoot: root, ConfigDir: t.TempDir()}
	models, err := discoverModels()
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("want 2 models (regular + symlink), got %d: %+v", len(models), models)
	}
}

func TestHandleLogsBranches(t *testing.T) {
	app = appConfig{ServiceName: "llama-coder"}

	// Invalid --lines falls back to the default (no error).
	withRunCmd(t, "l1\nl2", nil)
	rec := httptest.NewRecorder()
	handleLogs(rec, httptest.NewRequest(http.MethodGet, "/api/logs?lines=abc", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}

	// runCmd failure -> 503.
	withRunCmd(t, "", errors.New("journalctl failed"))
	rec = httptest.NewRecorder()
	handleLogs(rec, httptest.NewRequest(http.MethodGet, "/api/logs", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestHandleHealthTestError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	model := "/opt/models/m.gguf"
	app = appConfig{ConfigDir: t.TempDir(), WaitTimeout: 5, LlamaServer: "/x"}
	cfg := defaultConfig(model)
	cfg.Flags["--port"] = strconv.Itoa(portFromURL(srv.URL))
	if err := cfg.save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	rec := httptest.NewRecorder()
	handleHealthTest(rec, httptest.NewRequest(http.MethodPost, "/api/health-test?model="+model, nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 on inference error, got %d", rec.Code)
	}
}

func TestHandleConfigSaveExtra(t *testing.T) {
	app = appConfig{ConfigDir: t.TempDir()}
	body := `{"model":"/opt/models/m.gguf","flags":{"--port":"9090"},"extra":"--foo --bar baz"}`
	rec := httptest.NewRecorder()
	handleConfigSave(rec, httptest.NewRequest(http.MethodPost, "/api/config", strings.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d body=%s", rec.Code, rec.Body.String())
	}
	data, err := os.ReadFile(configPathFor("/opt/models/m.gguf"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(data), `__extra__="--foo --bar baz"`) {
		t.Fatalf("extra not persisted quoted: %s", string(data))
	}
}

func TestHandleConfigSaveMissingModel(t *testing.T) {
	app = appConfig{ConfigDir: t.TempDir()}
	rec := httptest.NewRecorder()
	handleConfigSave(rec, httptest.NewRequest(http.MethodPost, "/api/config", strings.NewReader(`{"flags":{"--port":"1"}}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleModelsEmpty(t *testing.T) {
	app = appConfig{ModelRoot: t.TempDir(), ConfigDir: t.TempDir()}
	rec := httptest.NewRecorder()
	handleModels(rec, httptest.NewRequest(http.MethodGet, "/api/models", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"models":[]`) {
		t.Fatalf("expected empty models array, got %s", rec.Body.String())
	}
}

func TestStopRestartReadLogsError(t *testing.T) {
	app = appConfig{ServiceName: "llama-coder"}
	withRunCmd(t, "", errors.New("systemctl failed"))
	if _, err := stopService(); err == nil {
		t.Fatalf("stopService should error")
	}
	if _, err := restartService(); err == nil {
		t.Fatalf("restartService should error")
	}
	if _, err := readLogs(100); err == nil {
		t.Fatalf("readLogs should error")
	}
}

func TestReadLogsDefaultLines(t *testing.T) {
	app = appConfig{ServiceName: "llama-coder"}
	withRunCmd(t, "line", nil)
	out, err := readLogs(0) // non-positive -> default of 100
	if err != nil || out != "line" {
		t.Fatalf("readLogs(0) = %q, %v", out, err)
	}
}

func TestConfigFallbacksOnMissingKeys(t *testing.T) {
	empty := &Config{Model: "x", Flags: map[string]string{}}
	if portFromConfig(empty) != 8080 {
		t.Fatalf("portFromConfig on missing key = default")
	}
	if hostFromConfig(empty) != "0.0.0.0" {
		t.Fatalf("hostFromConfig on missing key = default")
	}
}

func TestHandleHealthTestMissingModel(t *testing.T) {
	app = appConfig{ConfigDir: t.TempDir()}
	rec := httptest.NewRecorder()
	handleHealthTest(rec, httptest.NewRequest(http.MethodPost, "/api/health-test", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleConfigSaveBoolNilValue(t *testing.T) {
	app = appConfig{ConfigDir: t.TempDir()}
	body := `{"model":"/opt/models/m.gguf","flags":{"--flash-attn":true,"--x":null}}`
	rec := httptest.NewRecorder()
	handleConfigSave(rec, httptest.NewRequest(http.MethodPost, "/api/config", strings.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d body=%s", rec.Code, rec.Body.String())
	}
	data, err := os.ReadFile(configPathFor("/opt/models/m.gguf"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(data), `--flash-attn="true"`) {
		t.Fatalf("bool not persisted: %s", string(data))
	}
	if !strings.Contains(string(data), `--x=""`) {
		t.Fatalf("nil not persisted: %s", string(data))
	}
}

func TestHandleConfigSaveBadJSON(t *testing.T) {
	app = appConfig{ConfigDir: t.TempDir()}
	rec := httptest.NewRecorder()
	handleConfigSave(rec, httptest.NewRequest(http.MethodPost, "/api/config", strings.NewReader("{not json")))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestSettingsSaveMkdirError(t *testing.T) {
	file := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	app = appConfig{ConfigDir: file} // a file, not a dir -> MkdirAll fails
	if err := (Settings{WebHost: "127.0.0.1", WebPort: 9090}).save(); err == nil {
		t.Fatalf("expected save to fail when config dir is a file")
	}
}

func TestUnquoteShellDoubleQuoteDefault(t *testing.T) {
	// A double-quoted string with a non-escape char exercises the default write branch.
	if got := unquoteShell(`"abc"`); got != "abc" {
		t.Fatalf("unquoteShell(\"%q\") = %q", `"abc"`, got)
	}
	if got := unquoteShell(`"a\"b"`); got != `a"b` {
		t.Fatalf("unquoteShell escaped = %q", got)
	}
}

func TestUnquoteShellShort(t *testing.T) {
	// Strings shorter than 2 chars skip the quote-handling branch entirely.
	for _, s := range []string{"", "a"} {
		if got := unquoteShell(s); got != s {
			t.Fatalf("unquoteShell(%q) = %q", s, got)
		}
	}
}

func TestParseConfigFileInvalidLine(t *testing.T) {
	dir := t.TempDir()
	app = appConfig{ConfigDir: dir}
	p := filepath.Join(dir, "m.conf")
	if err := os.WriteFile(p, []byte("--port=9090\nthis_line_has_no_equals\n\n# comment\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, err := parseConfigFile(p, "/opt/models/m.gguf")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Flags["--port"] != "9090" {
		t.Fatalf("--port = %q", cfg.Flags["--port"])
	}
	if _, ok := cfg.Flags["this_line_has_no_equals"]; ok {
		t.Fatalf("invalid line should be skipped, got %+v", cfg.Flags)
	}
}

func TestDiscoverModelsHasConfig(t *testing.T) {
	root := t.TempDir()
	configDir := t.TempDir()
	m := filepath.Join(root, "a.gguf")
	if err := os.WriteFile(m, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// A matching config file so HasConfig is true.
	os.WriteFile(filepath.Join(configDir, "a.conf"), []byte("--port=1"), 0o644)
	app = appConfig{ModelRoot: root, ConfigDir: configDir}
	models, err := discoverModels()
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(models) != 1 || !models[0].HasConfig {
		t.Fatalf("expected 1 model with HasConfig=true, got %+v", models)
	}
}

func TestLoadFlagsEmptyBinaryPath(t *testing.T) {
	app = appConfig{LlamaServer: ""}
	if f := loadFlags(); f != nil {
		t.Fatalf("expected nil flags for empty binary path, got %v", f)
	}
}

func TestParseFlagLineShortOnly(t *testing.T) {
	// A flag with only a short name (no --long) falls back to names[0] as canonical.
	f, ok := parseFlagLine("-t   N   number of CPU threads (default: -1)")
	if !ok {
		t.Fatalf("short-only flag did not parse")
	}
	if f.Canonical != "-t" {
		t.Fatalf("canonical = %q, want -t", f.Canonical)
	}
	if f.Kind != KindValue {
		t.Fatalf("kind = %q, want value", f.Kind)
	}
}

func TestStartServiceHealthTimeout(t *testing.T) {
	model := "/opt/models/m.gguf"
	app = appConfig{ConfigDir: t.TempDir(), ServiceName: "llama-coder",
		ServiceFile: filepath.Join(t.TempDir(), "m.service"), WaitTimeout: 1, LlamaServer: "/opt/llama-bin/llama-server"}
	if err := defaultConfig(model).save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	withRunCmd(t, "", nil) // systemctl calls succeed
	_, err := startService(model)
	if err == nil {
		t.Fatalf("expected health-wait timeout error")
	}
}

func TestParseConfigFileQuotedAndComments(t *testing.T) {
	dir := t.TempDir()
	app = appConfig{ConfigDir: dir}
	p := filepath.Join(dir, "m.conf")
	content := "# a comment\n\n" +
		"--port=9090\n" +
		`--api-key="quoted key"` + "\n" +
		`--load-mode='mlock'` + "\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, err := parseConfigFile(p, "/opt/models/m.gguf")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Flags["--port"] != "9090" {
		t.Fatalf("--port = %q", cfg.Flags["--port"])
	}
	if cfg.Flags["--api-key"] != "quoted key" {
		t.Fatalf("--api-key = %q", cfg.Flags["--api-key"])
	}
	if cfg.Flags["--load-mode"] != "mlock" {
		t.Fatalf("--load-mode = %q", cfg.Flags["--load-mode"])
	}
}

func TestNewMuxRoutes(t *testing.T) {
	app = appConfig{ServiceName: "llama-coder"}
	srv := httptest.NewServer(newMux())
	defer srv.Close()

	cases := []string{"/", "/api/version", "/api/presets", "/api/settings"}
	for _, path := range cases {
		resp, err := srv.Client().Get(srv.URL + path)
		if err != nil {
			t.Fatalf("get %s: %v", path, err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s: code = %d, want 200", path, resp.StatusCode)
		}
		_ = resp.Body.Close()
	}
	// POST routes must be registered too (expect a body/JSON, not 405).
	resp, err := srv.Client().Post(srv.URL+"/api/config", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("post /api/config: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST /api/config (missing model): code = %d, want 400", resp.StatusCode)
	}
}
