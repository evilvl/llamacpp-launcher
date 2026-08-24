package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Settings — global web-server settings (bind address).
// Unlike Config, they are not bound to a model and are saved once.
type Settings struct {
	WebHost string `json:"webHost"`
	WebPort int    `json:"webPort"`
}

// settings — active web settings; initialized in main from the saved ones plus flags.
var settings Settings

// settingsFileName — the web-settings filename.
const settingsFileName = "webui-settings.json"

// boundAddr — the actual bound address (may differ from the requested one,
// if a free port was used). Set in main after listenWithFallback.
var boundAddr string

// applyReexec runs process replacement with new web settings: it starts a new
// process, waits until it accepts requests on the new port, and after sending the
// response terminates the parent process. In tests it's replaced by a stub.
var applyReexec = func(host, port string) {
	go func() {
		cmd, err := reexecCommand(host, port)
		if err != nil {
			log.Printf("re-exec: %v", err)
			return
		}
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			log.Printf("re-exec: start failed: %v", err)
			return
		}
		addr := host + ":" + port
		if !waitForReady(addr, 15*time.Second) {
			log.Printf("re-exec: replacement did not become ready on %s", addr)
			return
		}
		time.Sleep(300 * time.Millisecond)
		os.Exit(0)
	}()
}

// settingsPath returns the path to the settings file in ConfigDir.
func settingsPath() string {
	dir := strings.TrimRight(app.ConfigDir, "/")
	if dir == "" {
		return settingsFileName
	}
	return dir + "/" + settingsFileName
}

// loadSettings reads the saved web settings; on missing or invalid ones, uses defaults.
func loadSettings() Settings {
	b, err := os.ReadFile(settingsPath())
	if err != nil {
		return Settings{WebHost: "127.0.0.1", WebPort: 8080}
	}
	var s Settings
	if err := json.Unmarshal(b, &s); err != nil || s.WebPort < 1 || s.WebPort > 65535 || strings.TrimSpace(s.WebHost) == "" {
		return Settings{WebHost: "127.0.0.1", WebPort: 8080}
	}
	return s
}

// saveSettings atomically saves the web settings.
func (s Settings) save() error {
	dir := strings.TrimRight(app.ConfigDir, "/")
	if dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := settingsPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, settingsPath())
}

// validate checks the settings.
func (s Settings) validate() error {
	if s.WebPort < 1 || s.WebPort > 65535 {
		return fmt.Errorf("port out of range 1-65535 (got %d)", s.WebPort)
	}
	if strings.TrimSpace(s.WebHost) == "" {
		return errors.New("host not specified")
	}
	if strings.ContainsAny(s.WebHost, "/ \t\n") {
		return fmt.Errorf("invalid host: %q", s.WebHost)
	}
	return nil
}

// listenWithFallback listens on the requested address; if it's busy, falls back to
// a free port chosen by the OS (127.0.0.1:0) and reports the actual address.
func listenWithFallback(addr string) (net.Listener, string, error) {
	ln, err := net.Listen("tcp", addr)
	if err == nil {
		return ln, ln.Addr().String(), nil
	}
	free, ferr := net.Listen("tcp", "127.0.0.1:0")
	if ferr != nil {
		return nil, "", fmt.Errorf("could not bind %s and find a free port: %w", addr, ferr)
	}
	log.Printf("warning: %s is in use, web UI started on free port %s", addr, free.Addr().String())
	return free, free.Addr().String(), nil
}

// handleSettingsGet returns the current web settings and the actual address.
func handleSettingsGet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"webHost":   settings.WebHost,
		"webPort":   strconv.Itoa(settings.WebPort),
		"boundAddr": boundAddr,
	})
}

// handleSettingsSave saves the new web settings and triggers process replacement.
func handleSettingsSave(w http.ResponseWriter, r *http.Request) {
	var req struct {
		WebHost string `json:"webHost"`
		WebPort string `json:"webPort"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	port := 0
	if n, err := strconv.Atoi(strings.TrimSpace(req.WebPort)); err == nil {
		port = n
	}
	newSettings := Settings{WebHost: strings.TrimSpace(req.WebHost), WebPort: port}
	if err := newSettings.validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// No changes — confirm without restart.
	if newSettings.WebHost == settings.WebHost && newSettings.WebPort == settings.WebPort {
		writeJSON(w, okSettings(false))
		return
	}

	prev := settings
	if err := newSettings.save(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	settings = newSettings
	writeJSON(w, okSettings(true))

	// The process is replaced after the response is sent (goroutine) so the client gets it.
	applyReexec(settings.WebHost, strconv.Itoa(settings.WebPort))
	_ = prev
}

func okSettings(restarted bool) map[string]any {
	return map[string]any{
		"ok":        true,
		"webHost":   settings.WebHost,
		"webPort":   strconv.Itoa(settings.WebPort),
		"boundAddr": boundAddr,
		"restarted": restarted,
	}
}

// rebuildWebArgs drops the web-host/web-port flags so the new values
// come from the environment (LLAMA_WEB_HOST / LLAMA_WEB_PORT), not the old flags.
func rebuildWebArgs(args []string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--web-host" || a == "--web-port":
			i++ // skip the value
		case strings.HasPrefix(a, "--web-host="), strings.HasPrefix(a, "--web-port="):
			// already inside the token — just skip it
		default:
			out = append(out, a)
		}
	}
	return out
}

// setEnv sets the environment variable, replacing all old definitions of the key.
func setEnv(env []string, key, val string) []string {
	out := make([]string, 0, len(env)+1)
	for _, e := range env {
		if !strings.HasPrefix(e, key+"=") {
			out = append(out, e)
		}
	}
	return append(out, key+"="+val)
}

// reexecCommand builds the command to restart the process with new web settings.
func reexecCommand(host, port string) (*exec.Cmd, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("cannot resolve executable: %w", err)
	}
	cmd := exec.Command(exe, rebuildWebArgs(os.Args[1:])...)
	cmd.Env = setEnv(setEnv(os.Environ(), "LLAMA_WEB_HOST", host), "LLAMA_WEB_PORT", port)
	return cmd, nil
}

// waitForReady returns true if http://addr/api/version responds 200 within the timeout.
func waitForReady(addr string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		client := &http.Client{Timeout: time.Second}
		resp, err := client.Get("http://" + addr + "/api/version")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	return false
}
