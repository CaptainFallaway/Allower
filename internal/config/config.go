package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/CaptainFallaway/Allower/pkg/hashset"
	"go.yaml.in/yaml/v4"
)

const configTemplate = `#Example config.yaml

entrypoints:
    - name: http
      addr: :80
      target: sweden.se:443
      rules: [only_swedish]

rules:
    - name: only_swedish
      countries: [SE]
`

func Load(path string) (*Config, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, createConfigFile(path)
	} else if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	decoder := yaml.NewDecoder(file)
	config := new(Config)

	if err := decoder.Decode(config); err != nil {
		return nil, fmt.Errorf("failed to decode config file: %w", err)
	}

	return config, validateConfig(config)
}

func validateConfig(c *Config) error {
	var errs []error

	// First let's check so the addr and target addr are properly formatted
	for _, entrypoint := range c.Entrypoints {
		if _, _, err := net.SplitHostPort(entrypoint.Addr); err != nil {
			errs = append(errs, fmt.Errorf("invalid entrypoint addr %q in %q: %w", entrypoint.Addr, entrypoint.Name, err))
		}
		if _, _, err := net.SplitHostPort(entrypoint.Target); err != nil {
			errs = append(errs, fmt.Errorf("invalid entrypoint target %q in %q: %w", entrypoint.Target, entrypoint.Name, err))
		}
	}

	ruleSet := hashset.New[string]()
	for _, rule := range c.Rules {
		ruleSet.Add(rule.Name)
	}

	for i := range c.Entrypoints {
		entrypoint := &c.Entrypoints[i]

		// Set keepalive and dial timeout to 30s if not set, as this is a reasonable default for most use cases
		if entrypoint.Keepalive.Duration == 0 {
			entrypoint.Keepalive.Duration = time.Second * 30
		}

		if entrypoint.DialTimeout.Duration == 0 {
			entrypoint.DialTimeout.Duration = time.Second * 30
		}

		// Check so all rules used by entrypoints are defined
		for _, ruleName := range entrypoint.Rules {
			if !ruleSet.Contains(ruleName) {
				errs = append(errs, fmt.Errorf("entrypoint %q references undefined rule %q", entrypoint.Name, ruleName))
			}
		}
	}

	for i := range c.Rules {
		rule := &c.Rules[i]

		// Check the AS numbers, they should be in the format AS12345
		for i, as := range rule.ASs {
			if strings.HasPrefix(as.Number, "AS") {
				errs = append(errs, fmt.Errorf("rule %q invalid AS no. %d: AS number must be in the format AS12345", rule.Name, i))
			}
		}

		// Check the range rules, either from and to must be defined or the range is only a prefix
		for i, r := range rule.Ranges {
			if !r.From.IsValid() && !r.To.IsValid() {
				if !r.Prefix.IsValid() {
					errs = append(errs, fmt.Errorf("rule %q invalid range no. %d for: either prefix or from, to must be defined for a range", rule.Name, i))
				}
				r.Type = RangeTypePrefix
				continue
			}
			if r.From.IsValid() && r.To.IsValid() {
				if r.Prefix.IsValid() {
					errs = append(errs, fmt.Errorf("rule %q invalid range no. %d: both prefix and from, to cannot be defined for a range", rule.Name, i))
				}
				r.Type = RangeTypeFromTo
				continue
			}
			errs = append(errs, fmt.Errorf("rule %q invalid range no. %d: either from, to or a prefix must be defined", rule.Name, i))
		}
	}

	return errors.Join(errs...)
}

func createConfigFile(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(configTemplate)
	if err != nil {
		return fmt.Errorf("failed to write config template: %w", err)
	}

	return ErrCreatedConfigFile{path}
}
