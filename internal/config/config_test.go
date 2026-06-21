package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CaptainFallaway/Allower/internal/config"
)

func writeConfigFile(t *testing.T, contents string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	if err := os.WriteFile(path, []byte(strings.TrimSpace(contents)+"\n"), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	return path
}

func loadConfig(t *testing.T, contents string) *config.Config {
	t.Helper()

	path := writeConfigFile(t, contents)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("config.Load(%q) returned error: %v", path, err)
	}

	return cfg
}

func TestLoadAppliesDefaultEntrypointDurations(t *testing.T) {
	cfg := loadConfig(t, `
entrypoints:
  - name: defaults
    addr: 127.0.0.1:8080
    target: 10.0.0.1:80
rules: []
`)

	if got := cfg.Entrypoints[0].Keepalive.Duration; got != 2*time.Minute {
		t.Fatalf("default keepalive = %v, want 2m", got)
	}
	if got := cfg.Entrypoints[0].DialTimeout.Duration; got != 15*time.Second {
		t.Fatalf("default dial_timeout = %v, want 15s", got)
	}
}

func TestLoadNormalizesRangeTypes(t *testing.T) {
	cfg := loadConfig(t, `
entrypoints:
  - name: app
    addr: 127.0.0.1:8080
    target: 10.0.0.1:80
rules:
  - name: only_swedish
    ranges:
      - from: 85.24.194.40
        to: 85.24.194.42
      - prefix: 85.24.194.0/25
`)

	ranges := cfg.Rules[0].Ranges
	if got := ranges[0].Type; got != config.RangeTypeFromTo {
		t.Fatalf("range[0].Type = %v, want RangeTypeFromTo", got)
	}
	if got := ranges[0].From.String(); got != "85.24.194.40" {
		t.Fatalf("range[0].From = %q, want %q", got, "85.24.194.40")
	}
	if got := ranges[0].To.String(); got != "85.24.194.42" {
		t.Fatalf("range[0].To = %q, want %q", got, "85.24.194.42")
	}
	if got := ranges[1].Type; got != config.RangeTypePrefix {
		t.Fatalf("range[1].Type = %v, want RangeTypePrefix", got)
	}
	if got := ranges[1].Prefix.String(); got != "85.24.194.0/25" {
		t.Fatalf("range[1].Prefix = %q, want %q", got, "85.24.194.0/25")
	}
}

func TestLoadParsesConfigFields(t *testing.T) {
	cfg := loadConfig(t, `
entrypoints:
  - name: defaults
    addr: 127.0.0.1:8080
    target: 10.0.0.1:80
  - name: tuned
    addr: :9090
    keepalive: 45s
    target: 10.0.0.2:443
    dial_timeout: 2m
    rules: [only_swedish]
rules:
  - name: only_swedish
    allow:
      - 1.2.3.4
    block:
      - 5.6.7.8
    countries: [SE]
    continents: [EU]
    ass:
      - number: AS24429
        name: Taobao
        domain: alibabacloud.com
`)

	if got := cfg.Entrypoints[0].Addr; got != "127.0.0.1:8080" {
		t.Fatalf("entrypoint[0].Addr = %q, want %q", got, "127.0.0.1:8080")
	}
	if got := cfg.Entrypoints[1].Keepalive.Duration; got != 45*time.Second {
		t.Fatalf("entrypoint[1].Keepalive = %v, want 45s", got)
	}
	if got := cfg.Entrypoints[1].DialTimeout.Duration; got != 2*time.Minute {
		t.Fatalf("entrypoint[1].DialTimeout = %v, want 2m", got)
	}
	if got := cfg.Entrypoints[1].Rules; len(got) != 1 || got[0] != "only_swedish" {
		t.Fatalf("entrypoint[1].Rules = %#v, want [only_swedish]", got)
	}

	rule := cfg.Rules[0]
	if got := rule.Name; got != "only_swedish" {
		t.Fatalf("rule.Name = %q, want %q", got, "only_swedish")
	}
	if got := len(rule.Allow); got != 1 || rule.Allow[0].String() != "1.2.3.4" {
		t.Fatalf("rule.Allow = %#v, want [1.2.3.4]", rule.Allow)
	}
	if got := len(rule.Block); got != 1 || rule.Block[0].String() != "5.6.7.8" {
		t.Fatalf("rule.Block = %#v, want [5.6.7.8]", rule.Block)
	}
	if got := rule.Countries; len(got) != 1 || got[0] != "SE" {
		t.Fatalf("rule.Countries = %#v, want [SE]", got)
	}
	if got := rule.Continents; len(got) != 1 || got[0] != "EU" {
		t.Fatalf("rule.Continents = %#v, want [EU]", got)
	}
	if got := len(rule.ASs); got != 1 {
		t.Fatalf("len(rule.ASs) = %d, want 1", got)
	}
	if got := rule.ASs[0].Number; got != "AS24429" {
		t.Fatalf("rule.ASs[0].Number = %q, want %q", got, "AS24429")
	}
	if got := rule.ASs[0].Name; got != "Taobao" {
		t.Fatalf("rule.ASs[0].Name = %q, want %q", got, "Taobao")
	}
	if got := rule.ASs[0].Domain; got != "alibabacloud.com" {
		t.Fatalf("rule.ASs[0].Domain = %q, want %q", got, "alibabacloud.com")
	}
}

