package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/fiatjaf/eventstore"
	eventstorelmdb "github.com/fiatjaf/eventstore/lmdb"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/nbd-wtf/go-nostr"
)

type Storage struct {
	db             eventstore.Store
	archiveEnabled bool
	analyticsDB    *sqlx.DB // Local analytics database (SQLite)
}

func New(path string, archiveEnabled bool) (*Storage, error) {
	return newLMDBStorage(path, archiveEnabled, 0)
}

func newLMDBStorage(path string, archiveEnabled bool, extraFlags uint) (*Storage, error) {
	db := &eventstorelmdb.LMDBBackend{
		Path: path,
	}

	if extraFlags != 0 {
		setLMDBExtraFlags(db, extraFlags)
	}

	if err := db.Init(); err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}

	storage := &Storage{db: db, archiveEnabled: archiveEnabled}

	analyticsPath := analyticsPathFor(path)
	if err := os.MkdirAll(filepath.Dir(analyticsPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create analytics directory: %w", err)
	}
	// Add connection parameters to prevent indefinite blocking on locks
	// _timeout: milliseconds to wait for locks before returning SQLITE_BUSY
	// _journal_mode=WAL: allows concurrent reads during writes
	// _synchronous=NORMAL: balance between safety and performance
	analyticsDB, err := sqlx.Open("sqlite3", analyticsPath+"?_timeout=5000&_journal_mode=WAL&_synchronous=NORMAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open analytics database: %w", err)
	}
	storage.analyticsDB = analyticsDB
	log.Printf("Connected to analytics database (SQLite): %s (timeout=5s, WAL mode)", analyticsPath)

	if archiveEnabled {
		log.Println("Event archiving enabled for kind:3 history")
	}

	return storage, nil
}

func analyticsPathFor(eventPath string) string {
	return filepath.Join(filepath.Dir(eventPath), "analytics.sqlite")
}

func (s *Storage) SaveEvent(ctx context.Context, evt *nostr.Event) error {
	if s.archiveEnabled && shouldArchiveKind(evt.Kind) {
		s.archiveOldVersion(ctx, evt)
	}
	start := time.Now()
	err := s.db.SaveEvent(ctx, evt)
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		log.Printf("SLOW db.SaveEvent: kind=%d tags=%d elapsed=%v", evt.Kind, len(evt.Tags), elapsed)
	}
	if err != nil {
		return err
	}

	return nil
}

func shouldArchiveKind(kind int) bool {
	return kind == 3
}

// archiveOldVersion archives the current version before replacement.
func (s *Storage) archiveOldVersion(ctx context.Context, newEvt *nostr.Event) {
	// Query for existing event
	start := time.Now()
	existing, err := s.QueryEvents(ctx, nostr.Filter{
		Kinds:   []int{newEvt.Kind},
		Authors: []string{newEvt.PubKey},
		Limit:   1,
	})
	if elapsed := time.Since(start); elapsed > 20*time.Millisecond {
		log.Printf("SLOW archiveOldVersion.QueryEvents: kind=%d elapsed=%v", newEvt.Kind, elapsed)
	}
	if err != nil || len(existing) == 0 {
		return
	}

	oldEvt := existing[0]
	// Only archive if old event is different and older
	if oldEvt.ID != newEvt.ID && oldEvt.CreatedAt < newEvt.CreatedAt {
		start = time.Now()
		s.ArchiveEvent(ctx, oldEvt)
		if elapsed := time.Since(start); elapsed > 20*time.Millisecond {
			log.Printf("SLOW ArchiveEvent: kind=%d tags=%d elapsed=%v", oldEvt.Kind, len(oldEvt.Tags), elapsed)
		}
	}
}

func (s *Storage) QueryEvents(ctx context.Context, filter nostr.Filter) ([]*nostr.Event, error) {
	// Add 5 second timeout to prevent query pile-up
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Use eventstore's native query capabilities
	ch, err := s.db.QueryEvents(ctx, filter)
	if err != nil {
		return nil, err
	}

	events := make([]*nostr.Event, 0)
	for evt := range ch {
		events = append(events, evt)
	}

	return events, nil
}

func (s *Storage) DeleteEvent(ctx context.Context, evt *nostr.Event) error {
	return s.db.DeleteEvent(ctx, evt)
}

func (s *Storage) CountEventsByKind(ctx context.Context, kind int) (int64, error) {
	// Use the optimized CountEvents method with a simple kind filter
	return s.CountEvents(ctx, nostr.Filter{Kinds: []int{kind}})
}

// GetEventCountsByKind returns counts for all kinds stored in the database
// Uses cached counts from SQLite only - returns empty if cache not populated
func (s *Storage) GetEventCountsByKind(ctx context.Context) (map[int]int64, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return make(map[int]int64), nil
	}

	rows, err := dbConn.QueryContext(ctx, "SELECT kind, event_count FROM cached_event_counts")
	if err != nil {
		// Table might not exist yet or query failed - return empty
		return make(map[int]int64), nil
	}
	defer rows.Close()

	result := make(map[int]int64)
	for rows.Next() {
		var kind int
		var count int64
		if err := rows.Scan(&kind, &count); err != nil {
			return nil, err
		}
		result[kind] = count
	}

	return result, rows.Err()
}

func (s *Storage) Close() {
	s.db.Close()
	if s.analyticsDB != nil {
		s.analyticsDB.Close()
	}
}

// EventStore returns the underlying eventstore.Store for direct access
func (s *Storage) EventStore() eventstore.Store {
	return s.db
}

// GetProfileNames returns a map of pubkey -> display name from kind:0 events
func (s *Storage) GetProfileNames(ctx context.Context, pubkeys []string) (map[string]string, error) {
	if len(pubkeys) == 0 {
		return make(map[string]string), nil
	}

	events, err := s.QueryEvents(ctx, nostr.Filter{
		Kinds:   []int{0},
		Authors: pubkeys,
	})
	if err != nil {
		return nil, err
	}

	names := make(map[string]string)
	for _, evt := range events {
		var profile struct {
			Name        string `json:"name"`
			DisplayName string `json:"display_name"`
		}
		if err := json.Unmarshal([]byte(evt.Content), &profile); err != nil {
			continue
		}
		name := profile.DisplayName
		if name == "" {
			name = profile.Name
		}
		if name != "" {
			names[evt.PubKey] = name
		}
	}

	return names, nil
}

type ProfileInfo struct {
	Name    string
	Picture string
}

// GetProfileInfo returns a map of pubkey -> ProfileInfo (name + picture) from kind:0 events
func (s *Storage) GetProfileInfo(ctx context.Context, pubkeys []string) (map[string]ProfileInfo, error) {
	if len(pubkeys) == 0 {
		return make(map[string]ProfileInfo), nil
	}

	events, err := s.QueryEvents(ctx, nostr.Filter{
		Kinds:   []int{0},
		Authors: pubkeys,
	})
	if err != nil {
		return nil, err
	}

	profiles := make(map[string]ProfileInfo)
	for _, evt := range events {
		var profile struct {
			Name        string `json:"name"`
			DisplayName string `json:"display_name"`
			Picture     string `json:"picture"`
		}
		if err := json.Unmarshal([]byte(evt.Content), &profile); err != nil {
			continue
		}
		name := profile.DisplayName
		if name == "" {
			name = profile.Name
		}
		profiles[evt.PubKey] = ProfileInfo{
			Name:    name,
			Picture: profile.Picture,
		}
	}

	return profiles, nil
}
