package config

import (
	"errors"
	"fmt"
	"net"
	"os"

	"go.yaml.in/yaml/v4"
)

const configTemplate = `#Example config.yaml

entrypoints:
    - name: http
      addr: :80
      target: example.com:443
      rules: [only_swedish]

rules:
    - name: only_swedish
      countries: [SE]
`

var ErrCreatedFile = errors.New("config.yaml has been created, please fill in the content and restart the program")

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

	// Then let's check so all rules used by entrypoints are defined
	ruleMap := make(map[string]struct{})
	for _, rule := range c.Rules {
		ruleMap[rule.Name] = struct{}{}
	}

	for _, entrypoint := range c.Entrypoints {
		for _, ruleName := range entrypoint.Rules {
			if _, ok := ruleMap[ruleName]; !ok {
				errs = append(errs, fmt.Errorf("entrypoint %q references undefined rule %q", entrypoint.Name, ruleName))
			}
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

	return ErrCreatedFile
}
