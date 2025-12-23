package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/fiatjaf/eventstore"
	"github.com/fiatjaf/eventstore/lmdb"
	"github.com/fiatjaf/eventstore/postgresql"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/nbd-wtf/go-nostr"
)

type Storage struct {
	db             eventstore.Store
	archiveEnabled bool
	analyticsDB    *sqlx.DB // Separate PostgreSQL database for analytics
}

func New(backend, path string, archiveEnabled bool, analyticsDBURL string) (*Storage, error) {
	var db eventstore.Store

	switch backend {
	case "lmdb":
		db = &lmdb.LMDBBackend{
			Path:    path,
			MapSize: 1 << 34, // 16GB
		}
	case "postgresql":
		db = &postgresql.PostgresBackend{
			DatabaseURL: path,
			QueryLimit:  1000000,
		}
	default:
		return nil, fmt.Errorf("unsupported storage backend: %s (supported: lmdb, postgresql)", backend)
	}

	if err := db.Init(); err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}

	storage := &Storage{db: db, archiveEnabled: archiveEnabled}

	// Connect to separate analytics database if provided (PostgreSQL only)
	if analyticsDBURL != "" {
		analyticsDB, err := sqlx.Connect("postgres", analyticsDBURL)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to analytics database: %w", err)
		}
		storage.analyticsDB = analyticsDB
		log.Printf("Connected to separate analytics database (PostgreSQL): %s", analyticsDBURL)
	}


	if archiveEnabled {
		log.Println("Event archiving enabled for replaceable events")
	}

	return storage, nil
}

func (s *Storage) SaveEvent(ctx context.Context, evt *nostr.Event) error {
	if s.archiveEnabled && isReplaceableKind(evt.Kind) {
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

// isReplaceableKind returns true for replaceable event kinds (NIP-01)
func isReplaceableKind(kind int) bool {
	// Kind 0 (profile), Kind 3 (contacts), 10000-19999 (replaceable)
	return kind == 0 || kind == 3 || (kind >= 10000 && kind < 20000)
}

// archiveOldVersion archives the current version before replacement (only for trusted pubkeys)
func (s *Storage) archiveOldVersion(ctx context.Context, newEvt *nostr.Event) {
	// Only archive history for trusted pubkeys
	start := time.Now()
	trusted := s.IsPubkeyTrusted(ctx, newEvt.PubKey)
	if elapsed := time.Since(start); elapsed > 20*time.Millisecond {
		log.Printf("SLOW IsPubkeyTrusted: elapsed=%v pubkey=%s", elapsed, newEvt.PubKey[:8])
	}
	if !trusted {
		return
	}

	// Query for existing event
	start = time.Now()
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
func (s *Storage) GetEventCountsByKind(ctx context.Context) (map[int]int64, error) {
	// For SQL backends (SQLite/PostgreSQL), query the event table directly
	dbConn := s.getDBConn()
	if dbConn != nil {
		rows, err := dbConn.QueryContext(ctx, `SELECT kind, COUNT(*) FROM event GROUP BY kind`)
		if err == nil {
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
	}

	// For LMDB: iterate through events and count by kind
	// This is slower but works without SQL tables
	result := make(map[int]int64)

	// Query all events (no filter) and count by kind
	ch, err := s.db.QueryEvents(ctx, nostr.Filter{})
	if err != nil {
		return nil, err
	}

	for evt := range ch {
		result[evt.Kind]++
	}

	return result, nil
}

func (s *Storage) Close() {
	s.db.Close()
}

// EventStore returns the underlying eventstore.Store for direct access
func (s *Storage) EventStore() eventstore.Store {
	return s.db
}

// SearchProfiles searches kind:0 events for profiles matching the query
func (s *Storage) SearchProfiles(ctx context.Context, query string, limit int) ([]*nostr.Event, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	// Search in content field (which contains JSON with name, display_name, about, nip05)
	// Also search by pubkey prefix using PostgreSQL ILIKE (case-insensitive)
	sql := `
		SELECT id, pubkey, created_at, kind, tags, content, sig
		FROM event
		WHERE kind = 0
		AND (
			content ILIKE '%' || $1 || '%'
			OR pubkey LIKE $2 || '%'
		)
		ORDER BY created_at DESC
		LIMIT $3`

	rows, err := dbConn.QueryContext(ctx, sql, query, query, limit*2) // Fetch extra to account for duplicates
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seen := make(map[string]*nostr.Event)
	for rows.Next() {
		var evt nostr.Event
		var tagsJSON string
		if err := rows.Scan(&evt.ID, &evt.PubKey, &evt.CreatedAt, &evt.Kind, &tagsJSON, &evt.Content, &evt.Sig); err != nil {
			continue
		}
		if err := json.Unmarshal([]byte(tagsJSON), &evt.Tags); err != nil {
			evt.Tags = nil
		}
		// Keep only the latest event per pubkey
		if existing, ok := seen[evt.PubKey]; !ok || evt.CreatedAt > existing.CreatedAt {
			seen[evt.PubKey] = &evt
		}
	}

	results := make([]*nostr.Event, 0, len(seen))
	for _, evt := range seen {
		results = append(results, evt)
		if len(results) >= limit {
			break
		}
	}

	return results, nil
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
