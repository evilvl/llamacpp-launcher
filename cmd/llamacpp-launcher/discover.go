package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// ModelInfo — информация о модели для списка в интерфейсе.
type ModelInfo struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	HasConfig bool   `json:"has_config"`
}

// discoverModels находит все .gguf файлы в MODEL_ROOT (рекурсивно), отсортирует по имени.
// Файлы вида *.gguf.part (незавершённая загрузка) пропускаются — как в configure-llama-cpp.
func discoverModels() ([]ModelInfo, error) {
	info, err := os.Stat(app.ModelRoot)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errNotDir
	}

	var models []ModelInfo
	err = filepath.Walk(app.ModelRoot, func(path string, fi os.FileInfo, e error) error {
		if e != nil || fi.IsDir() {
			return nil
		}
		name := fi.Name()
		if !strings.HasSuffix(name, ".gguf") {
			return nil
		}
		size := int64(0)
		if fi.Mode().IsRegular() {
			size = fi.Size()
		} else {
			if st, serr := os.Stat(path); serr == nil {
				size = st.Size()
			}
		}
		rel, _ := filepath.Rel(app.ModelRoot, path)
		models = append(models, ModelInfo{
			Path:      path,
			Name:      rel,
			Size:      size,
			HasConfig: fileExists(configPathFor(path)),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(models, func(i, j int) bool {
		if models[i].HasConfig != models[j].HasConfig {
			return models[i].HasConfig
		}
		return models[i].Name < models[j].Name
	})
	return models, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// humanSize форматирует размер в человек-читаемый вид (аналог human_size из bash).
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return strconv.FormatInt(n, 10) + " B"
	}
	div, exp := int64(unit), 0
	for d := n / unit; d >= unit; d /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
