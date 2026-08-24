package main

import (
	"os/exec"
	"regexp"
	"sort"
	"strings"
)

// Kind classifies the flag type for building the form.
type Kind string

const (
	KindToggle Kind = "toggle" // switch, no argument (--perf/--no-perf)
	KindEnum   Kind = "enum"   // choose from a list ([on|off] or {a,b,c})
	KindValue  Kind = "value"  // takes an arbitrary value (N, TYPE, FNAME...)
)

// FlagDef describes one llama-server argument (from --help).
type FlagDef struct {
	Canonical string   `json:"name"`    // main name, e.g. --gpu-layers
	Aliases   []string `json:"aliases"` // all short/long names
	Kind      Kind     `json:"kind"`
	Choices   []string `json:"choices,omitempty"`
	Default   string   `json:"default"`
	Desc      string   `json:"desc"`
}

var (
	reDefault = regexp.MustCompile(`\(default: ([^)]+?)\)`)
	// Token option: -t, --threads, --no-perf, etc.
	reOpt = regexp.MustCompile(`^-+[a-z0-9][a-z0-9-]*$`)
)

// knownRange describes `lo-hi`-shaped arguments that do not start with an uppercase letter or parenthesis.
var knownRange = map[string]bool{"lo-hi": true}

func isOpt(tok string) bool { return reOpt.MatchString(tok) }

// parseFlagLine parses one help line with the flag description.
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

	// Canonical is the first long alias (after --).
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

	// Enum (on|off / {a,b,c}) is checked before free values: isArgToken
	// treats [...] and {...} as value tokens, otherwise the enum would be lost.
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

	// Description: everything after the argument (or after the names).
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

	// Options are separated by either | or , depending on the --help build.
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

// parseHelp parses the full --help output, discarding section header lines.
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

// loadFlags loads the flag list from the llama-server --help output (if the binary is available).
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

// sortFlags sorts flags by name for deterministic output.
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
