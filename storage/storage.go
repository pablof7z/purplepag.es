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

	return &Storage{db: db}, nil
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

func (s *Storage) Close() error {
	if closer, ok := s.db.(interface{ Close() error }); ok {
		return closer.Close()
	}
	return nil
}
