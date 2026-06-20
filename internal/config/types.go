package config

import (
	"net/netip"
)

type Config struct {
	Entrypoints []Entrypoint `yaml:"entrypoints"`
	Rules       []Rule       `yaml:"rules"`
}

// Entrypoint defines a HTTP or TCP entrypoint for the proxy server.
type Entrypoint struct {
	Name   string   `yaml:"name"`
	Addr   string   `yaml:"addr"`
	Target string   `yaml:"target"`
	Rules  []string `yaml:"rules"`
}

// Rule defines a set of conditions and actions for traffic management.
type Rule struct {
	Name       string       `yaml:"name"`
	Block      []netip.Addr `yaml:"block"`
	Allow      []netip.Addr `yaml:"allow"`
	Ranges     []Range      `yaml:"ranges"`
	ASNumbers  []string     `yaml:"as_numbers"`
	Countries  []string     `yaml:"countries"`
	Continents []string     `yaml:"continents"`
}

type Range struct {
	From netip.Addr `yaml:"from"`
	To   netip.Addr `yaml:"to"`
}
