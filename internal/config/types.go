package config

import (
	"net/netip"
)

type Config struct {
	Entrypoints []Entrypoint `yaml:"entrypoints"`
	Rules       []Rule       `yaml:"rules"`
}

type Entrypoint struct {
	Name   string   `yaml:"name"`
	Addr   string   `yaml:"addr"`
	Target string   `yaml:"target"`
	Rules  []string `yaml:"rules"`
}

type Rule struct {
	Name       string       `yaml:"name"`
	Block      []netip.Addr `yaml:"block"`
	Allow      []netip.Addr `yaml:"allow"`
	Ranges     []Range      `yaml:"ranges"`
	ASs        []AS         `yaml:"ass"`
	Countries  []string     `yaml:"countries"`
	Continents []string     `yaml:"continents"`
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
