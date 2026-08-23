package main

import (
	"testing"
)

func TestParseFlagLineKinds(t *testing.T) {
	cases := []struct {
		line        string
		canonical   string
		kind        Kind
		wantChoices []string
		wantDefault string
	}{
		{"-fa,   --flash-attn [on|off|auto]       set Flash Attention use ... (default: 'auto')",
			"--flash-attn", KindEnum, []string{"on", "off", "auto"}, "auto"},
		{"-t,    --threads N                      number of CPU threads ... (default: -1)",
			"--threads", KindValue, nil, "-1"},
		{"-ctk,  --cache-type-k TYPE              KV cache data type for K ... (default: f16)",
			"--cache-type-k", KindValue, nil, "f16"},
		{"--perf, --no-perf                       whether to enable ... (default: false)",
			"--perf", KindToggle, nil, "false"},
		{"-ngl,  --gpu-layers, --n-gpu-layers N   max. number of layers ... (default: auto)",
			"--gpu-layers", KindValue, nil, "auto"},
	}
	for _, c := range cases {
		f, ok := parseFlagLine(c.line)
		if !ok {
			t.Errorf("line did not parse: %q", c.line)
			continue
		}
		if f.Canonical != c.canonical {
			t.Errorf("canonical = %q, want %q", f.Canonical, c.canonical)
		}
		if f.Kind != c.kind {
			t.Errorf("kind = %q, want %q (line=%q)", f.Kind, c.kind, c.line)
		}
		if c.wantChoices != nil {
			if len(f.Choices) != len(c.wantChoices) {
				t.Errorf("choices = %v, want %v", f.Choices, c.wantChoices)
			} else {
				for i := range c.wantChoices {
					if f.Choices[i] != c.wantChoices[i] {
						t.Errorf("choices = %v, want %v", f.Choices, c.wantChoices)
						break
					}
				}
			}
		}
		if f.Default != c.wantDefault {
			t.Errorf("default = %q, want %q", f.Default, c.wantDefault)
		}
	}
}

func TestExtractChoices(t *testing.T) {
	got := extractChoices("[on|off|auto]")
	want := []string{"on", "off", "auto"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}

	got = extractChoices("{none,linear,yarn}")
	want = []string{"none", "linear", "yarn"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("pipe-choices got %v want %v", got, want)
		}
	}
}

func TestParseHelpSkipsSectionsAndDedups(t *testing.T) {
	help := `----- common params -----

-t,    --threads N   number of CPU threads ... (default: -1)
--perf, --no-perf    whether to enable ... (default: false)

----- sampling params -----

-n,    --n-predict N number of tokens ... (default: -1)
`
	flags := parseHelp(help)
	if len(flags) != 3 {
		t.Fatalf("want 3 flags, got %d: %+v", len(flags), flags)
	}
	// section headers must not leak as flags
	for _, f := range flags {
		if f.Canonical == "" {
			t.Errorf("empty canonical in %+v", f)
		}
	}
	byKey := map[string]Kind{}
	for _, f := range flags {
		byKey[f.Canonical] = f.Kind
	}
	if byKey["--threads"] != KindValue || byKey["--perf"] != KindToggle || byKey["--n-predict"] != KindValue {
		t.Errorf("kinds wrong: %+v", byKey)
	}
}

func TestSortFlagsDeterministic(t *testing.T) {
	flags := []FlagDef{
		{Canonical: "--zeta"},
		{Canonical: "--alpha"},
		{Canonical: "--mu"},
	}
	sorted := sortFlags(flags)
	want := []string{"--alpha", "--mu", "--zeta"}
	for i, w := range want {
		if sorted[i].Canonical != w {
			t.Fatalf("sorted[%d]=%q want %q (got %v)", i, sorted[i].Canonical, w, sorted)
		}
	}
}

func TestIsArgToken(t *testing.T) {
	if !isArgToken("N") || !isArgToken("TYPE") || !isArgToken("[on|off]") ||
		!isArgToken("{a,b}") || !isArgToken("<x>") || !isArgToken("lo-hi") {
		t.Errorf("false negatives from isArgToken")
	}
	if isArgToken("whether") || isArgToken("number") {
		t.Errorf("true positives from isArgToken")
	}
}
