package main

/* Config */

type CommandConfig struct{}

type CacheConfig struct {
	MaxEntries int `yaml:"max_entries"`
}

type Config struct {
	// List of commands to memoize
	Commands map[string]CommandConfig `yaml:"memoize_commands"`
	Cache    CacheConfig              `yaml:"cache"`
}
