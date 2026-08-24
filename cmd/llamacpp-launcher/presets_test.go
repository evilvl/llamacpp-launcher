package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFormatFlagValue(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{"on", "on"},
		{8080.0, "8080"},
		{3.14, "3.14"},
		{true, "true"},
		{false, "false"},
		{nil, ""},
	}
	for _, c := range cases {
		if got := formatFlagValue(c.in); got != c.want {
			t.Errorf("formatFlagValue(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPresetsNonEmpty(t *testing.T) {
	if len(presets) == 0 {
		t.Fatalf("presets is empty")
	}
	seen := map[string]bool{}
	for _, p := range presets {
		if p.Name == "" {
			t.Errorf("preset with empty name")
		}
		if seen[p.Name] {
			t.Errorf("duplicate preset name %q", p.Name)
		}
		seen[p.Name] = true
		if p.Title == "" || p.Desc == "" {
			t.Errorf("preset %q missing title/desc", p.Name)
		}
		if len(p.Flags) == 0 {
			t.Errorf("preset %q has no flags", p.Name)
		}
		for k, v := range p.Flags {
			if k == "" || v == "" {
				t.Errorf("preset %q has empty flag %q=%q", p.Name, k, v)
			}
		}
	}
}

func TestFindPreset(t *testing.T) {
	if findPreset("fast-streaming") == nil {
		t.Fatalf("expected to find fast-streaming")
	}
	if findPreset("fast-streaming").Title == "" {
		t.Fatalf("found preset has empty title")
	}
	if findPreset("does-not-exist") != nil {
		t.Fatalf("expected nil for unknown preset")
	}
}

func TestPresetApply(t *testing.T) {
	p := findPreset("cpu-only")
	if p == nil {
		t.Fatalf("cpu-only preset missing")
	}
	cfg := defaultConfig("/opt/models/m.gguf")
	p.apply(cfg)
	if cfg.Flags["--gpu-layers"] != "0" {
		t.Fatalf("apply did not set --gpu-layers=0, got %q", cfg.Flags["--gpu-layers"])
	}
	if cfg.Flags["--numa"] != "off" {
		t.Fatalf("apply did not set --numa=off, got %q", cfg.Flags["--numa"])
	}
}

func TestHandlePresets(t *testing.T) {
	rec := httptest.NewRecorder()
	handlePresets(rec, httptest.NewRequest(http.MethodGet, "/api/presets", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	var got []Preset
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != len(presets) {
		t.Fatalf("got %d presets, want %d", len(got), len(presets))
	}
}
