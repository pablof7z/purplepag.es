package storage

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/fiatjaf/eventstore"
	"github.com/fiatjaf/eventstore/lmdb"
	"github.com/fiatjaf/eventstore/sqlite3"
	"github.com/nbd-wtf/go-nostr"
)

type Storage struct {
	db eventstore.Store
}

func New(backend, path string) (*Storage, error) {
	var db eventstore.Store

	switch backend {
	case "lmdb":
		db = &lmdb.LMDBBackend{
			Path:    path,
			MapSize: 1 << 34, // 16GB
		}
	case "sqlite3":
		db = &sqlite3.SQLite3Backend{
			DatabaseURL: path,
			QueryLimit:  1000000,
		}
	default:
		return nil, fmt.Errorf("unsupported storage backend: %s (supported: lmdb, sqlite3)", backend)
	}

	if err := db.Init(); err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}

	storage := &Storage{db: db}

	// Apply SQLite optimizations if using SQLite
	if backend == "sqlite3" {
		if err := storage.ApplySQLiteOptimizations(); err != nil {
			return nil, fmt.Errorf("failed to apply SQLite optimizations: %w", err)
		}
	}

	return storage, nil
}

func (s *Storage) ApplySQLiteOptimizations() error {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil
	}

	optimizations := []string{
		// Enable WAL mode for better concurrency
		"PRAGMA journal_mode=WAL",
		// Reduce fsync frequency (faster, but slightly less durable)
		"PRAGMA synchronous=NORMAL",
		// Use 64MB cache (negative value = KB)
		"PRAGMA cache_size=-64000",
		// Store temp tables in memory
		"PRAGMA temp_store=MEMORY",
		// Use memory-mapped I/O for reads (256MB)
		"PRAGMA mmap_size=268435456",
	}

	for _, pragma := range optimizations {
		if _, err := dbConn.Exec(pragma); err != nil {
			return fmt.Errorf("failed to execute %s: %w", pragma, err)
		}
	}

	// Add strategic indexes for hydrator queries
	indexes := []string{
		// Index for counting events by kind (used by stats page)
		"CREATE INDEX IF NOT EXISTS idx_event_kind ON event(kind)",
		// Composite index for kind+pubkey lookups (used by CheckPubkeyEventKinds)
		"CREATE INDEX IF NOT EXISTS idx_kind_pubkey ON event(kind, pubkey)",
		// Index for profile searches (kind 0 events sorted by created_at)
		"CREATE INDEX IF NOT EXISTS idx_kind_created ON event(kind, created_at DESC)",
	}

	for _, indexSQL := range indexes {
		if _, err := dbConn.Exec(indexSQL); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	return nil
}

func (s *Storage) SaveEvent(ctx context.Context, evt *nostr.Event) error {
	// Archive old version for replaceable events before saving
	if isReplaceableKind(evt.Kind) {
		s.archiveOldVersion(ctx, evt)
	}
	return s.db.SaveEvent(ctx, evt)
}

// isReplaceableKind returns true for replaceable event kinds (NIP-01)
func isReplaceableKind(kind int) bool {
	// Kind 0 (profile), Kind 3 (contacts), 10000-19999 (replaceable)
	return kind == 0 || kind == 3 || (kind >= 10000 && kind < 20000)
}

// archiveOldVersion archives the current version before replacement (only for trusted pubkeys)
func (s *Storage) archiveOldVersion(ctx context.Context, newEvt *nostr.Event) {
	// Only archive history for trusted pubkeys
	if !s.IsPubkeyTrusted(ctx, newEvt.PubKey) {
		return
	}

	// Query for existing event
	existing, err := s.QueryEvents(ctx, nostr.Filter{
		Kinds:   []int{newEvt.Kind},
		Authors: []string{newEvt.PubKey},
		Limit:   1,
	})
	if err != nil || len(existing) == 0 {
		return
	}

	oldEvt := existing[0]
	// Only archive if old event is different and older
	if oldEvt.ID != newEvt.ID && oldEvt.CreatedAt < newEvt.CreatedAt {
		s.ArchiveEvent(ctx, oldEvt)
	}
}

func (s *Storage) QueryEvents(ctx context.Context, filter nostr.Filter) ([]*nostr.Event, error) {
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

func (s *Storage) CountEvents(ctx context.Context, kind int) (int64, error) {
	if counter, ok := s.db.(interface {
		CountEvents(context.Context, nostr.Filter) (int64, error)
	}); ok {
		return counter.CountEvents(ctx, nostr.Filter{Kinds: []int{kind}})
	}

	events, err := s.QueryEvents(ctx, nostr.Filter{Kinds: []int{kind}})
	if err != nil {
		return 0, err
	}
	return int64(len(events)), nil
}

// GetEventCountsByKind returns counts for all kinds stored in the database
func (s *Storage) GetEventCountsByKind(ctx context.Context) (map[int]int64, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	rows, err := dbConn.QueryContext(ctx, `SELECT kind, COUNT(*) FROM event GROUP BY kind`)
	if err != nil {
		return nil, err
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
	// Also search by pubkey prefix
	rows, err := dbConn.QueryContext(ctx, `
		SELECT id, pubkey, created_at, kind, tags, content, sig
		FROM event
		WHERE kind = 0
		AND (
			content LIKE '%' || ? || '%' COLLATE NOCASE
			OR pubkey LIKE ? || '%'
		)
		ORDER BY created_at DESC
		LIMIT ?
	`, query, query, limit*2) // Fetch extra to account for duplicates
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
