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

// Settings — глобальные настройки веб-сервера (адрес привязки).
// В отличие от Config не привязаны к модели и сохраняются один раз.
type Settings struct {
	WebHost string `json:"webHost"`
	WebPort int    `json:"webPort"`
}

// settings — активные веб-настройки; инициализируются в main из сохранённых + флагов.
var settings Settings

// settingsFileName — имя файла сохранения веб-настроек.
const settingsFileName = "webui-settings.json"

// boundAddr — фактический привязанный адрес (может отличаться от запрашиваемого,
// если использовался свободный порт). Заполняется в main после listenWithFallback.
var boundAddr string

// applyReexec запускает замену процесса с новыми web-настройками: стартует новый
// процесс, ждёт, пока он примет запросы на новом порту, и после отправки ответа
// завершает родительский процесс. В тестах заменяется на заглушку.
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

// settingsPath возвращает путь к файлу настроек в ConfigDir.
func settingsPath() string {
	dir := strings.TrimRight(app.ConfigDir, "/")
	if dir == "" {
		return settingsFileName
	}
	return dir + "/" + settingsFileName
}

// loadSettings читает сохранённые веб-настройки; при отсутствии или некорректности — дефолты.
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

// saveSettings атомарно сохраняет веб-настройки.
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

// validate проверяет корректность настроек.
func (s Settings) validate() error {
	if s.WebPort < 1 || s.WebPort > 65535 {
		return fmt.Errorf("порт вне диапазона 1-65535 (получено %d)", s.WebPort)
	}
	if strings.TrimSpace(s.WebHost) == "" {
		return errors.New("хост не указан")
	}
	if strings.ContainsAny(s.WebHost, "/ \t\n") {
		return fmt.Errorf("некорректный хост: %q", s.WebHost)
	}
	return nil
}

// listenWithFallback слушает запрошенный адрес; если он занят — переключается на
// свободный порт, выбранный ОС (127.0.0.1:0), и сообщает фактический адрес.
func listenWithFallback(addr string) (net.Listener, string, error) {
	ln, err := net.Listen("tcp", addr)
	if err == nil {
		return ln, ln.Addr().String(), nil
	}
	free, ferr := net.Listen("tcp", "127.0.0.1:0")
	if ferr != nil {
		return nil, "", fmt.Errorf("не удалось связаться с %s и найти свободный порт: %w", addr, ferr)
	}
	log.Printf("внимание: %s занят, веб-интерфейс запущен на свободном порту %s", addr, free.Addr().String())
	return free, free.Addr().String(), nil
}

// handleSettingsGet возвращает текущие веб-настройки и фактический адрес.
func handleSettingsGet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"webHost":   settings.WebHost,
		"webPort":   strconv.Itoa(settings.WebPort),
		"boundAddr": boundAddr,
	})
}

// handleSettingsSave сохраняет новые веб-настройки и запускает замену процесса.
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

	// Никаких изменений — подтверждаем без рестарта.
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

	// Процесс заменяется после отправки ответа (goroutine), чтобы клиент его получил.
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

// rebuildWebArgs отбрасывает флаги web-host/web-port из аргументов, чтобы новые значения
// пришли из окружения (LLAMA_WEB_HOST / LLAMA_WEB_PORT), а не затерлись старыми флагами.
func rebuildWebArgs(args []string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--web-host" || a == "--web-port":
			i++ // пропускаем значение
		case strings.HasPrefix(a, "--web-host="), strings.HasPrefix(a, "--web-port="):
			// уже внутри токена — просто пропускаем
		default:
			out = append(out, a)
		}
	}
	return out
}

// setEnv устанавливает переменную окружения, заменяя все старые определения ключа.
func setEnv(env []string, key, val string) []string {
	out := make([]string, 0, len(env)+1)
	for _, e := range env {
		if !strings.HasPrefix(e, key+"=") {
			out = append(out, e)
		}
	}
	return append(out, key+"="+val)
}

// reexecCommand собирает команду для перезапуска процесса с новыми web-направленными настройками.
func reexecCommand(host, port string) (*exec.Cmd, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("cannot resolve executable: %w", err)
	}
	cmd := exec.Command(exe, rebuildWebArgs(os.Args[1:])...)
	cmd.Env = setEnv(setEnv(os.Environ(), "LLAMA_WEB_HOST", host), "LLAMA_WEB_PORT", port)
	return cmd, nil
}

// waitForReady возвращает true, если http://addr/api/version отвечает 200 внутри таймаута.
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
