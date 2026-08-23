package main

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func segContains(segs []string, want string) bool {
	for _, s := range segs {
		if s == want {
			return true
		}
	}
	return false
}

func TestBuildExecStartSegments(t *testing.T) {
	cfg := defaultConfig("/opt/models/m.gguf")
	cfg.Flags["--host"] = "0.0.0.0"
	cfg.Flags["--port"] = "8080"
	cfg.Flags["--api-key"] = "testkey"
	segs := cfg.buildExecStart("/opt/llama-bin/llama-server")

	if len(segs) == 0 || segs[0] != "/opt/llama-bin/llama-server" {
		t.Fatalf("first segment must be binary, got %q", segs[0])
	}
	for _, want := range []string{
		"-m /opt/models/m.gguf",
		"--host 0.0.0.0",
		"--port 8080",
		"--api-key testkey",
	} {
		if !segContains(segs, want) {
			t.Errorf("segment %q missing. segs=%v", want, segs)
		}
	}
}

func TestBuildExecStartTogglesBare(t *testing.T) {
	appFlags = []FlagDef{{Canonical: "--kv-offload", Kind: KindToggle}}
	cfg := defaultConfig("/opt/models/m.gguf")
	// kv-offload is a toggle flag; value "1" => bare flag, "0" => omitted.
	cfg.Flags["--kv-offload"] = "1"
	segs := cfg.buildExecStart("/x/llama-server")
	if !segContains(segs, "--kv-offload") {
		t.Errorf("toggle enabled should appear bare, segs=%v", segs)
	}

	cfg.Flags["--kv-offload"] = "0"
	segs = cfg.buildExecStart("/x/llama-server")
	if segContains(segs, "--kv-offload") {
		t.Errorf("toggle disabled should be omitted, segs=%v", segs)
	}
}

func TestBuildExecStartExtra(t *testing.T) {
	cfg := defaultConfig("/opt/models/m.gguf")
	cfg.Flags["__extra__"] = "--foo --bar baz"
	segs := cfg.buildExecStart("/x/llama-server")
	for _, want := range []string{"--foo", "--bar", "baz"} {
		if !segContains(segs, want) {
			t.Errorf("extra arg %q missing, segs=%v", want, segs)
		}
	}
}

func TestConfigHelpers(t *testing.T) {
	cfg := defaultConfig("/opt/models/m.gguf")

	if got := portFromConfig(cfg); got != 8080 {
		t.Errorf("portFromConfig default = %d, want 8080", got)
	}
	if got := hostFromConfig(cfg); got != "0.0.0.0" {
		t.Errorf("hostFromConfig default = %q, want 0.0.0.0", got)
	}
	if got := apiKeyFromConfig(cfg); got != "" {
		t.Errorf("apiKeyFromConfig default = %q, want empty", got)
	}

	cfg.Flags["--port"] = "9000"
	cfg.Flags["--host"] = "127.0.0.1"
	cfg.Flags["--api-key"] = "secret"
	if got := portFromConfig(cfg); got != 9000 {
		t.Errorf("portFromConfig = %d, want 9000", got)
	}
	if got := hostFromConfig(cfg); got != "127.0.0.1" {
		t.Errorf("hostFromConfig = %q, want 127.0.0.1", got)
	}
	if got := apiKeyFromConfig(cfg); got != "secret" {
		t.Errorf("apiKeyFromConfig = %q, want secret", got)
	}
}

