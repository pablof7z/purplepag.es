package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
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
	Backend        string `json:"backend"`
	Path           string `json:"path"`
	ArchiveEnabled *bool  `json:"archive_enabled"`
	AnalyticsDBURL string `json:"analytics_db_url"` // Optional: separate PostgreSQL for analytics
}

type SyncConfig struct {
	Enabled bool     `json:"enabled"`
	Relays  []string `json:"relays"`
	Kinds   []int    `json:"kinds"`
}

type ProfileHydrationConfig struct {
	Enabled         bool `json:"enabled"`
	MinFollowers    int  `json:"min_followers"`
	RetryAfterHours int  `json:"retry_after_hours"`
	IntervalMinutes int  `json:"interval_minutes"`
	BatchSize       int  `json:"batch_size"`
}

type TrustedSyncConfig struct {
	Disabled        bool  `json:"disabled"` // disabled instead of enabled, so default (false) means enabled
	IntervalMinutes int   `json:"interval_minutes"`
	BatchSize       int   `json:"batch_size"`
	Kinds           []int `json:"kinds"`
	TimeoutSeconds  int   `json:"timeout_seconds"`
}

type LimitsConfig struct {
	MaxSubscriptions    int `json:"max_subscriptions"`
	MaxFilters          int `json:"max_filters"`
	MaxLimit            int `json:"max_limit"`
	MaxEventTags        int `json:"max_event_tags"`
	MaxContentLength    int `json:"max_content_length"`
	EventsPerDayLimit   int `json:"events_per_day_limit"`
	MinTrustedFollowers int `json:"min_trusted_followers"`
}

// KindRange represents either a single kind or a range of kinds
type KindRange struct {
	Start int
	End   int // same as Start for single kinds
}

// KindSet holds both individual kinds and ranges for efficient matching
type KindSet struct {
	kinds  map[int]bool
	ranges []KindRange
}

func (ks *KindSet) UnmarshalJSON(data []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	ks.kinds = make(map[int]bool)
	ks.ranges = nil

	for _, item := range raw {
		// Try as integer first
		var kind int
		if err := json.Unmarshal(item, &kind); err == nil {
			ks.kinds[kind] = true
			continue
		}

		// Try as string range (e.g., "10000-19999")
		var rangeStr string
		if err := json.Unmarshal(item, &rangeStr); err == nil {
			parts := strings.Split(rangeStr, "-")
			if len(parts) != 2 {
				return fmt.Errorf("invalid kind range format: %s (expected 'start-end')", rangeStr)
			}

			start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
			if err != nil {
				return fmt.Errorf("invalid range start: %s", parts[0])
			}

			end, err := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err != nil {
				return fmt.Errorf("invalid range end: %s", parts[1])
			}

			if start > end {
				return fmt.Errorf("invalid range: start (%d) > end (%d)", start, end)
			}

			ks.ranges = append(ks.ranges, KindRange{Start: start, End: end})
			continue
		}

		return fmt.Errorf("allowed_kinds must contain integers or range strings like \"10000-19999\"")
	}

	return nil
}

func (ks *KindSet) Contains(kind int) bool {
	if ks.kinds[kind] {
		return true
	}
	for _, r := range ks.ranges {
		if kind >= r.Start && kind <= r.End {
			return true
		}
	}
	return false
}

// ToSlice returns all explicit kinds (not ranges) for backwards compatibility
func (ks *KindSet) ToSlice() []int {
	result := make([]int, 0, len(ks.kinds))
	for k := range ks.kinds {
		result = append(result, k)
	}
	return result
}

// IsEmpty returns true if no kinds are configured
func (ks *KindSet) IsEmpty() bool {
	return len(ks.kinds) == 0 && len(ks.ranges) == 0
}

type Config struct {
	Relay            RelayInfo              `json:"relay"`
	Server           ServerConfig           `json:"server"`
	Storage          StorageConfig          `json:"storage"`
	AllowedKinds     KindSet                `json:"allowed_kinds"`
	SyncKinds        []int                  `json:"sync_kinds"`
	Sync             SyncConfig             `json:"sync"`
	ProfileHydration ProfileHydrationConfig `json:"profile_hydration"`
	TrustedSync      TrustedSyncConfig      `json:"trusted_sync"`
	Limits           LimitsConfig           `json:"limits"`
	StatsPassword    string                 `json:"stats_password"`
}

// DefaultSyncKinds returns the default kinds to sync (NIP-51 lists + profiles)
func DefaultSyncKinds() []int {
	return []int{
		0,     // Profiles
		3,     // Follow list
		10000, // Mute list
		10001, // Pinned notes
		10002, // Relay list
		10003, // Bookmarks
		10004, // Communities
		10005, // Public chats
		10006, // Blocked relays
		10007, // Search relays
		10009, // Simple groups
		10015, // Interests
		10030, // Emojis
		10050, // DM relays
	}
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

	// Set defaults for sync kinds
	if len(cfg.SyncKinds) == 0 {
		cfg.SyncKinds = DefaultSyncKinds()
	}

	// Set default for storage archiving (enabled by default)
	if cfg.Storage.ArchiveEnabled == nil {
		defaultTrue := true
		cfg.Storage.ArchiveEnabled = &defaultTrue
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

	// Set defaults for trusted sync
	if cfg.TrustedSync.IntervalMinutes == 0 {
		cfg.TrustedSync.IntervalMinutes = 30
	}
	if cfg.TrustedSync.BatchSize == 0 {
		cfg.TrustedSync.BatchSize = 50
	}
	if len(cfg.TrustedSync.Kinds) == 0 {
		cfg.TrustedSync.Kinds = cfg.SyncKinds
	}
	if cfg.TrustedSync.TimeoutSeconds == 0 {
		cfg.TrustedSync.TimeoutSeconds = 30
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
	if cfg.Limits.EventsPerDayLimit == 0 {
		cfg.Limits.EventsPerDayLimit = 50000
	}
	if cfg.Limits.MinTrustedFollowers == 0 {
		cfg.Limits.MinTrustedFollowers = 1000
	}

	return &cfg, nil
}

func (c *Config) IsKindAllowed(kind int) bool {
	return c.AllowedKinds.Contains(kind)
}
