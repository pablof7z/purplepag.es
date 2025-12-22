package config

import (
	"encoding/json"
	"os"
)

type RelayInfo struct {
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Pubkey        string   `json:"pubkey"`
	Contact       string   `json:"contact"`
	Icon          string   `json:"icon"`
	SupportedNIPs []int    `json:"supported_nips"`
	Software      string   `json:"software"`
	Version       string   `json:"version"`
}

type ServerConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type StorageConfig struct {
	Backend string `json:"backend"`
	Path    string `json:"path"`
}

type SyncConfig struct {
	Enabled bool     `json:"enabled"`
	Relays  []string `json:"relays"`
	Kinds   []int    `json:"kinds"`
}

type ProfileHydrationConfig struct {
	Enabled          bool `json:"enabled"`
	MinFollowers     int  `json:"min_followers"`
	RetryAfterHours  int  `json:"retry_after_hours"`
	IntervalMinutes  int  `json:"interval_minutes"`
	BatchSize        int  `json:"batch_size"`
}

type LimitsConfig struct {
	MaxSubscriptions int `json:"max_subscriptions"`
	MaxFilters       int `json:"max_filters"`
	MaxLimit         int `json:"max_limit"`
	MaxEventTags     int `json:"max_event_tags"`
	MaxContentLength int `json:"max_content_length"`
}

type Config struct {
	Relay            RelayInfo              `json:"relay"`
	Server           ServerConfig           `json:"server"`
	Storage          StorageConfig          `json:"storage"`
	AllowedKinds     []int                  `json:"allowed_kinds"`
	Sync             SyncConfig             `json:"sync"`
	ProfileHydration ProfileHydrationConfig `json:"profile_hydration"`
	Limits           LimitsConfig           `json:"limits"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Set defaults for profile hydration
	if cfg.ProfileHydration.MinFollowers == 0 {
		cfg.ProfileHydration.MinFollowers = 10
	}
	if cfg.ProfileHydration.RetryAfterHours == 0 {
		cfg.ProfileHydration.RetryAfterHours = 24
	}
	if cfg.ProfileHydration.IntervalMinutes == 0 {
		cfg.ProfileHydration.IntervalMinutes = 5
	}
	if cfg.ProfileHydration.BatchSize == 0 {
		cfg.ProfileHydration.BatchSize = 50
	}

	// Set defaults for limits
	if cfg.Limits.MaxSubscriptions == 0 {
		cfg.Limits.MaxSubscriptions = 50
	}
	if cfg.Limits.MaxFilters == 0 {
		cfg.Limits.MaxFilters = 10
	}
	if cfg.Limits.MaxLimit == 0 {
		cfg.Limits.MaxLimit = 2000
	}
	if cfg.Limits.MaxEventTags == 0 {
		cfg.Limits.MaxEventTags = 2000
	}
	if cfg.Limits.MaxContentLength == 0 {
		cfg.Limits.MaxContentLength = 131072
	}

	return &cfg, nil
}

func (c *Config) IsKindAllowed(kind int) bool {
	for _, k := range c.AllowedKinds {
		if k == kind {
			return true
		}
	}
	return false
}
