package storage

import (
	"context"
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
		// Composite index for kind+pubkey lookups (used by CheckPubkeyEventKinds)
		"CREATE INDEX IF NOT EXISTS idx_kind_pubkey ON event(kind, pubkey)",
	}

	for _, indexSQL := range indexes {
		if _, err := dbConn.Exec(indexSQL); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	return nil
}

func (s *Storage) SaveEvent(ctx context.Context, evt *nostr.Event) error {
	return s.db.SaveEvent(ctx, evt)
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
