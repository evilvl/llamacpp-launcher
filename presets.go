package main

// Preset — именованный шаблон флагов llama-server, применяемый к любой модели.
type Preset struct {
	Name  string            `json:"name"`  // техническое имя (id), напр. "fast-streaming"
	Title string            `json:"title"` // человечное имя, напр. "Fast streaming"
	Desc  string            `json:"desc"`  // краткое описание сценария
	Flags map[string]string `json:"flags"` // значения флагов (канонические имена)
}

// presets — встроенные сценарии запуска. Значения подобраны под популярные
// кейсы; модель и железо пользователь подбирает сам.
var presets = []Preset{
	{
		Name:  "fast-streaming",
		Title: "Fast streaming",
		Desc:  "Низкая задержка, стриминг на слове: малый контекст, много слоёв на GPU, высокий parallel.",
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
		Desc:  "Широкий контекст для длинных документов: flash-attn + fit по VRAM.",
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
		Desc:  "Без GPU: максимум потоков CPU для инференса на процессоре.",
		Flags: map[string]string{
			"--gpu-layers": "0",
			"--numa":       "off",
			"--n-batch":    "1024",
		},
	},
	{
		Name:  "vram-friendly",
		Title: "VRAM fit",
		Desc:  "Ограниченная VRAM: fit к таргету с flash-attn, умеренное число слоёв.",
		Flags: map[string]string{
			"--fit":        "on",
			"--fit-target": "256",
			"--fit-ctx":    "256000",
			"--gpu-layers": "20",
			"--flash-attn": "on",
		},
	},
}

// findPreset находит пресет по техническому имени.
func findPreset(name string) *Preset {
	for i := range presets {
		if presets[i].Name == name {
			return &presets[i]
		}
	}
	return nil
}

// apply применяет флаги пресета к конфигу (переопределяет текущие значения).
func (p *Preset) apply(c *Config) {
	for k, v := range p.Flags {
		c.Flags[k] = v
	}
}
