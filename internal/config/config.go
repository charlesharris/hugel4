// Package config holds the garden's settings.
//
// JSON rather than TOML, for the same reason the pile is JSON: the shapes are
// small, and a dependency to read six keys costs more than it saves.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Config is the garden.
type Config struct {
	// Kin groups the names one project has been known by. A project renamed
	// across rewrites leaves knowledge filed under every name it ever had, and
	// soil that treats those as different beds penalises the oldest and most
	// settled decisions exactly because they are old.
	Kin map[string][]string `json:"kin,omitempty"`
}

// Home is the garden directory.
//
// Everything the garden keeps hangs off this one path, so it is also the one
// place a test can be stopped from reaching the gardener's real garden. See
// Sandbox.
func Home() (string, error) {
	if h := os.Getenv("HUGEL_HOME"); h != "" {
		return Sandbox(h), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home dir: %w", err)
	}
	return Sandbox(filepath.Join(home, ".hugel")), nil
}

func path() (string, error) {
	h, err := Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, "config.json"), nil
}

// Load reads the config. A missing file is an empty config, not an error: a
// garden works without one.
func Load() (*Config, error) {
	p, err := path()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", p, err)
	}
	return &c, nil
}

// Save writes the config.
func Save(c *Config) error {
	p, err := path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("create garden dir: %w", err)
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return os.WriteFile(p, append(b, '\n'), 0o644)
}

// KinOf returns every name a bed's project has been known by, including the bed
// itself. Membership is symmetric: asking from any name gives the whole group,
// so soil drawn in hugel4 reaches knowledge filed under hugel and vice versa.
func (c *Config) KinOf(bed string) []string {
	if bed == "" {
		return nil
	}
	group := map[string]bool{bed: true}
	for canonical, names := range c.Kin {
		all := append([]string{canonical}, names...)
		member := false
		for _, n := range all {
			if strings.EqualFold(n, bed) {
				member = true
				break
			}
		}
		if !member {
			continue
		}
		for _, n := range all {
			group[n] = true
		}
	}
	out := make([]string, 0, len(group))
	for n := range group {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// AddKin records that these names are the same project.
func (c *Config) AddKin(canonical string, names ...string) {
	if c.Kin == nil {
		c.Kin = map[string][]string{}
	}
	seen := map[string]bool{canonical: true}
	merged := append([]string{}, c.Kin[canonical]...)
	for _, n := range merged {
		seen[n] = true
	}
	for _, n := range names {
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		merged = append(merged, n)
	}
	sort.Strings(merged)
	c.Kin[canonical] = merged
}
