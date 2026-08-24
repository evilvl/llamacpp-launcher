package main

import (
	"embed"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"
)

//go:embed ui/index.html ui/styles.css ui/i18n.js ui/app.js
var uiFS embed.FS

// app is the global configuration (paths, names, timeouts).
type appConfig struct {
	ModelRoot   string
	LlamaServer string
	ServiceName string
	ConfigDir   string
	ServiceFile string
	WaitTimeout int
}

var app = appConfig{}

// newMux registers all HTTP routes. Extracted from main so the wiring can be
// exercised without binding a socket.
func newMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", handleIndex)
	mux.HandleFunc("GET /styles.css", handleStaticAsset)
	mux.HandleFunc("GET /i18n.js", handleStaticAsset)
	mux.HandleFunc("GET /app.js", handleStaticAsset)
	mux.HandleFunc("GET /api/flags", handleFlags)
	mux.HandleFunc("GET /api/presets", handlePresets)
	mux.HandleFunc("GET /api/models", handleModels)
	mux.HandleFunc("GET /api/config", handleConfigGet)
	mux.HandleFunc("POST /api/config", handleConfigSave)
	mux.HandleFunc("GET /api/status", handleStatus)
	mux.HandleFunc("POST /api/start", handleStart)
	mux.HandleFunc("POST /api/stop", serviceAction(stopService))
	mux.HandleFunc("POST /api/restart", serviceAction(restartService))
	mux.HandleFunc("GET /api/logs", handleLogs)
	mux.HandleFunc("POST /api/health-test", handleHealthTest)
	mux.HandleFunc("GET /api/version", handleVersion)
	mux.HandleFunc("GET /api/settings", handleSettingsGet)
	mux.HandleFunc("POST /api/settings", handleSettingsSave)
	return mux
}

// appFlags is the llama-server flag list loaded from --help.
var appFlags []FlagDef

var errNotDir = errors.New("not a directory")

// serveFunc starts the HTTP server. It is a package var so tests can run main()
// without blocking on (*http.Server).Serve; production uses the real method.
var serveFunc = (*http.Server).Serve

func main() {
	webSettings := loadSettings()
	webHost := flag.String("web-host", envOr("LLAMA_WEB_HOST", webSettings.WebHost), "web UI address")
	webPort := flag.Int("web-port", envInt("LLAMA_WEB_PORT", webSettings.WebPort), "web UI port")
	flag.Parse()

	settings = Settings{WebHost: *webHost, WebPort: *webPort}

	ln, srv, err := run(*webHost, *webPort)
	if err != nil {
		log.Fatalf("server error: %v", err)
	}
	log.Printf("llamacpp-launcher listens on http://%s  (service=%s)", boundAddr, app.ServiceName)
	log.Printf("log: journalctl -u %s -f", app.ServiceName)
	if err := serveFunc(srv, ln); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// run wires the app (settings, config, flags, routes) and returns a ready,
// listening server. Extracted from main so the bootstrap can be tested without
// parsing flags or blocking on Serve.
func run(webHost string, webPort int) (net.Listener, *http.Server, error) {
	app = appConfig{
		ModelRoot:   envOr("LLAMA_MODEL_ROOT", "/opt/models"),
		LlamaServer: envOr("LLAMA_SERVER_BIN", "/opt/llama-bin/llama-server"),
		ServiceName: envOr("LLAMA_SERVICE_NAME", "llama-coder"),
		ConfigDir:   envOr("LLAMA_CONFIG_DIR", "/etc/llama-cpp/configs"),
		ServiceFile: envOr("LLAMA_SERVICE_FILE", "/etc/systemd/system/"+envOr("LLAMA_SERVICE_NAME", "llama-coder")+".service"),
		WaitTimeout: envInt("LLAMA_WAIT_TIMEOUT", 600),
	}

	if fi, err := os.Stat(app.ModelRoot); err == nil && fi.IsDir() {
		log.Printf("model root: %s", app.ModelRoot)
	} else {
		log.Printf("WARNING: model root not available: %s (%v)", app.ModelRoot, err)
	}
	if _, err := os.Stat(app.LlamaServer); err == nil {
		log.Printf("llama-server: %s", app.LlamaServer)
	} else {
		log.Printf("WARNING: llama-server not found: %s (%v)", app.LlamaServer, err)
	}

	appFlags = loadFlags()
	log.Printf("loaded llama-server flags: %d", len(appFlags))

	addr := fmt.Sprintf("%s:%d", webHost, webPort)
	ln, actual, err := listenWithFallback(addr)
	if err != nil {
		return nil, nil, err
	}
	boundAddr = actual

	srv := &http.Server{
		Handler:           newMux(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	return ln, srv, nil
}

func handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"name": "llamacpp-launcher", "service": app.ServiceName})
}

func envOr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