func TestGenerateUnitFormat(t *testing.T) {
	app = appConfig{LlamaServer: "/opt/llama-bin/llama-server"}
	cfg := defaultConfig("/opt/models/m.gguf")
	cfg.Flags["--port"] = "8080"
	cfg.Flags["--api-key"] = "testkey"
	unit := generateUnit(cfg)

	if !strings.Contains(unit, "ExecStart=/opt/llama-bin/llama-server ") {
		t.Errorf("ExecStart must start with binary, unit=%q", unit)
	}

	backslash := string(rune(92))
	want := "ExecStart=/opt/llama-bin/llama-server " + backslash + "\n-m /opt/models/m.gguf " +
		backslash + "\n--host 0.0.0.0 " + backslash + "\n--port 8080 " + backslash + "\n--api-key testkey"
	if !strings.Contains(unit, want) {
		t.Errorf("segments must be joined as \"flag value\" on continuation lines\nwant:%q\n---\n%s", want, unit)
	}

	// Only the ExecStart segment lines matter: every continuation line after
	// the binary must be a flag (starts with "-"), i.e. a flag and its value
	// must never be split across two lines. Segments end at the first blank
	// line that separates them from the rest of the unit.
	value := unit[strings.Index(unit, "ExecStart=")+len("ExecStart="):]
	if di := strings.Index(value, "\n\n"); di >= 0 {
		value = value[:di]
	}
	parts := strings.Split(value, "\n")
	for j, p := range parts {
		trimmed := strings.TrimRight(p, " \\\t")
		if j == 0 {
			if !strings.HasPrefix(trimmed, "/opt/llama-bin/llama-server") {
				t.Errorf("ExecStart first segment must be binary: %q", trimmed)
			}
			continue
		}
		if !strings.HasPrefix(trimmed, "-") {
			t.Errorf("continuation line is not a flag (value got split off): %q", trimmed)
		}
	}
}

func TestParseConfigFileMissingReturnsDefaults(t *testing.T) {
	cfg, err := parseConfigFile("/nonexistent/path.conf", "/opt/models/m.gguf")
	if err != nil {
		t.Fatalf("missing config should not error: %v", err)
	}
	if cfg.Model != "/opt/models/m.gguf" {
		t.Errorf("model = %q", cfg.Model)
	}
	if cfg.Flags["--host"] != "0.0.0.0" {
		t.Errorf("default host missing: %q", cfg.Flags["--host"])
	}
}

func portFromURL(u string) int {
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(u, "http://"))
	return mustAtoi(port)
}

func mustAtoi(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

func TestSanitize(t *testing.T) {
	cases := map[string]string{
		"/opt/models/foo.gguf": "_opt_models_foo.gguf",
		"a\\b:c d":             "a_b_c_d",
		"":                     "",
	}
	for in, want := range cases {
		if got := sanitize(in); got != want {
			t.Errorf("sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWaitForHealthSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := waitForHealth("127.0.0.1", portFromURL(srv.URL), 5); err != nil {
		t.Fatalf("waitForHealth = %v, want nil", err)
	}
}

func TestWaitForHealthTimeout(t *testing.T) {
	// Server answers 503 forever -> waitForHealth must give up at the deadline.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	done := make(chan error, 1)
	go func() { done <- waitForHealth("127.0.0.1", portFromURL(srv.URL), 1) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("waitForHealth = nil, want timeout error")
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("waitForHealth did not return within 10s")
	}
}

func TestInferenceTestSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"LLAMA_OK"}}],"timings":{"prediction_per_second":42.0}}`)
	}))
	defer srv.Close()

	tmp := t.TempDir()
	app = appConfig{ConfigDir: tmp, WaitTimeout: 5}
	cfg := defaultConfig("/opt/models/m.gguf")
	cfg.Flags["--port"] = strconv.Itoa(portFromURL(srv.URL))
	if err := cfg.save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	res, err := inferenceTest(cfg.Model)
	if err != nil {
		t.Fatalf("inferenceTest err = %v", err)
	}
	if res["content"] != "LLAMA_OK" {
		t.Fatalf("content = %v, want LLAMA_OK", res["content"])
	}
}

func TestInferenceTestHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, "nope")
	}))
	defer srv.Close()

	tmp := t.TempDir()
	app = appConfig{ConfigDir: tmp, WaitTimeout: 5}
	cfg := defaultConfig("/opt/models/m.gguf")
	cfg.Flags["--port"] = strconv.Itoa(portFromURL(srv.URL))
	if err := cfg.save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	_, err := inferenceTest(cfg.Model)
	if err == nil {
		t.Fatalf("inferenceTest err = nil, want HTTP error")
	}
}

func TestInferenceTestConnectFailure(t *testing.T) {
	// Nothing is listening -> inferenceTest must return without panicking.
	tmp := t.TempDir()
	app = appConfig{ConfigDir: tmp, WaitTimeout: 5}
	cfg := defaultConfig("/opt/models/m.gguf")
	cfg.Flags["--port"] = "1"
	if err := cfg.save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	res, err := inferenceTest(cfg.Model)
	if err != nil {
		t.Fatalf("connect failure returns nil error, got %v", err)
	}
	if _, ok := res["error"]; !ok {
		t.Fatalf("expected error key in result, got %v", res)
	}
}
