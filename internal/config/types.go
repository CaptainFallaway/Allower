package config

import (
	"net/netip"
	"time"

	"go.yaml.in/yaml/v4"
)

type Config struct {
	Entrypoints []Entrypoint `yaml:"entrypoints"`
	Rules       []Rule       `yaml:"rules"`
}

type Entrypoint struct {
	Name        string   `yaml:"name"`
	Addr        string   `yaml:"addr"`
	Keepalive   Duration `yaml:"keepalive"`
	Target      string   `yaml:"target"`
	DialTimeout Duration `yaml:"dial_timeout"`
	Rules       []string `yaml:"rules"`
}

type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	var s string
	if err := node.Decode(&s); err != nil {
		return err
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	d.Duration = v
	return nil
}

type Rule struct {
	Name       string       `yaml:"name"`
	Block      []netip.Addr `yaml:"block"`
	Allow      []netip.Addr `yaml:"allow"`
	Ranges     []Range      `yaml:"ranges"`
	Countries  []string     `yaml:"countries"`
	Continents []string     `yaml:"continents"`
	ASs        []AS         `yaml:"ass"`
}

type RangeType int

const (
	RangeTypePrefix RangeType = iota
	RangeTypeFromTo
)

type Range struct {
	Type   RangeType    // Type of the range: Prefix or FromTo set
	From   netip.Addr   `yaml:"from"`
	To     netip.Addr   `yaml:"to"`
	Prefix netip.Prefix `yaml:"prefix"`
}

type AS struct {
	Number string `yaml:"number"`
	Name   string `yaml:"name"`
	Domain string `yaml:"domain"`
}
