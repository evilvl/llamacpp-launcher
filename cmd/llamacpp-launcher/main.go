package main

import (
	"embed"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

//go:embed ui/index.html
var uiFS embed.FS

// app — глобальная конфигурация (пути, имена, таймауты).
type appConfig struct {
	ModelRoot   string
	LlamaServer string
	ServiceName string
	ConfigDir   string
	ServiceFile string
	WaitTimeout int
}

var app = appConfig{}

// appFlags — список флагов llama-server, загруженный из --help.
var appFlags []FlagDef

var errNotDir = errors.New("не каталог")

func main() {
	webSettings := loadSettings()
	webHost := flag.String("web-host", envOr("LLAMA_WEB_HOST", webSettings.WebHost), "адрес веб-интерфейса")
	webPort := flag.Int("web-port", envInt("LLAMA_WEB_PORT", webSettings.WebPort), "порт веб-интерфейса")
	flag.Parse()

	settings = Settings{WebHost: *webHost, WebPort: *webPort}

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
		log.Printf("WARNING: model root не доступен: %s (%v)", app.ModelRoot, err)
	}
	if _, err := os.Stat(app.LlamaServer); err == nil {
		log.Printf("llama-server: %s", app.LlamaServer)
	} else {
		log.Printf("WARNING: llama-server не найден: %s (%v)", app.LlamaServer, err)
	}

	appFlags = loadFlags()
	log.Printf("загружено флагов llama-server: %d", len(appFlags))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", handleIndex)
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

	addr := fmt.Sprintf("%s:%d", *webHost, *webPort)
	ln, actual, err := listenWithFallback(addr)
	if err != nil {
		log.Fatalf("server error: %v", err)
	}
	boundAddr = actual

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("llamacpp-launcher слушает http://%s  (service=%s)", actual, app.ServiceName)
	log.Printf("журнал: journalctl -u %s -f", app.ServiceName)
	if err := srv.Serve(ln); err != nil {
		log.Fatalf("server error: %v", err)
	}
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