func TestLoadReturnsDecodeErrorForInvalidDuration(t *testing.T) {
	path := writeConfigFile(t, `
entrypoints:
  - name: app
    addr: 127.0.0.1:8080
    keepalive: not-a-duration
    target: 10.0.0.1:80
rules: []
`)

	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected an error")
	}
	if got := err.Error(); !strings.Contains(got, "failed to decode config file") {
		t.Fatalf("error = %q, want decode failure", got)
	}
}

func TestLoadValidationErrors(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{
			name: "invalid entrypoint addr",
			yaml: `
entrypoints:
  - name: app
    addr: 127.0.0.1
    target: 10.0.0.1:80
rules: []
`,
			want: "invalid entrypoint addr",
		},
		{
			name: "invalid entrypoint target",
			yaml: `
entrypoints:
  - name: app
    addr: 127.0.0.1:8080
    target: 10.0.0.1
rules: []
`,
			want: "invalid entrypoint target",
		},
		{
			name: "undefined rule reference",
			yaml: `
entrypoints:
  - name: app
    addr: 127.0.0.1:8080
    target: 10.0.0.1:80
    rules: [missing]
rules:
  - name: existing
`,
			want: `references undefined rule "missing"`,
		},
		{
			name: "invalid AS number prefix",
			yaml: `
entrypoints:
  - name: app
    addr: 127.0.0.1:8080
    target: 10.0.0.1:80
rules:
  - name: as
    ass:
      - number: 24429
`,
			want: "AS number must be in the format AS12345",
		},
		{
			name: "range with only from",
			yaml: `
entrypoints:
  - name: app
    addr: 127.0.0.1:8080
    target: 10.0.0.1:80
rules:
  - name: range
    ranges:
      - from: 10.0.0.1
`,
			want: "either from, to or a prefix must be defined",
		},
		{
			name: "range with no endpoints",
			yaml: `
entrypoints:
  - name: app
    addr: 127.0.0.1:8080
    target: 10.0.0.1:80
rules:
  - name: range
    ranges:
      - {}
`,
			want: "either prefix or from, to must be defined",
		},
		{
			name: "range with both prefix and from/to",
			yaml: `
entrypoints:
  - name: app
    addr: 127.0.0.1:8080
    target: 10.0.0.1:80
rules:
  - name: range
    ranges:
      - from: 10.0.0.1
        to: 10.0.0.2
        prefix: 10.0.0.0/24
`,
			want: "both prefix and from, to cannot be defined",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfigFile(t, tt.yaml)
			_, err := config.Load(path)
			if err == nil {
				t.Fatal("expected an error")
			}
			if got := err.Error(); !strings.Contains(got, tt.want) {
				t.Fatalf("error = %q, want substring %q", got, tt.want)
			}
		})
	}
}

func TestLoadCreatesTemplateWhenConfigMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")

	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected an error")
	}

	var created config.ErrCreatedConfigFile
	if !errors.As(err, &created) {
		t.Fatalf("expected ErrCreatedConfigFile, got %T: %v", err, err)
	}
	if created.Path != path {
		t.Fatalf("created path = %q, want %q", created.Path, path)
	}

	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("failed to read created config file: %v", readErr)
	}
	contents := string(data)
	if !strings.Contains(contents, "entrypoints:") {
		t.Fatalf("created config template missing entrypoints section: %q", contents)
	}
	if !strings.Contains(contents, "rules:") {
		t.Fatalf("created config template missing rules section: %q", contents)
	}
}
