package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	b, err := uiFS.ReadFile("ui/index.html")
	if err != nil {
		http.Error(w, "ui missing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(b)
}

func handleModels(w http.ResponseWriter, r *http.Request) {
	models, err := discoverModels()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if models == nil {
		models = []ModelInfo{}
	}
	writeJSON(w, map[string]any{"models": models})
}

type configResponse struct {
	Model  string            `json:"model"`
	Flags  map[string]string `json:"flags"`
	Extra  string            `json:"extra"`
}

func handleFlags(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, sortFlags(appFlags))
}

func handlePresets(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, presets)
}

func handleConfigGet(w http.ResponseWriter, r *http.Request) {
	model := r.URL.Query().Get("model")
	if model == "" {
		writeError(w, http.StatusBadRequest, "missing model")
		return
	}
	cfg, _ := parseConfigFile(configPathFor(model), model)

	flags := map[string]string{}
	for _, f := range appFlags {
		if v, ok := cfg.Flags[f.Canonical]; ok {
			flags[f.Canonical] = v
		} else {
			flags[f.Canonical] = f.Default
		}
	}
	// Сохраняем произвольные ключи, которых нет в списке флагов (например __extra__).
	for k, v := range cfg.Flags {
		if _, ok := flags[k]; !ok {
			flags[k] = v
		}
	}

	writeJSON(w, configResponse{Model: model, Flags: flags, Extra: cfg.Flags["__extra__"]})
}

func handleConfigSave(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model string         `json:"model"`
		Flags map[string]any `json:"flags"`
		Extra string         `json:"extra"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if req.Model == "" {
		writeError(w, http.StatusBadRequest, "missing model")
		return
	}
	cfg := defaultConfig(req.Model)
	for k, v := range req.Flags {
		cfg.Flags[k] = formatFlagValue(v)
	}
	if strings.TrimSpace(req.Extra) != "" {
		cfg.Flags["__extra__"] = req.Extra
	}
	if err := cfg.save(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true, "path": configPathFor(cfg.Model)})
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, status())
}

func handleStart(w http.ResponseWriter, r *http.Request) {
	model := r.URL.Query().Get("model")
	if model == "" {
		writeError(w, http.StatusBadRequest, "missing model")
		return
	}
	st, err := startService(model)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, st)
}

func handleLogs(w http.ResponseWriter, r *http.Request) {
	n := 100
	if v := r.URL.Query().Get("lines"); v != "" {
		if nv, err := strconv.Atoi(v); err == nil && nv > 0 {
			n = nv
		}
	}
	out, err := readLogs(n)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, map[string]any{"logs": out})
}

func handleHealthTest(w http.ResponseWriter, r *http.Request) {
	model := r.URL.Query().Get("model")
	if model == "" {
		writeError(w, http.StatusBadRequest, "missing model")
		return
	}
	res, err := inferenceTest(model)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, res)
}
