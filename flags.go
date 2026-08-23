package main

import (
	"os/exec"
	"regexp"
	"sort"
	"strings"
)

// Kind классифицирует тип флага для построения формы.
type Kind string

const (
	KindToggle Kind = "toggle" // переключатель, без аргумента (--perf/--no-perf)
	KindEnum   Kind = "enum"   // выбор из списка ([on|off] или {a,b,c})
	KindValue  Kind = "value"  // принимает произвольное значение (N, TYPE, FNAME...)
)

// FlagDef — описание одного аргумента llama-server (из --help).
type FlagDef struct {
	Canonical string   `json:"name"`   // основное имя, например --gpu-layers
	Aliases   []string `json:"aliases"` // все короткие/длинные имена
	Kind      Kind     `json:"kind"`
	Choices   []string `json:"choices,omitempty"`
	Default   string   `json:"default"`
	Desc      string   `json:"desc"`
}

var (
	reDefault = regexp.MustCompile(`\(default: ([^)]+?)\)`)
	// Токен-опция: -t, --threads, --no-perf и т.п.
	reOpt = regexp.MustCompile(`^-+[a-z0-9][a-z0-9-]*$`)
)

// knownRange — аргументы вида `lo-hi`, которые не начинаются с заглавной/скобки.
var knownRange = map[string]bool{"lo-hi": true}

func isOpt(tok string) bool { return reOpt.MatchString(tok) }

// parseFlagLine разбирает одну строку help'а с описанием флага.
func parseFlagLine(line string) (*FlagDef, bool) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return nil, false
	}

	var names []string
	i := 0
	for i < len(fields) {
		tok := strings.TrimRight(fields[i], ",")
		if !isOpt(tok) {
			break
		}
		names = append(names, tok)
		i++
	}
	if len(names) == 0 {
		return nil, false
	}

	// Canonical — первый длинный alias (после --).
	canonical := ""
	for _, n := range names {
		if strings.HasPrefix(n, "--") && canonical == "" {
			canonical = n
		}
	}
	if canonical == "" {
		canonical = names[0]
	}

	flag := &FlagDef{Canonical: canonical, Aliases: names}

	// Enum (on|off / {a,b,c}) проверяем до свободных значений: isArgToken
	// считает [...] и {...} токенами значений, иначе enum затерялся бы.
	if i < len(fields) {
		arg := fields[i]
		if strings.HasPrefix(arg, "[") || strings.HasPrefix(arg, "{") {
			flag.Kind = KindEnum
			flag.Choices = extractChoices(arg)
		} else if isArgToken(arg) {
			flag.Kind = KindValue
		} else {
			flag.Kind = KindToggle
		}
	} else {
		flag.Kind = KindToggle
	}

	// Default.
	if m := reDefault.FindStringSubmatch(line); m != nil {
		flag.Default = strings.Trim(m[1], `"'`)
	}

	// Описание — всё, что после аргумента (или после имён).
	descStart := i
	if flag.Kind != KindToggle {
		descStart = i + 1
	}
	desc := strings.Join(fields[descStart:], " ")
	flag.Desc = strings.TrimSpace(desc)
	if len(flag.Desc) > 160 {
		flag.Desc = flag.Desc[:160] + "…"
	}
	return flag, true
}

func isArgToken(tok string) bool {
	if strings.HasPrefix(tok, "[") || strings.HasPrefix(tok, "{") || strings.HasPrefix(tok, "<") {
		return true
	}
	if knownRange[tok] {
		return true
	}
	return regexp.MustCompile(`^[A-Z][A-Z0-9_.]*$`).MatchString(tok)
}

func extractChoices(arg string) []string {
	inner := arg
	inner = strings.TrimPrefix(inner, "[")
	inner = strings.TrimPrefix(inner, "{")
	inner = strings.TrimSuffix(inner, "]")
	inner = strings.TrimSuffix(inner, "}")

	// Варианты разделяются либо |, либо , — зависит от сборки --help.
	parts := strings.Split(inner, "|")
	if len(parts) == 1 {
		parts = strings.Split(inner, ",")
	}
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// parseHelp разбирает полный вывод --help, отбрасывая строки секций.
func parseHelp(output string) []FlagDef {
	var flags []FlagDef
	seen := map[string]bool{}
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "----- ") {
			continue
		}
		f, ok := parseFlagLine(line)
		if !ok || f == nil {
			continue
		}
		if seen[f.Canonical] {
			continue
		}
		seen[f.Canonical] = true
		flags = append(flags, *f)
	}
	return flags
}

// loadFlags загружает список флагов из вывода llama-server --help (если бинарник доступен).
func loadFlags() []FlagDef {
	if app.LlamaServer == "" {
		return nil
	}
	out, err := exec.Command(app.LlamaServer, "--help").CombinedOutput()
	if err != nil {
		return nil
	}
	return parseHelp(string(out))
}

// sortFlags сортирует флаги по имени для детерминированного вывода.
func sortFlags(flags []FlagDef) []FlagDef {
	out := make([]FlagDef, len(flags))
	copy(out, flags)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Canonical != out[j].Canonical {
			return out[i].Canonical < out[j].Canonical
		}
		return false
	})
	return out
}
