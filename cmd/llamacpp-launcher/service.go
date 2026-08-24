package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// runCmd runs a command and returns the combined stdout+stderr.
var runCmd = func(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// generateUnit builds the content of the systemd unit for a model (analog of generate_service from bash).
func generateUnit(c *Config) string {
	unit := `[Unit]
Description=llama.cpp LLM Coder GPU + CPU
After=network-online.target
Wants=network-online.target
### OPTIMIZED: protection against frequent restarts
StartLimitIntervalSec=30
StartLimitBurst=3

[Service]
Type=simple
User=root

Environment="CUDA_VISIBLE_DEVICES=0"
Environment="GGML_CUDA_GRAPHS=1"
Environment="GGML_CUDA_FORCE_MMQ=0"
Environment="GGML_CUDA_NO_PEER_COPY=1"
Environment="GGML_CUDA_NO_VMM=0"

# NVIDIA power-management is best-effort: applied only when nvidia-smi exists.
# On AMD/CPU hosts it logs a note and continues (never hard-fails). Set
# LLAMA_INSTALL_NVIDIA_TOOLS to a command to auto-install the tools when missing.
ExecStartPre=-/bin/sh -c 'if command -v nvidia-smi >/dev/null 2>&1; then /usr/bin/nvidia-smi -pm 1; /usr/bin/nvidia-smi -pl 320 2>/dev/null || true; elif [ -n "$LLAMA_INSTALL_NVIDIA_TOOLS" ]; then eval "$LLAMA_INSTALL_NVIDIA_TOOLS" || true; else echo "[llama-coder] nvidia-smi not found - CUDA acceleration disabled"; fi'

ExecStart=` + strings.Join(c.buildExecStart(app.LlamaServer), " \\\n") + `

Restart=on-failure
RestartSec=10
KillSignal=SIGTERM
TimeoutStopSec=30
LimitNOFILE=1048576
LimitMEMLOCK=infinity
OOMScoreAdjust=-100

[Install]
WantedBy=multi-user.target
`
	return unit
}

// writeUnit atomically writes the unit to /etc/systemd/system.
func writeUnit(c *Config) error {
	tmp := app.ServiceFile + ".tmp"
	if err := os.WriteFile(tmp, []byte(generateUnit(c)), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, app.ServiceFile)
}

// startService loads/generates the unit, enables and starts the service, waits for /health.
func startService(model string) (map[string]any, error) {
	cfg := loadActiveConfig(model)
	if err := writeUnit(cfg); err != nil {
		return status(), err
	}

	if _, err := runCmd("systemctl", "daemon-reload"); err != nil {
		return status(), err
	}
	if _, err := runCmd("systemctl", "enable", app.ServiceName); err != nil {
		return status(), err
	}
	if _, err := runCmd("systemctl", "start", app.ServiceName); err != nil {
		return status(), err
	}

	werr := waitForHealth(hostFromConfig(cfg), portFromConfig(cfg), app.WaitTimeout)

	st := status()
	st["started_model"] = cfg.Model
	st["wait_error"] = nullStr(werr)
	return st, werr
}

// stopService stops the service.
func stopService() (map[string]any, error) {
	if _, err := runCmd("systemctl", "stop", app.ServiceName); err != nil {
		return status(), err
	}
	return status(), nil
}

// restartService restarts the service.
func restartService() (map[string]any, error) {
	if _, err := runCmd("systemctl", "restart", app.ServiceName); err != nil {
		return status(), err
	}
	return status(), nil
}

// serviceAction wraps the model-less action handlers (stop/restart):
// calls the given function and serializes the result as JSON.
func serviceAction(do func() (map[string]any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		st, err := do()
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		writeJSON(w, st)
	}
}

// serviceStatus reads the service state via systemctl show.
func serviceStatus() (map[string]any, error) {
	out, err := runCmd("systemctl", "show",
		"-p", "ActiveState", "-p", "SubState",
		"-p", "UnitFileState", "-p", "LoadState",
		"-p", "MainPID", "-p", "NRestarts",
		"--value", app.ServiceName)
	if err != nil {
		return status(), err
	}
	m := map[string]any{}
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 2 {
			m[parts[0]] = parts[1]
		}
	}
	return m, nil
}

// status is a safe wrapper over serviceStatus (it does not fail if systemd is unavailable).
func status() map[string]any {
	m, err := serviceStatus()
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	return m
}

// readLogs prints the last N lines of the service log.
func readLogs(n int) (string, error) {
	if n <= 0 {
		n = 100
	}
	return runCmd("journalctl", "-u", app.ServiceName, "-n", strconv.Itoa(n), "--no-pager")
}

// waitForHealth waits until /health returns 200, or times out.
func waitForHealth(host string, port, timeout int) error {
	target := port
	if port == 0 {
		target = 8080
	}
	deadline := time.Now().Add(time.Duration(timeout) * time.Second)
	url := fmt.Sprintf("http://127.0.0.1:%d/health", target)

	for {
		client := &http.Client{Timeout: 2 * time.Second}
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("server did not become healthy within %d sec (http=%v)", timeout, err)
		}
		time.Sleep(2 * time.Second)
	}
}

// inferenceTest sends a test request to the API and parses the response.
func inferenceTest(model string) (map[string]any, error) {
	cfg := loadActiveConfig(model)

	body, _ := json.Marshal(map[string]any{
		"model":       "local-model",
		"temperature": 0,
		"max_tokens":  16,
		"stream":      false,
		"messages": []map[string]string{
			{"role": "user", "content": "Reply with exactly: LLAMA_OK"},
		},
	})

	url := fmt.Sprintf("http://127.0.0.1:%d/v1/chat/completions", portFromConfig(cfg))
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if key := apiKeyFromConfig(cfg); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	client := &http.Client{Timeout: 180 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return map[string]any{"error": err.Error()}, nil
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	result := map[string]any{"http": resp.StatusCode}
	if resp.StatusCode != http.StatusOK {
		result["raw"] = string(raw)
		return result, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Timings map[string]float64 `json:"timings"`
	}
	if json.Unmarshal(raw, &parsed) == nil {
		if len(parsed.Choices) > 0 {
			result["content"] = parsed.Choices[0].Message.Content
		}
		if len(parsed.Timings) > 0 {
			result["timings"] = parsed.Timings
		}
	}
	return result, nil
}

func nullStr(err error) any {
	if err != nil {
		return err.Error()
	}
	return nil
}
