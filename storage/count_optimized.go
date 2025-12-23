package storage

import (
	"context"

	"github.com/fiatjaf/eventstore"
	"github.com/nbd-wtf/go-nostr"
)

// CountEvents implements khatru's COUNT handler interface
// This is called by khatru when it receives a COUNT message from a client
func (s *Storage) CountEvents(ctx context.Context, filter nostr.Filter) (int64, error) {
	// Check if the backend implements the Counter interface
	if counter, ok := s.db.(eventstore.Counter); ok {
		return counter.CountEvents(ctx, filter)
	}

	// Fallback: query and count
	events, err := s.QueryEvents(ctx, filter)
	if err != nil {
		return 0, err
	}
	return int64(len(events)), nil
}
