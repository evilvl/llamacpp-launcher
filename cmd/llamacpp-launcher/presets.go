package main

// Preset is a named template of llama-server flags applied to any model.
type Preset struct {
	Name  string            `json:"name"`  // technical name (id), e.g. "fast-streaming"
	Title string            `json:"title"` // human-readable name, e.g. "Fast streaming"
	Desc  string            `json:"desc"`  // short description of the scenario
	Flags map[string]string `json:"flags"` // flag values (canonical names)
}

// presets are the built-in startup scenarios. The values are tuned for popular
// use cases; the user picks the model and hardware themselves.
var presets = []Preset{
	{
		Name:  "fast-streaming",
		Title: "Fast streaming",
		Desc:  "Low latency, word-level streaming: small context, many GPU layers, high parallel.",
		Flags: map[string]string{
			"--ctx-size":   "4096",
			"--gpu-layers": "33",
			"--parallel":   "8",
			"--n-batch":    "512",
			"--flash-attn": "on",
		},
	},
	{
		Name:  "large-context",
		Title: "Large context (32K)",
		Desc:  "Wide context for long documents: flash-attn + fit to VRAM.",
		Flags: map[string]string{
			"--ctx-size":   "32768",
			"--gpu-layers": "24",
			"--flash-attn": "on",
			"--fit":        "on",
			"--fit-ctx":    "32768",
			"--numa":       "distribute",
		},
	},
	{
		Name:  "cpu-only",
		Title: "CPU only",
		Desc:  "No GPU: maximum CPU threads for inference on the processor.",
		Flags: map[string]string{
			"--gpu-layers": "0",
			"--numa":       "off",
			"--n-batch":    "1024",
		},
	},
	{
		Name:  "vram-friendly",
		Title: "VRAM fit",
		Desc:  "Limited VRAM: fit to target with flash-attn, moderate number of layers.",
		Flags: map[string]string{
			"--fit":        "on",
			"--fit-target": "256",
			"--fit-ctx":    "256000",
			"--gpu-layers": "20",
			"--flash-attn": "on",
		},
	},
}

// findPreset finds a preset by its technical name.
func findPreset(name string) *Preset {
	for i := range presets {
		if presets[i].Name == name {
			return &presets[i]
		}
	}
	return nil
}

// apply applies the preset flags to the config (overriding the current values).
func (p *Preset) apply(c *Config) {
	for k, v := range p.Flags {
		c.Flags[k] = v
	}
}
